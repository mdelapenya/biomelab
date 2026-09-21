package git

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-git/go-billy/v6/osfs"
	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/format/gitignore"
	githttp "github.com/go-git/go-git/v6/plumbing/transport/http"
	xworktree "github.com/go-git/go-git/v6/x/plumbing/worktree"

	"github.com/mdelapenya/biomelab/internal/command"
)

// biomelabDir is the per-worktree sidecar directory that biomelab uses
// to store notes, PR drafts, and other transient state. Kept under the
// worktree so it travels with the sandbox bind mount.
const (
	biomelabDir         = ".biomelab"
	biomelabExcludeLine = "/.biomelab/"
)

// SyncStatus indicates whether a branch is up-to-date with its remote tracking branch.
type SyncStatus int

const (
	SyncUnknown    SyncStatus = iota
	SyncUpToDate              // local == remote
	SyncAhead                 // local has commits not on remote
	SyncBehind                // remote has commits not on local
	SyncDiverged              // both have commits the other doesn't
	SyncNoUpstream            // no remote tracking branch
)

// Worktree holds the information about a single git worktree.
//
// Name is the git worktree identifier — the directory entry under
// .git/worktrees/<Name>. It is the sanitized form of the branch (slashes
// replaced with dashes), and is what RemoveWorktree expects. Empty for the
// main worktree, which has no metadata dir.
type Worktree struct {
	Name     string
	Path     string
	Branch   string
	IsMain   bool
	IsDirty  bool
	Detached bool
	Sync     SyncStatus
}

// Repository wraps a go-git repository and its worktree manager.
type Repository struct {
	mu       sync.Mutex
	repo     *gogit.Repository
	wt       *xworktree.Worktree
	repoRoot string

	// generation increments each time the worktree set is mutated
	// (Create/Remove/FetchPR). Snapshots capture this value under mu so
	// refresh pipelines can drop results taken before a later mutation —
	// otherwise a long-running refresh dispatched after a delete can
	// resurrect a removed card with its pre-delete snapshot.
	generation atomic.Uint64
}

// Snapshot is a worktree listing paired with the generation counter at the
// moment the listing was taken. Apply-time staleness checks compare
// Generation against the last-applied value to drop pre-mutation snapshots.
type Snapshot struct {
	Worktrees  []Worktree
	Generation uint64
}

// OpenRepository opens the git repository at the given path.
func OpenRepository(path string) (*Repository, error) {
	repo, err := gogit.PlainOpen(path)
	if err != nil {
		return nil, err
	}

	wt, err := xworktree.New(repo.Storer)
	if err != nil {
		return nil, err
	}

	return &Repository{
		repo:     repo,
		wt:       wt,
		repoRoot: path,
	}, nil
}

// Root returns the repository root path.
func (r *Repository) Root() string {
	return r.repoRoot
}

// reopen refreshes the go-git repo and worktree manager handles.
// This is necessary because go-git caches packfile indexes in memory,
// and if git gc/repack runs externally, the cache goes stale causing
// "packfile not found" errors.
func (r *Repository) reopen() error {
	repo, err := gogit.PlainOpen(r.repoRoot)
	if err != nil {
		return err
	}
	wt, err := xworktree.New(repo.Storer)
	if err != nil {
		return err
	}
	r.repo = repo
	r.wt = wt
	return nil
}

// OriginURL returns the first URL of the first remote (typically origin).
// Returns an empty string if no remote is configured.
func (r *Repository) OriginURL() string {
	remotes, err := r.repo.Remotes()
	if err == nil && len(remotes) > 0 {
		urls := remotes[0].Config().URLs
		if len(urls) > 0 {
			return urls[0]
		}
	}
	return ""
}

// RepoName returns the "owner/repo" name derived from the origin remote URL.
// Falls back to the directory name if no remote is configured. When no origin
// is set but the repo lives under a forge-style path (e.g.
// ".../github.com/<owner>/<repo>"), returns "<owner>/<repo>" so that repos
// without a configured remote (like an empty, freshly-init'd repo) still get
// a consistent name.
func (r *Repository) RepoName() string {
	if url := r.OriginURL(); url != "" {
		if name := parseRepoName(url); name != "" {
			return name
		}
	}
	return inferRepoNameFromPath(r.repoRoot)
}

// inferRepoNameFromPath returns "<owner>/<repo>" when repoRoot lives under a
// directory that looks like a forge hostname (contains a dot, e.g.
// "github.com", "gitlab.com", "bitbucket.org"). Otherwise returns the
// directory basename.
func inferRepoNameFromPath(repoRoot string) string {
	base := filepath.Base(repoRoot)
	parent := filepath.Dir(repoRoot)
	if parent == repoRoot || parent == "." || parent == "/" {
		return base
	}
	parentName := filepath.Base(parent)
	grandparent := filepath.Dir(parent)
	if grandparent == parent {
		return base
	}
	grandparentName := filepath.Base(grandparent)
	if strings.Contains(grandparentName, ".") && parentName != "" && parentName != "." && parentName != "/" {
		return parentName + "/" + base
	}
	return base
}

