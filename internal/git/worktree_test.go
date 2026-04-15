package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-billy/v6/osfs"
	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	xworktree "github.com/go-git/go-git/v6/x/plumbing/worktree"
)

// runGit runs a git command in dir, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// setupTestRepo creates a temporary git repository with an initial commit.
func setupTestRepo(t *testing.T) (string, *gogit.Repository) {
	t.Helper()
	dir := t.TempDir()

	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	// Create an initial commit so HEAD exists.
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("failed to get worktree: %v", err)
	}

	f, err := wt.Filesystem.Create("README.md")
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	_, _ = f.Write([]byte("# Test\n"))
	_ = f.Close()

	_, err = wt.Add("README.md")
	if err != nil {
		t.Fatalf("failed to add file: %v", err)
	}

	_, err = wt.Commit("initial commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "test",
			Email: "test@test.com",
		},
	})
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	return dir, repo
}

func TestOpenRepository(t *testing.T) {
	dir, _ := setupTestRepo(t)

	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}

	if repo.Root() != dir {
		t.Errorf("got root %q, want %q", repo.Root(), dir)
	}
}

func TestListWorktrees_MainOnly(t *testing.T) {
	dir, _ := setupTestRepo(t)

	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}

	wts, err := repo.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}

	if len(wts) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(wts))
	}

	if !wts[0].IsMain {
		t.Error("expected main worktree")
	}

	if wts[0].Branch != "master" && wts[0].Branch != "main" {
		t.Errorf("unexpected branch %q", wts[0].Branch)
	}

	if wts[0].IsDirty {
		t.Error("expected clean worktree")
	}
}

func TestListWorktrees_WithLinked(t *testing.T) {
	dir, repo := setupTestRepo(t)

	// Create a linked worktree using go-git's x/plumbing/worktree.
	wt, err := xworktree.New(repo.Storer)
	if err != nil {
		t.Fatalf("failed to create worktree manager: %v", err)
	}

	linkedPath := filepath.Join(filepath.Dir(dir), "test-branch")
	t.Cleanup(func() { _ = os.RemoveAll(linkedPath) })

	linkedFS := osfs.New(linkedPath)
	err = wt.Add(linkedFS, "test-branch")
	if err != nil {
		t.Fatalf("failed to add worktree: %v", err)
	}

	// Open with our wrapper and list.
	biomelab, err := OpenRepository(dir)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}

	wts, err := biomelab.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}

	if len(wts) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(wts))
	}

	found := false
	for _, w := range wts {
		if w.Branch == "test-branch" {
			found = true
			if w.IsMain {
				t.Error("linked worktree should not be main")
			}
		}
	}
	if !found {
		t.Error("linked worktree not found in list")
	}
}

func TestCreateAndRemoveWorktree(t *testing.T) {
	dir, _ := setupTestRepo(t)

	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}

	branchName := "feature-test"
	linkedPath := filepath.Join(filepath.Dir(dir), branchName)
	t.Cleanup(func() { _ = os.RemoveAll(linkedPath) })

	err = repo.CreateWorktree(branchName)
	if err != nil {
		t.Fatalf("CreateWorktree failed: %v", err)
	}

	// Verify it exists.
	wts, err := repo.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}
	if len(wts) != 2 {
		t.Fatalf("expected 2 worktrees after create, got %d", len(wts))
	}

	// Remove it.
	err = repo.RemoveWorktree(branchName)
	if err != nil {
		t.Fatalf("RemoveWorktree failed: %v", err)
	}

	wts, err = repo.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}
	if len(wts) != 1 {
		t.Fatalf("expected 1 worktree after remove, got %d", len(wts))
	}
}

// setupWorktreeManually creates a linked worktree with its metadata.
// wtName is the worktree name (used for the metadata dir under .git/worktrees/).
// branchName is the branch (may contain slashes like "ralph/issue-1456").
// This simulates what `git worktree add -b ralph/issue-1456 ../1492` does.
func setupWorktreeManually(t *testing.T, repoDir, wtName, branchName string) string {
	t.Helper()

	repo, err := OpenRepository(repoDir)
	if err != nil {
		t.Fatalf("failed to open repo: %v", err)
	}
	head, err := repo.repo.Head()
	if err != nil {
		t.Fatalf("failed to get HEAD: %v", err)
	}
	commitHash := head.Hash().String()

	wtPath := filepath.Join(filepath.Dir(repoDir), wtName)
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatalf("failed to create worktree dir: %v", err)
	}

	metaDir := filepath.Join(repoDir, ".git", "worktrees", wtName)
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatalf("failed to create meta dir: %v", err)
	}

	writeFile := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", path, err)
		}
	}

	writeFile(filepath.Join(metaDir, "gitdir"), filepath.Join(wtPath, ".git")+"\n")
	writeFile(filepath.Join(metaDir, "HEAD"), "ref: refs/heads/"+branchName+"\n")
	writeFile(filepath.Join(metaDir, "commondir"), "../..\n")
	if err := os.MkdirAll(filepath.Join(metaDir, "refs"), 0o755); err != nil {
		t.Fatalf("failed to create refs dir: %v", err)
	}
	writeFile(filepath.Join(wtPath, ".git"), "gitdir: "+metaDir+"\n")

	// Create the branch reference (may need intermediate dirs for slashed names).
	refsDir := filepath.Join(repoDir, ".git", "refs", "heads")
	if err := os.MkdirAll(filepath.Dir(filepath.Join(refsDir, branchName)), 0o755); err != nil {
		t.Fatalf("failed to create branch ref dir: %v", err)
	}
	writeFile(filepath.Join(refsDir, branchName), commitHash+"\n")

	return wtPath
}

