package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-billy/v6/osfs"
	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	githttp "github.com/go-git/go-git/v6/plumbing/transport/http"
	xworktree "github.com/go-git/go-git/v6/x/plumbing/worktree"
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
type Worktree struct {
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
// Falls back to the directory name if no remote is configured.
func (r *Repository) RepoName() string {
	if url := r.OriginURL(); url != "" {
		if name := parseRepoName(url); name != "" {
			return name
		}
	}
	return filepath.Base(r.repoRoot)
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
		name := remote.Config().Name
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)

		opts := &gogit.FetchOptions{RemoteName: name}
		err := r.repo.FetchContext(ctx, opts)
		if err != nil && err != gogit.NoErrAlreadyUpToDate {
			if isAuthError(err) {
				auth, credErr := r.resolveCredentialsForRemote(remote)
				if credErr == nil {
					opts.Auth = auth
					err = r.repo.FetchContext(ctx, opts)
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

// ListWorktreesQuick returns worktrees with only branch info — no dirty/sync checks.
// Used for fast initial render.
func (r *Repository) ListWorktreesQuick() ([]Worktree, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.reopen(); err != nil {
		return nil, err
	}

	var result []Worktree

	// Main worktree — branch only.
	head, err := r.repo.Head()
	if err != nil {
		return nil, err
	}
	mainWt := Worktree{Path: r.repoRoot, IsMain: true}
	if head.Name().IsBranch() {
		mainWt.Branch = head.Name().Short()
	} else {
		mainWt.Detached = true
		mainWt.Branch = head.Hash().String()[:7]
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
		wt := Worktree{Path: wtPath}
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
	head, err := r.repo.Head()
	if err != nil {
		return nil, err
	}

	wt := &Worktree{
		Path:   r.repoRoot,
		IsMain: true,
	}

	if head.Name().IsBranch() {
		wt.Branch = head.Name().Short()
	} else {
		wt.Detached = true
		wt.Branch = head.Hash().String()[:7]
	}

	goWt, err := r.repo.Worktree()
	if err != nil {
		return nil, err
	}
	status, err := goWt.Status()
	if err != nil {
		return nil, err
	}
	wt.IsDirty = !status.IsClean()
	wt.Sync = r.syncStatus(wt.Branch)

	return wt, nil
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
		// Fallback: assume gwaim-worktrees directory.
		wtPath = filepath.Join(r.worktreesDir(), name)
	}

	wt := &Worktree{
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
	wt.IsDirty = !status.IsClean()
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

// worktreesDir returns the directory where gwaim stores linked worktrees.
// Uses .gwaim-worktrees/ in the repo root. Users must add this directory
// to their global gitignore (~/.config/git/ignore or core.excludesFile).
func (r *Repository) worktreesDir() string {
	return filepath.Join(r.repoRoot, ".gwaim-worktrees")
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
	safe := sanitizeWorktreeName(branchName)
	wtPath := filepath.Join(r.worktreesDir(), safe)
	wtFS := osfs.New(wtPath)
	return r.wt.Add(wtFS, safe)
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

// Repair runs "git worktree repair" to fix broken worktree links.
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
	urls := remote.Config().URLs
	if len(urls) == 0 {
		return nil, fmt.Errorf("remote %q has no URLs", remote.Config().Name)
	}
	return credentialFill(urls[0])
}

// FetchPR fetches a GitHub pull request's head ref and creates a worktree for it.
// The PR ref (refs/pull/<N>/head) is fetched to a local branch named branchName.
// If remoteURL is non-empty, it is used as the fetch URL (for fork PRs);
// otherwise the default origin remote is used.
// Returns the path of the created worktree.
func (r *Repository) FetchPR(prNumber int, branchName, remoteURL string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.reopen(); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Fetch refs/pull/<N>/head to refs/heads/<branchName>.
	// Keep the original branch name (slashes allowed in git refs).
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
				return "", fmt.Errorf("fetch PR auth: %w", credErr)
			}
			opts.Auth = auth
			err = r.repo.FetchContext(ctx, opts)
		}
		if err != nil && err != gogit.NoErrAlreadyUpToDate {
			return "", fmt.Errorf("fetch PR ref: %w", err)
		}
	}

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
	cmd := exec.Command("git", "worktree", "add", wtPath, branchName)
	cmd.Dir = r.repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("create worktree: %w: %s", err, strings.TrimSpace(string(out)))
	}

	return wtPath, nil
}

// RemoveWorktree fully removes a linked worktree: removes the worktree directory,
// removes the worktree metadata, deletes the branch, and prunes stale entries.
func (r *Repository) RemoveWorktree(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Read the worktree path before removing metadata.
	wtMetaDir := filepath.Join(r.repoRoot, ".git", "worktrees", name)
	wtPath, err := readWorktreePath(wtMetaDir)
	if err != nil {
		// Fallback: assume gwaim-worktrees directory.
		wtPath = filepath.Join(r.worktreesDir(), name)
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
	_ = r.repo.DeleteBranch(name)
	// Also delete the branch reference itself (may not exist; ignore errors).
	refName := plumbing.NewBranchReferenceName(name)
	_ = r.repo.Storer.RemoveReference(refName)

	// Prune stale worktree entries by removing any metadata dirs
	// whose gitdir points to a non-existent path.
	r.pruneWorktrees()

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

// RepoRoot finds the root of the git repository containing the given path.
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

	return wt.Filesystem.Root(), nil
}