// parseRepoName extracts "owner/repo" from a git remote URL.
func parseRepoName(remoteURL string) string {
	// Handle SSH: git@github.com:owner/repo.git
	// SSH URLs have a colon after the host, but no "://" scheme prefix.
	if idx := strings.Index(remoteURL, ":"); idx > 0 &&
		!strings.Contains(remoteURL[:idx], "/") &&
		!strings.Contains(remoteURL[:idx], "//") &&
		(len(remoteURL) <= idx+2 || remoteURL[idx:idx+3] != "://") {
		path := remoteURL[idx+1:]
		path = strings.TrimSuffix(path, ".git")
		return path
	}
	// Handle HTTPS: https://github.com/owner/repo.git
	remoteURL = strings.TrimSuffix(remoteURL, ".git")
	parts := strings.Split(remoteURL, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	return ""
}

// Fetch updates remote tracking refs so sync status is accurate.
// It fetches from all configured remotes so that repos with multiple
// remotes (e.g. origin, upstream) stay current.
// Uses a 15-second context timeout per remote to cancel slow fetches cleanly.
func (r *Repository) Fetch() error {
	return r.FetchContext(context.Background())
}

// FetchContext stops remote fetches and credential lookups with the refresh run.
func (r *Repository) FetchContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.reopen(); err != nil {
		return err
	}

	remotes, err := r.repo.Remotes()
	if err != nil {
		return fmt.Errorf("listing remotes: %w", err)
	}

	var firstErr error
	for _, remote := range remotes {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := remote.Config().Name
		remoteCtx, cancel := context.WithTimeout(ctx, 15*time.Second)

		opts := &gogit.FetchOptions{RemoteName: name}
		err := r.repo.FetchContext(remoteCtx, opts)
		if err != nil && err != gogit.NoErrAlreadyUpToDate {
			if isAuthError(err) {
				auth, credErr := r.resolveCredentialsForRemoteContext(remoteCtx, remote)
				if credErr == nil {
					opts.Auth = auth
					err = r.repo.FetchContext(remoteCtx, opts)
				}
			}
			if err != nil && err != gogit.NoErrAlreadyUpToDate && firstErr == nil {
				firstErr = fmt.Errorf("fetch %s: %w", name, err)
			}
		}

		cancel()
	}

	return firstErr
}

// parseHeadFile reads a git HEAD file and reports the short branch name or
// short detached hash. Returns ok=false if the file is missing or empty.
func parseHeadFile(path string) (branch string, detached bool, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, false
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return "", false, false
	}
	if strings.HasPrefix(s, "ref: ") {
		ref := strings.TrimPrefix(s, "ref: ")
		return strings.TrimPrefix(ref, "refs/heads/"), false, true
	}
	return s[:min(7, len(s))], true, true
}

// ListWorktreesQuick returns worktrees with only branch info — no dirty/sync checks.
// Used for fast initial render.
func (r *Repository) ListWorktreesQuick() ([]Worktree, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.listWorktreesQuickLocked()
}

// SnapshotQuick returns a quick worktree listing together with the
// generation counter at the moment of the snapshot, captured under the
// same lock. Use this in refresh pipelines where downstream apply-time
// checks need to detect pre-mutation snapshots.
func (r *Repository) SnapshotQuick() (Snapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	wts, err := r.listWorktreesQuickLocked()
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Worktrees: wts, Generation: r.generation.Load()}, nil
}

func (r *Repository) listWorktreesQuickLocked() ([]Worktree, error) {
	if err := r.reopen(); err != nil {
		return nil, err
	}

	var result []Worktree

	// Main worktree — branch only.
	mainWt := Worktree{Path: r.repoRoot, IsMain: true}
	head, err := r.repo.Head()
	switch {
	case err == nil:
		if head.Name().IsBranch() {
			mainWt.Branch = head.Name().Short()
		} else {
			mainWt.Detached = true
			mainWt.Branch = head.Hash().String()[:7]
		}
	case errors.Is(err, plumbing.ErrReferenceNotFound):
		// Unborn HEAD: repo has no commits yet. Read .git/HEAD directly so
		// the main card still shows up for a freshly enrolled empty repo.
		branch, detached, ok := parseHeadFile(filepath.Join(r.repoRoot, ".git", "HEAD"))
		if !ok {
			return nil, err
		}
		mainWt.Branch = branch
		mainWt.Detached = detached
	default:
		return nil, err
	}
	result = append(result, mainWt)

	// Linked worktrees — branch from HEAD file only.
	names, err := r.wt.List()
	if err != nil {
		return result, nil
	}
	for _, name := range names {
		wtMetaDir := filepath.Join(r.repoRoot, ".git", "worktrees", name)
		wtPath, pathErr := readWorktreePath(wtMetaDir)
		if pathErr != nil {
			wtPath = filepath.Join(r.worktreesDir(), name)
		}
		wt := Worktree{Name: name, Path: wtPath}
		headData, headErr := os.ReadFile(filepath.Join(wtMetaDir, "HEAD"))
		if headErr != nil {
			continue
		}
		headStr := strings.TrimSpace(string(headData))
		if strings.HasPrefix(headStr, "ref: ") {
			ref := strings.TrimPrefix(headStr, "ref: ")
			wt.Branch = strings.TrimPrefix(ref, "refs/heads/")
		} else {
			wt.Detached = true
			wt.Branch = headStr[:min(7, len(headStr))]
		}
		result = append(result, wt)
	}
	return result, nil
}