func TestRemoveWorktree_WithSlashedBranch(t *testing.T) {
	dir, _ := setupTestRepo(t)

	// Git stores the worktree under a flat name (e.g., "1492"),
	// even if the branch is "ralph/issue-1456".
	wtName := "1492"
	branchName := "ralph/issue-1456"
	wtPath := setupWorktreeManually(t, dir, wtName, branchName)
	t.Cleanup(func() { _ = os.RemoveAll(wtPath) })

	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}

	// Verify the worktree appears with the slashed branch name.
	wts, err := repo.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}

	found := false
	for _, wt := range wts {
		if wt.Branch == branchName {
			found = true
		}
	}
	if !found {
		t.Fatalf("worktree with branch %q not found in list", branchName)
	}

	// Remove using the worktree name (not the branch name).
	err = repo.RemoveWorktree(wtName)
	if err != nil {
		t.Fatalf("RemoveWorktree failed: %v", err)
	}

	// Verify it's gone from the list.
	wts, err = repo.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}
	for _, wt := range wts {
		if wt.Branch == branchName {
			t.Errorf("worktree with branch %q still present after removal", branchName)
		}
	}

	// Verify directory and metadata are gone.
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree directory %q still exists", wtPath)
	}
	metaDir := filepath.Join(dir, ".git", "worktrees", wtName)
	if _, err := os.Stat(metaDir); !os.IsNotExist(err) {
		t.Errorf("metadata directory %q still exists", metaDir)
	}
}

func TestRemoveWorktree_Simple(t *testing.T) {
	dir, _ := setupTestRepo(t)

	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}

	branchName := "feature-remove"
	linkedPath := filepath.Join(filepath.Dir(dir), branchName)
	t.Cleanup(func() { _ = os.RemoveAll(linkedPath) })

	err = repo.CreateWorktree(branchName)
	if err != nil {
		t.Fatalf("CreateWorktree failed: %v", err)
	}

	err = repo.RemoveWorktree(branchName)
	if err != nil {
		t.Fatalf("RemoveWorktree failed: %v", err)
	}

	if _, err := os.Stat(linkedPath); !os.IsNotExist(err) {
		t.Error("worktree directory still exists")
	}

	metaDir := filepath.Join(dir, ".git", "worktrees", branchName)
	if _, err := os.Stat(metaDir); !os.IsNotExist(err) {
		t.Error("metadata directory still exists")
	}

	wts, err := repo.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}
	if len(wts) != 1 {
		t.Fatalf("expected 1 worktree after remove, got %d", len(wts))
	}
}

func TestDirtyDetection(t *testing.T) {
	dir, _ := setupTestRepo(t)

	// Modify a file to make it dirty.
	err := os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("dirty"), 0644)
	if err != nil {
		t.Fatalf("failed to create dirty file: %v", err)
	}

	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}

	wts, err := repo.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}

	if len(wts) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(wts))
	}

	if !wts[0].IsDirty {
		t.Error("expected dirty worktree after creating untracked file")
	}
}

func TestRepoRoot(t *testing.T) {
	dir, _ := setupTestRepo(t)

	// Create a subdirectory and test detection from there.
	subDir := filepath.Join(dir, "sub", "dir")
	err := os.MkdirAll(subDir, 0755)
	if err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	root, err := RepoRoot(subDir)
	if err != nil {
		t.Fatalf("RepoRoot failed: %v", err)
	}

	if root != dir {
		t.Errorf("got root %q, want %q", root, dir)
	}
}

func TestSanitizeWorktreeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"main", "main"},
		{"feature-foo", "feature-foo"},
		{"ralph/issue-19", "ralph-issue-19"},
		{"org/repo/deep", "org-repo-deep"},
		{"no-slash", "no-slash"},
	}
	for _, tt := range tests {
		got := sanitizeWorktreeName(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeWorktreeName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// setupRemoteWithPRRef creates a bare-ish git repo that advertises a
// refs/pull/<prNumber>/head ref, which is how GitHub exposes PR heads.
// Returns the path to the remote repo.
func setupRemoteWithPRRef(t *testing.T, prNumber int) string {
	t.Helper()

	remoteDir := t.TempDir()
	runGit(t, remoteDir, "init", "-b", "main")
	runGit(t, remoteDir, "-c", "user.email=test@test.com", "-c", "user.name=Test",
		"commit", "--allow-empty", "-m", "base commit")
	// Create a PR head ref (simulating GitHub's refs/pull/<N>/head).
	runGit(t, remoteDir, "update-ref",
		fmt.Sprintf("refs/pull/%d/head", prNumber), "HEAD")
	return remoteDir
}

func TestFetchPR_SlashedBranchName(t *testing.T) {
	t.Run("worktree dir is sanitized, branch ref preserves original name", func(t *testing.T) {
		remoteDir := setupRemoteWithPRRef(t, 19)

		mainDir := t.TempDir()
		runGit(t, mainDir, "init", "-b", "main")
		runGit(t, mainDir, "-c", "user.email=test@test.com", "-c", "user.name=Test",
			"commit", "--allow-empty", "-m", "initial commit")
		runGit(t, mainDir, "remote", "add", "origin", remoteDir)

		repo, err := OpenRepository(mainDir)
		if err != nil {
			t.Fatalf("OpenRepository: %v", err)
		}

		branchName := "ralph/issue-19"
		// Pass remoteDir as remoteURL to simulate fetching from a fork.
		wtPath, err := repo.FetchPR(19, branchName, remoteDir)
		if err != nil {
			t.Fatalf("FetchPR: %v", err)
		}

		// Worktree path must use the sanitized directory name.
		wantPath := filepath.Join(mainDir, ".biomelab-worktrees", "ralph-issue-19")
		if wtPath != wantPath {
			t.Errorf("wtPath = %q, want %q", wtPath, wantPath)
		}

		// Worktree directory must exist on disk.
		if _, err := os.Stat(wtPath); os.IsNotExist(err) {
			t.Error("worktree directory was not created")
		}

		// The worktree's HEAD must reference the original branch name (with slash).
		headFile := filepath.Join(mainDir, ".git", "worktrees", "ralph-issue-19", "HEAD")
		data, err := os.ReadFile(headFile)
		if err != nil {
			t.Fatalf("read worktree HEAD: %v", err)
		}
		if !strings.Contains(string(data), "refs/heads/ralph/issue-19") {
			t.Errorf("expected HEAD to reference refs/heads/ralph/issue-19, got: %s", strings.TrimSpace(string(data)))
		}
	})

	t.Run("no collision when sanitized name differs from existing branch", func(t *testing.T) {
		remoteDir := setupRemoteWithPRRef(t, 42)

		mainDir := t.TempDir()
		runGit(t, mainDir, "init", "-b", "main")
		runGit(t, mainDir, "-c", "user.email=test@test.com", "-c", "user.name=Test",
			"commit", "--allow-empty", "-m", "initial commit")
		runGit(t, mainDir, "remote", "add", "origin", remoteDir)
		// Pre-create a local branch named with a dash (what used to collide).
		runGit(t, mainDir, "branch", "ralph-issue-42")

		repo, err := OpenRepository(mainDir)
		if err != nil {
			t.Fatalf("OpenRepository: %v", err)
		}

		// ralph/issue-42 (slash) should NOT collide with ralph-issue-42 (dash).
		wtPath, err := repo.FetchPR(42, "ralph/issue-42", remoteDir)
		if err != nil {
			t.Fatalf("FetchPR: %v", err)
		}

		if _, err := os.Stat(wtPath); os.IsNotExist(err) {
			t.Error("worktree directory was not created")
		}
	})
}

// setupRepoWithMultipleRemotes creates a local repo with two remotes ("origin" and "upstream")
// that share a common history. Origin is the initial repo, upstream is cloned from origin,
// and local is cloned from origin with upstream added as a second remote.
func setupRepoWithMultipleRemotes(t *testing.T, branchName string) (localDir string, originDir string, upstreamDir string) {
	t.Helper()

	// Create origin remote repo with an initial commit.
	originDir = t.TempDir()
	runGit(t, originDir, "init", "-b", branchName)
	runGit(t, originDir, "-c", "user.email=test@test.com", "-c", "user.name=Test",
		"commit", "--allow-empty", "-m", "initial commit")

	// Create upstream by cloning origin (shared history).
	upstreamDir = t.TempDir()
	runGit(t, upstreamDir, "clone", originDir, ".")

	// Create local repo cloned from origin, then add upstream.
	localDir = t.TempDir()
	runGit(t, localDir, "clone", originDir, ".")
	runGit(t, localDir, "remote", "add", "upstream", upstreamDir)
	runGit(t, localDir, "fetch", "upstream")

	return localDir, originDir, upstreamDir
}

func TestFetch_MultipleRemotes(t *testing.T) {
	branchName := "main"
	localDir, originDir, upstreamDir := setupRepoWithMultipleRemotes(t, branchName)

	// Add a commit to origin.
	runGit(t, originDir, "-c", "user.email=test@test.com", "-c", "user.name=Test",
		"commit", "--allow-empty", "-m", "origin second commit")

	// Add a commit to upstream.
	runGit(t, upstreamDir, "-c", "user.email=test@test.com", "-c", "user.name=Test",
		"commit", "--allow-empty", "-m", "upstream second commit")

	repo, err := OpenRepository(localDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}

	// Record remote refs before fetch.
	originBefore := runGit(t, localDir, "rev-parse", "refs/remotes/origin/"+branchName)
	upstreamBefore := runGit(t, localDir, "rev-parse", "refs/remotes/upstream/"+branchName)

	if err := repo.Fetch(); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// After fetch, both remote tracking refs should be updated.
	originAfter := runGit(t, localDir, "rev-parse", "refs/remotes/origin/"+branchName)
	upstreamAfter := runGit(t, localDir, "rev-parse", "refs/remotes/upstream/"+branchName)

	if originAfter == originBefore {
		t.Error("origin tracking ref was not updated by Fetch")
	}
	if upstreamAfter == upstreamBefore {
		t.Error("upstream tracking ref was not updated by Fetch")
	}
}

func TestSyncStatus_ChecksUpstream(t *testing.T) {
	branchName := "main"
	localDir, _, upstreamDir := setupRepoWithMultipleRemotes(t, branchName)

	// Add a commit to upstream but not origin.
	runGit(t, upstreamDir, "-c", "user.email=test@test.com", "-c", "user.name=Test",
		"commit", "--allow-empty", "-m", "upstream ahead commit")

	// Fetch upstream so the tracking ref is updated.
	runGit(t, localDir, "fetch", "upstream")

	repo, err := OpenRepository(localDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}

	wts, err := repo.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}

	// The main worktree should be behind (upstream has a commit we don't have).
	if len(wts) == 0 {
		t.Fatal("expected at least one worktree")
	}
	if wts[0].Sync != SyncBehind {
		t.Errorf("expected SyncBehind, got %v", wts[0].Sync)
	}
}

func TestPull_MultipleRemotes(t *testing.T) {
	branchName := "main"
	localDir, originDir, upstreamDir := setupRepoWithMultipleRemotes(t, branchName)

	// Add commits to both remotes.
	runGit(t, originDir, "-c", "user.email=test@test.com", "-c", "user.name=Test",
		"commit", "--allow-empty", "-m", "origin new commit")
	runGit(t, upstreamDir, "-c", "user.email=test@test.com", "-c", "user.name=Test",
		"commit", "--allow-empty", "-m", "upstream new commit")

	repo, err := OpenRepository(localDir)
	if err != nil {
		t.Fatalf("OpenRepository: %v", err)
	}

	upstreamBefore := runGit(t, localDir, "rev-parse", "refs/remotes/upstream/"+branchName)

	if err := repo.Pull(); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	// Pull should have fetched upstream too (even though merge is from origin).
	upstreamAfter := runGit(t, localDir, "rev-parse", "refs/remotes/upstream/"+branchName)
	if upstreamAfter == upstreamBefore {
		t.Error("upstream tracking ref was not updated during Pull")
	}

	// Local branch should have origin's new commit (merged via pull).
	localHead := runGit(t, localDir, "rev-parse", "HEAD")
	originHead := runGit(t, originDir, "rev-parse", "HEAD")
	if localHead != originHead {
		t.Errorf("local HEAD %s != origin HEAD %s after pull", localHead, originHead)
	}
}

func TestParseRepoName(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		// HTTPS
		{"https://github.com/docker/sandboxes.git", "docker/sandboxes"},
		{"https://github.com/mdelapenya/biomelab.git", "mdelapenya/biomelab"},
		{"https://github.com/owner/repo", "owner/repo"},
		{"https://gitlab.com/group/project.git", "group/project"},
		// SSH
		{"git@github.com:docker/sandboxes.git", "docker/sandboxes"},
		{"git@github.com:mdelapenya/biomelab.git", "mdelapenya/biomelab"},
		{"git@gitlab.com:group/project.git", "group/project"},
		// Edge cases
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := parseRepoName(tt.url)
			if got != tt.want {
				t.Errorf("parseRepoName(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