// ListWorktrees returns all worktrees (main + linked) with their metadata.
func (r *Repository) ListWorktrees() ([]Worktree, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.listWorktreesLocked()
}

// Snapshot returns a full worktree listing together with the generation
// counter at the moment of the snapshot, captured under the same lock.
// See SnapshotQuick for the staleness-detection use case.
func (r *Repository) Snapshot() (Snapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	wts, err := r.listWorktreesLocked()
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Worktrees: wts, Generation: r.generation.Load()}, nil
}

// Generation returns the current mutation counter for tests and callers
// that need to observe whether a mutation has occurred.
func (r *Repository) Generation() uint64 {
	return r.generation.Load()
}

func (r *Repository) listWorktreesLocked() ([]Worktree, error) {
	if err := r.reopen(); err != nil {
		return nil, err
	}

	var result []Worktree

	// Add the main worktree.
	mainWt, err := r.mainWorktree()
	if err != nil {
		return nil, err
	}
	result = append(result, *mainWt)

	// Add linked worktrees.
	names, err := r.wt.List()
	if err != nil {
		return nil, err
	}

	for _, name := range names {
		wt, err := r.linkedWorktree(name)
		if err != nil {
			continue
		}
		result = append(result, *wt)
	}

	return result, nil
}

// mainWorktree returns info about the main worktree.
func (r *Repository) mainWorktree() (*Worktree, error) {
	wt := &Worktree{
		Path:   r.repoRoot,
		IsMain: true,
	}

	head, err := r.repo.Head()
	switch {
	case err == nil:
		if head.Name().IsBranch() {
			wt.Branch = head.Name().Short()
		} else {
			wt.Detached = true
			wt.Branch = head.Hash().String()[:7]
		}
	case errors.Is(err, plumbing.ErrReferenceNotFound):
		// Unborn HEAD: repo has no commits yet. Read .git/HEAD directly so
		// the main card still shows up for a freshly enrolled empty repo.
		branch, detached, ok := parseHeadFile(filepath.Join(r.repoRoot, ".git", "HEAD"))
		if !ok {
			return nil, err
		}
		wt.Branch = branch
		wt.Detached = detached
	default:
		return nil, err
	}

	goWt, err := r.repo.Worktree()
	if err != nil {
		return nil, err
	}
	status, err := goWt.Status()
	if err != nil {
		return nil, err
	}
	wt.IsDirty = isDirtyIgnoringBiomelabAndBootstrap(status, wt.Path)
	wt.Sync = r.syncStatus(wt.Branch)

	return wt, nil
}

// isDirtyIgnoringBiomelab returns true when status reports any change other
// than entries under the .biomelab/ directory. go-git's Status() does not
// honor .git/info/exclude (only in-tree .gitignore files), so biomelab's
// own scratchpad files would otherwise mark every worktree dirty even when
// `git status` agrees it's clean. Scoped to .biomelab/ on purpose — fixing
// the broader info/exclude gap in go-git is a separate concern.
func isDirtyIgnoringBiomelab(status gogit.Status) bool {
	for path, s := range status {
		if s.Staging == gogit.Unmodified && s.Worktree == gogit.Unmodified {
			continue
		}
		if path == ".biomelab" || strings.HasPrefix(path, ".biomelab/") {
			continue
		}
		return true
	}
	return false
}

var agentBootstrapExcludeLines = map[string]string{
	"AGENTS.md":                       "/AGENTS.md",
	"AGENTS.override.md":              "/AGENTS.override.md",
	"CLAUDE.md":                       "/CLAUDE.md",
	"GEMINI.md":                       "/GEMINI.md",
	".kiro/steering/biomelab-task.md": "/.kiro/steering/biomelab-task.md",
}

// isDirtyIgnoringBiomelabAndBootstrap accounts for the agent instruction
// files Biomelab creates and records in the repository's common info/exclude.
// go-git does not find that exclude file from a linked worktree because its
// .git entry is a file. Only an untracked bootstrap file with its exact,
// root-anchored exclusion is hidden; tracked changes and other files remain
// visible. If the exclusion state cannot be read, the conservative result is
// dirty.
func isDirtyIgnoringBiomelabAndBootstrap(status gogit.Status, worktreePath string) bool {
	if !isDirtyIgnoringBiomelab(status) {
		return false
	}

	var candidates []string
	for path, fileStatus := range status {
		if fileStatus.Staging != gogit.Untracked || fileStatus.Worktree != gogit.Untracked {
			continue
		}
		if _, ok := agentBootstrapExcludeLines[path]; ok {
			candidates = append(candidates, path)
		}
	}
	if len(candidates) == 0 {
		return true
	}

	excludePath, err := excludeFilePath(worktreePath)
	if err != nil {
		return true
	}
	excludes, err := os.ReadFile(excludePath)
	if err != nil {
		return true
	}
	patterns := excludePatterns(excludes)
	repositoryPatterns, err := gitignore.ReadPatterns(osfs.New(worktreePath), nil)
	if err != nil {
		return true
	}
	// Git gives repository .gitignore files higher priority than
	// info/exclude. Preserve that order so a later negation keeps the file
	// visible even when Biomelab's exact registration line is present.
	patterns = append(patterns, repositoryPatterns...)
	matcher := gitignore.NewMatcher(patterns)

	for path, fileStatus := range status {
		if fileStatus.Staging == gogit.Unmodified && fileStatus.Worktree == gogit.Unmodified {
			continue
		}
		if path == ".biomelab" || strings.HasPrefix(path, ".biomelab/") {
			continue
		}
		excludeLine, isBootstrap := agentBootstrapExcludeLines[path]
		if isBootstrap &&
			fileStatus.Staging == gogit.Untracked && fileStatus.Worktree == gogit.Untracked &&
			hasExcludeLine(excludes, excludeLine) &&
			matcher.Match(strings.Split(path, "/"), false) {
			continue
		}
		return true
	}
	return false
}

func excludePatterns(content []byte) []gitignore.Pattern {
	var patterns []gitignore.Pattern
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || len(strings.TrimSpace(line)) == 0 {
			continue
		}
		patterns = append(patterns, gitignore.ParsePattern(line, nil))
	}
	return patterns
}

// referenceRemotes are the remote names checked for sync status.
// Both origin and upstream are treated as reference remotes.
var referenceRemotes = []string{"origin", "upstream"}

// syncStatus compares the local branch commit with remote tracking branches.
// It checks both origin and upstream remotes, returning the most significant status.
func (r *Repository) syncStatus(branch string) SyncStatus {
	if branch == "" {
		return SyncUnknown
	}

	localRef, err := r.repo.Reference(plumbing.NewBranchReferenceName(branch), true)
	if err != nil {
		return SyncUnknown
	}
	localHash := localRef.Hash()

	// Check each reference remote; return the first non-trivial status found.
	foundAny := false
	for _, remoteName := range referenceRemotes {
		remoteRef, err := r.repo.Reference(plumbing.NewRemoteReferenceName(remoteName, branch), true)
		if err != nil {
			continue
		}
		foundAny = true
		remoteHash := remoteRef.Hash()

		if localHash == remoteHash {
			continue // up-to-date with this remote, check next
		}

		// Check ancestry to determine ahead/behind/diverged.
		localCommit, err := r.repo.CommitObject(localHash)
		if err != nil {
			return SyncUnknown
		}
		remoteCommit, err := r.repo.CommitObject(remoteHash)
		if err != nil {
			return SyncUnknown
		}

		localIsAncestor, _ := localCommit.IsAncestor(remoteCommit)
		remoteIsAncestor, _ := remoteCommit.IsAncestor(localCommit)

		switch {
		case remoteIsAncestor:
			return SyncAhead
		case localIsAncestor:
			return SyncBehind
		default:
			return SyncDiverged
		}
	}

	if !foundAny {
		return SyncNoUpstream
	}
	return SyncUpToDate
}

// linkedWorktree returns info about a linked worktree by name.
func (r *Repository) linkedWorktree(name string) (*Worktree, error) {
	wtMetaDir := filepath.Join(r.repoRoot, ".git", "worktrees", name)

	// Read the actual worktree path from the gitdir file.
	// The gitdir file contains the path to the worktree's .git file.
	wtPath, err := readWorktreePath(wtMetaDir)
	if err != nil {
		// Fallback: assume biomelab-worktrees directory.
		wtPath = filepath.Join(r.worktreesDir(), name)
	}

	wt := &Worktree{
		Name: name,
		Path: wtPath,
	}

	// Read branch from the per-worktree HEAD file in .git/worktrees/<name>/HEAD.
	// linkedRepo.Head() reads the shared HEAD which is always the main branch.
	headFile := filepath.Join(wtMetaDir, "HEAD")
	headData, err := os.ReadFile(headFile)
	if err != nil {
		return nil, err
	}
	headStr := strings.TrimSpace(string(headData))

	if strings.HasPrefix(headStr, "ref: ") {
		ref := strings.TrimPrefix(headStr, "ref: ")
		wt.Branch = strings.TrimPrefix(ref, "refs/heads/")
	} else {
		wt.Detached = true
		wt.Branch = headStr[:min(7, len(headStr))]
	}

	// Open the linked repo for status check.
	wtFS := osfs.New(wtPath)
	linkedRepo, err := r.wt.Open(wtFS)
	if err != nil {
		return nil, err
	}

	goWt, err := linkedRepo.Worktree()
	if err != nil {
		return nil, err
	}
	status, err := goWt.Status()
	if err != nil {
		return nil, err
	}
	wt.IsDirty = isDirtyIgnoringBiomelabAndBootstrap(status, wt.Path)
	wt.Sync = r.syncStatus(wt.Branch)

	return wt, nil
}

// readWorktreePath reads the gitdir file to determine the actual worktree path.
// The gitdir file contains a path like "/path/to/worktree/.git".
func readWorktreePath(wtMetaDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(wtMetaDir, "gitdir"))
	if err != nil {
		return "", err
	}
	gitdir := strings.TrimSpace(string(data))
	// The gitdir points to the .git file/dir inside the worktree.
	// The worktree path is its parent.
	return filepath.Dir(gitdir), nil
}

// readWorktreeBranch reads the per-worktree HEAD file and returns the branch
// short name (e.g. "ralph/issue-19"). Returns "" if HEAD is missing or
// detached.
func readWorktreeBranch(wtMetaDir string) string {
	data, err := os.ReadFile(filepath.Join(wtMetaDir, "HEAD"))
	if err != nil {
		return ""
	}
	headStr := strings.TrimSpace(string(data))
	ref, ok := strings.CutPrefix(headStr, "ref: ")
	if !ok {
		return ""
	}
	return strings.TrimPrefix(ref, "refs/heads/")
}

// worktreesDir returns the directory where biomelab stores linked worktrees.
// Uses .biomelab-worktrees/ in the repo root. Users must add this directory
// to their global gitignore (~/.config/git/ignore or core.excludesFile).
func (r *Repository) worktreesDir() string {
	return filepath.Join(r.repoRoot, ".biomelab-worktrees")
}

// sanitizeWorktreeName replaces path separators with dashes so the name is safe
// to use as a directory entry in .git/worktrees/ and as a branch name.
// go-git's worktree.Add rejects names that contain slashes.
func sanitizeWorktreeName(name string) string {
	return strings.ReplaceAll(name, "/", "-")
}

// CreateWorktree creates a new linked worktree with a new branch.
func (r *Repository) CreateWorktree(branchName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.reopen(); err != nil {
		return fmt.Errorf("refresh repository: %w", err)
	}
	safe := sanitizeWorktreeName(branchName)
	wtPath := filepath.Join(r.worktreesDir(), safe)
	branchRef := plumbing.NewBranchReferenceName(safe)
	if _, err := r.repo.Reference(branchRef, false); err == nil {
		return fmt.Errorf("branch %q already exists", safe)
	} else if !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return fmt.Errorf("check branch %q: %w", safe, err)
	}
	if _, err := os.Lstat(wtPath); err == nil {
		return fmt.Errorf("worktree path %q already exists", wtPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check worktree path %q: %w", wtPath, err)
	}
	metadataRoot, err := worktreeMetadataRoot(r.repoRoot)
	if err != nil {
		return fmt.Errorf("resolve worktree metadata directory: %w", err)
	}
	metadataPath := filepath.Join(metadataRoot, safe)
	if _, err := os.Lstat(metadataPath); err == nil {
		return fmt.Errorf("worktree metadata %q already exists", metadataPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check worktree metadata %q: %w", metadataPath, err)
	}
	wtFS := osfs.New(wtPath)
	if err := r.wt.Add(wtFS, safe); err != nil {
		return err
	}
	// Ensure .biomelab dir exists for new worktree so external tools can write
	// files without needing to create the directory themselves.
	_ = r.ensureBiomelabDir(wtPath)
	r.generation.Add(1)
	return nil
}

// worktreeMetadataRoot resolves the common Git directory before returning its
// worktrees directory. Repositories created with --separate-git-dir, submodule
// worktrees, and linked worktrees store a gitdir pointer in .git rather than a
// directory, so joining paths beneath <root>/.git would fail with ENOTDIR.
func worktreeMetadataRoot(repoRoot string) (string, error) {
	gitPath := filepath.Join(repoRoot, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", fmt.Errorf("stat .git: %w", err)
	}
	if info.IsDir() {
		return filepath.Join(gitPath, "worktrees"), nil
	}

	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", fmt.Errorf("read .git: %w", err)
	}
	const prefix = "gitdir:"
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf(".git file missing %q prefix", prefix)
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repoRoot, gitDir)
	}
	gitDir = filepath.Clean(gitDir)
	commonDir := gitDir
	if raw, readErr := os.ReadFile(filepath.Join(gitDir, "commondir")); readErr == nil {
		commonDir = strings.TrimSpace(string(raw))
		if !filepath.IsAbs(commonDir) {
			commonDir = filepath.Join(gitDir, commonDir)
		}
		commonDir = filepath.Clean(commonDir)
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return "", fmt.Errorf("read commondir: %w", readErr)
	}
	return filepath.Join(commonDir, "worktrees"), nil
}

// Pull fetches from all remotes and merges into the main worktree's current branch.
// This ensures repos with multiple remotes (e.g. origin, upstream) have all
// tracking refs updated. Credentials are obtained from the user's configured
// git credential helpers.
func (r *Repository) Pull() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.reopen(); err != nil {
		return err
	}

	// Fetch non-origin remotes first so their tracking refs are current.
	// Origin is left to wt.Pull() which handles fetch+merge atomically.
	remotes, err := r.repo.Remotes()
	if err != nil {
		return fmt.Errorf("listing remotes: %w", err)
	}
	for _, remote := range remotes {
		name := remote.Config().Name
		if name == gogit.DefaultRemoteName {
			continue // origin will be fetched+merged by wt.Pull() below
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		fetchOpts := &gogit.FetchOptions{RemoteName: name}
		ferr := r.repo.FetchContext(ctx, fetchOpts)
		if ferr != nil && ferr != gogit.NoErrAlreadyUpToDate && isAuthError(ferr) {
			auth, credErr := r.resolveCredentialsForRemote(remote)
			if credErr == nil {
				fetchOpts.Auth = auth
				_ = r.repo.FetchContext(ctx, fetchOpts) // best-effort
			}
		}
		cancel()
	}

	// Reopen so wt.Pull() sees fresh storer state after the non-origin fetches.
	if err := r.reopen(); err != nil {
		return err
	}

	// Pull from origin (fetch + merge).
	wt, err := r.repo.Worktree()
	if err != nil {
		return err
	}

	opts := &gogit.PullOptions{}

	// Try without auth first (public repos, SSH with agent).
	err = wt.Pull(opts)
	if err == nil || err == gogit.NoErrAlreadyUpToDate {
		return nil
	}

	// If auth is required, resolve credentials from git credential helpers.
	if isAuthError(err) {
		auth, credErr := r.resolveCredentials()
		if credErr != nil {
			return fmt.Errorf("authentication required but credential lookup failed: %w", credErr)
		}
		opts.Auth = auth
		err = wt.Pull(opts)
		if err == gogit.NoErrAlreadyUpToDate {
			return nil
		}
		return err
	}

	return err
}

func isAuthError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "authentication required") ||
		strings.Contains(msg, "401") ||
		strings.Contains(msg, "403")
}

func (r *Repository) resolveCredentials() (*githttp.BasicAuth, error) {
	remotes, err := r.repo.Remotes()
	if err != nil || len(remotes) == 0 {
		return nil, fmt.Errorf("no remotes configured")
	}
	return r.resolveCredentialsForRemote(remotes[0])
}

func (r *Repository) resolveCredentialsForRemote(remote *gogit.Remote) (*githttp.BasicAuth, error) {
	return r.resolveCredentialsForRemoteContext(context.Background(), remote)
}

func (r *Repository) resolveCredentialsForRemoteContext(ctx context.Context, remote *gogit.Remote) (*githttp.BasicAuth, error) {
	urls := remote.Config().URLs
	if len(urls) == 0 {
		return nil, fmt.Errorf("remote %q has no URLs", remote.Config().Name)
	}
	return credentialFillContext(ctx, urls[0])
}

// FetchPRRef fetches a pull request's head ref to a local branch without
// creating a worktree. FetchPR builds on it; kept exported so callers can
// fetch a ref and place the worktree themselves.
func (r *Repository) FetchPRRef(prNumber int, branchName, remoteURL string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.reopen(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	refSpec := config.RefSpec(fmt.Sprintf("+refs/pull/%d/head:refs/heads/%s", prNumber, branchName))
	opts := &gogit.FetchOptions{
		RefSpecs: []config.RefSpec{refSpec},
	}
	if remoteURL != "" {
		opts.RemoteURL = remoteURL
	}

	err := r.repo.FetchContext(ctx, opts)
	if err != nil && err != gogit.NoErrAlreadyUpToDate {
		if isAuthError(err) {
			auth, credErr := r.resolveCredentials()
			if credErr != nil {
				return fmt.Errorf("fetch PR auth: %w", credErr)
			}
			opts.Auth = auth
			err = r.repo.FetchContext(ctx, opts)
		}
		if err != nil && err != gogit.NoErrAlreadyUpToDate {
			return fmt.Errorf("fetch PR ref: %w", err)
		}
	}

	return nil
}

// FetchPR fetches a pull request's head ref and creates a worktree for it.
// Delegates ref fetching to FetchPRRef, then creates a worktree on the host.
// Returns the path of the created worktree.
func (r *Repository) FetchPR(prNumber int, branchName, remoteURL string) (string, error) {
	if err := r.FetchPRRef(prNumber, branchName, remoteURL); err != nil {
		return "", err
	}

	// Take the lock around the worktree creation + generation bump so a
	// concurrent snapshot sees the new dir and the new gen as one event.
	// Without this, a refresh that snapshots between cmd success and bump
	// would carry a list with the new worktree under a pre-mutation gen,
	// and could be incorrectly dropped as stale later.
	r.mu.Lock()
	defer r.mu.Unlock()

	// Create the worktree using the git CLI.
	// We sanitize slashes from the branch name only for the directory path;
	// the local branch ref itself keeps the original name (e.g. "ralph/issue-19").
	// git worktree add derives the .git/worktrees/<name> key from the directory
	// basename, so no slashes end up there either.
	if err := os.MkdirAll(r.worktreesDir(), 0o755); err != nil {
		return "", fmt.Errorf("create worktrees dir: %w", err)
	}
	safe := sanitizeWorktreeName(branchName)
	wtPath := filepath.Join(r.worktreesDir(), safe)
	cmd := command.Background("git", "worktree", "add", wtPath, branchName)
	cmd.Dir = r.repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("create worktree: %w: %s", err, strings.TrimSpace(string(out)))
	}

	r.generation.Add(1)
	return wtPath, nil
}

// RemoveWorktree fully removes a linked worktree: removes the worktree directory,
// removes the worktree metadata, deletes the branch, and prunes stale entries.
//
// name is the worktree identifier (the directory entry under .git/worktrees/),
// not the branch name — they differ for branches that contain slashes (the
// worktree name is the sanitized form). Pass Worktree.Name from ListWorktrees.
func (r *Repository) RemoveWorktree(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Read the worktree path and branch name before removing metadata.
	// The branch name read from HEAD may differ from `name` for branches
	// with slashes (worktree name is sanitized, branch ref keeps slashes).
	wtMetaDir := filepath.Join(r.repoRoot, ".git", "worktrees", name)
	if _, err := os.Stat(wtMetaDir); err != nil {
		return fmt.Errorf("worktree %q not found: %w", name, err)
	}
	wtPath, err := readWorktreePath(wtMetaDir)
	if err != nil {
		// Fallback: assume biomelab-worktrees directory.
		wtPath = filepath.Join(r.worktreesDir(), name)
	}
	branchName := readWorktreeBranch(wtMetaDir)
	if branchName == "" {
		branchName = name
	}

	// Remove the worktree directory from disk.
	if err := os.RemoveAll(wtPath); err != nil {
		return fmt.Errorf("remove worktree directory: %w", err)
	}

	// Remove the worktree metadata directory directly.
	// We don't use r.wt.Remove() because it rejects names with slashes
	// (e.g., "ralph/issue-1456") due to its name regex.
	if err := os.RemoveAll(wtMetaDir); err != nil {
		return fmt.Errorf("remove worktree metadata: %w", err)
	}

	// Delete the local branch (config may not exist; ignore errors).
	_ = r.repo.DeleteBranch(branchName)
	// Also delete the branch reference itself (may not exist; ignore errors).
	refName := plumbing.NewBranchReferenceName(branchName)
	_ = r.repo.Storer.RemoveReference(refName)

	// Prune stale worktree entries by removing any metadata dirs
	// whose gitdir points to a non-existent path.
	r.pruneWorktrees()

	r.generation.Add(1)
	return nil
}

// pruneWorktrees removes worktree metadata entries whose working directories no longer exist.
func (r *Repository) pruneWorktrees() {
	worktreesDir := filepath.Join(r.repoRoot, ".git", "worktrees")
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metaDir := filepath.Join(worktreesDir, entry.Name())
		wtPath, err := readWorktreePath(metaDir)
		if err != nil {
			continue
		}
		if _, err := os.Stat(wtPath); os.IsNotExist(err) {
			_ = os.RemoveAll(metaDir)
		}
	}
}

// RemoteInfo holds the name and URL of a git remote.
type RemoteInfo struct {
	Name string // remote name (e.g. "origin")
	URL  string // first URL of the remote
	Repo string // parsed "owner/repo" from URL, or empty
}

// ListRemotes returns information about all configured remotes.
func (r *Repository) ListRemotes() ([]RemoteInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.reopen(); err != nil {
		return nil, err
	}

	remotes, err := r.repo.Remotes()
	if err != nil {
		return nil, fmt.Errorf("listing remotes: %w", err)
	}

	var result []RemoteInfo
	for _, remote := range remotes {
		cfg := remote.Config()
		ri := RemoteInfo{Name: cfg.Name}
		if len(cfg.URLs) > 0 {
			ri.URL = cfg.URLs[0]
			ri.Repo = parseRepoName(cfg.URLs[0])
		}
		result = append(result, ri)
	}
	return result, nil
}

// Push pushes a local branch to a remote and sets the upstream tracking branch.
// Uses the same auth-retry pattern as Fetch and Pull: tries without credentials
// first, then resolves via git credential helpers on auth failure.
// After a successful push, configures branch.<name>.remote and branch.<name>.merge
// so that subsequent git/gh operations know the tracking branch (equivalent to
// git push --set-upstream).
func (r *Repository) Push(remoteName, branchName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.reopen(); err != nil {
		return err
	}

	remote, err := r.repo.Remote(remoteName)
	if err != nil {
		return fmt.Errorf("remote %q: %w", remoteName, err)
	}

	refSpec := config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", branchName, branchName))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := &gogit.PushOptions{
		RemoteName: remoteName,
		RefSpecs:   []config.RefSpec{refSpec},
	}

	err = remote.PushContext(ctx, opts)
	if err != nil && err != gogit.NoErrAlreadyUpToDate {
		if isAuthError(err) {
			auth, credErr := r.resolveCredentialsForRemote(remote)
			if credErr != nil {
				return fmt.Errorf("push auth: %w", credErr)
			}
			opts.Auth = auth
			err = remote.PushContext(ctx, opts)
			if err != nil && err != gogit.NoErrAlreadyUpToDate {
				return fmt.Errorf("push %s to %s: %w", branchName, remoteName, err)
			}
		} else {
			return fmt.Errorf("push %s to %s: %w", branchName, remoteName, err)
		}
	}

	// Set upstream tracking branch (equivalent to --set-upstream).
	r.setUpstreamTracking(remoteName, branchName)

	return nil
}

// setUpstreamTracking configures the branch to track the remote branch.
// This is equivalent to what "git push --set-upstream" does:
//
//	[branch "<name>"]
//	    remote = <remoteName>
//	    merge = refs/heads/<name>
func (r *Repository) setUpstreamTracking(remoteName, branchName string) {
	cfg, err := r.repo.Config()
	if err != nil {
		return
	}
	cfg.Branches[branchName] = &config.Branch{
		Name:   branchName,
		Remote: remoteName,
		Merge:  plumbing.NewBranchReferenceName(branchName),
	}
	_ = r.repo.Storer.SetConfig(cfg)
}

// HasStash returns true if the repository has any stash entries.
func (r *Repository) HasStash() (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.reopen(); err != nil {
		return false, err
	}

	_, err := r.repo.Reference(plumbing.ReferenceName("refs/stash"), false)
	if err != nil {
		return false, nil // no stash ref means no stash entries
	}
	return true, nil
}

// ensureBiomelabDir ensures the .biomelab directory exists in the given
// worktree path and that it's git-excluded. External tools (pr-scribe,
// note editor) write files inside without needing to create the directory
// themselves, and the entries never show up in `git status`.
// Errors are silently ignored as this is a best-effort initialization.
func (r *Repository) ensureBiomelabDir(wtPath string) error {
	if err := os.MkdirAll(filepath.Join(wtPath, biomelabDir), 0o755); err != nil {
		return err
	}
	return EnsureExcluded(wtPath, biomelabExcludeLine)
}

// MigrateWorktreeDirs ensures .biomelab exists for all worktrees in the
// repository (migration for older projects created before this feature).
// This ensures external tools like pr-scribe can write files to the directory.
// Errors are silently ignored as this is a best-effort background task.
func (r *Repository) MigrateWorktreeDirs() {
	wts, err := r.ListWorktrees()
	if err != nil {
		return
	}
	for _, wt := range wts {
		_ = r.ensureBiomelabDir(wt.Path)
	}
}

// RepoRoot returns the root directory of the main worktree containing path.
// If path lies inside a linked worktree (one created with `git worktree add`),
// the linked worktree's root is resolved to the main checkout that owns it,
// so callers never register a linked worktree as a repository.
func RepoRoot(path string) (string, error) {
	r, err := gogit.PlainOpenWithOptions(path, &gogit.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		return "", err
	}
	wt, err := r.Worktree()
	if err != nil {
		return "", err
	}
	root := wt.Filesystem.Root()
	if mainRoot, ok := mainWorktreeRoot(root); ok {
		return mainRoot, nil
	}
	return root, nil
}

// mainWorktreeRoot returns the main checkout's root when root is a linked
// worktree, detected by a `.git` *file* whose gitdir points at
// <main>/.git/worktrees/<name>. The owning repository is located through the
// worktree's `commondir` file (relative to the gitdir), falling back to
// stripping the trailing worktrees/<name> segments. ok=false when root is
// not a linked worktree or the layout is unrecognised.
func mainWorktreeRoot(root string) (string, bool) {
	dotGit := filepath.Join(root, ".git")
	info, err := os.Stat(dotGit)
	if err != nil || info.IsDir() {
		return "", false
	}
	data, err := os.ReadFile(dotGit)
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(string(data))
	gitdir, ok := strings.CutPrefix(line, "gitdir:")
	if !ok {
		return "", false
	}
	gitdir = strings.TrimSpace(gitdir)
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(root, gitdir)
	}
	gitdir = filepath.Clean(gitdir)

	var commonGit string
	if cd, err := os.ReadFile(filepath.Join(gitdir, "commondir")); err == nil {
		rel := strings.TrimSpace(string(cd))
		if filepath.IsAbs(rel) {
			commonGit = filepath.Clean(rel)
		} else {
			commonGit = filepath.Clean(filepath.Join(gitdir, rel))
		}
	} else if filepath.Base(filepath.Dir(gitdir)) == "worktrees" {
		commonGit = filepath.Dir(filepath.Dir(gitdir))
	} else {
		return "", false
	}
	if filepath.Base(commonGit) != ".git" {
		return "", false
	}
	return filepath.Dir(commonGit), true
}
