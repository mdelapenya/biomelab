package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v6/osfs"
	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/cache"
	githttp "github.com/go-git/go-git/v6/plumbing/transport/http"
	"github.com/go-git/go-git/v6/storage/filesystem"
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

func (s SyncStatus) String() string {
	switch s {
	case SyncUpToDate:
		return "up-to-date"
	case SyncAhead:
		return "ahead"
	case SyncBehind:
		return "behind"
	case SyncDiverged:
		return "diverged"
	case SyncNoUpstream:
		return "no upstream"
	default:
		return ""
	}
}

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

// RepoName returns the "owner/repo" name derived from the origin remote URL.
// Falls back to the directory name if no remote is configured.
func (r *Repository) RepoName() string {
	remotes, err := r.repo.Remotes()
	if err == nil && len(remotes) > 0 {
		urls := remotes[0].Config().URLs
		if len(urls) > 0 {
			if name := parseRepoName(urls[0]); name != "" {
				return name
			}
		}
	}
	return filepath.Base(r.repoRoot)
}

// parseRepoName extracts "owner/repo" from a git remote URL.
func parseRepoName(remoteURL string) string {
	// Handle SSH: git@github.com:owner/repo.git
	if idx := strings.Index(remoteURL, ":"); idx > 0 && !strings.Contains(remoteURL[:idx], "/") {
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
func (r *Repository) Fetch() error {
	opts := &gogit.FetchOptions{}

	err := r.repo.Fetch(opts)
	if err == nil || err == gogit.NoErrAlreadyUpToDate {
		return nil
	}

	// If auth is required, resolve credentials from git credential helpers.
	if isAuthError(err) {
		auth, credErr := r.resolveCredentials()
		if credErr != nil {
			return fmt.Errorf("fetch auth: %w", credErr)
		}
		opts.Auth = auth
		err = r.repo.Fetch(opts)
		if err == gogit.NoErrAlreadyUpToDate {
			return nil
		}
		return err
	}

	return err
}

// ListWorktrees returns all worktrees (main + linked) with their metadata.
func (r *Repository) ListWorktrees() ([]Worktree, error) {
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

// syncStatus compares the local branch commit with the remote tracking branch (origin/<branch>).
func (r *Repository) syncStatus(branch string) SyncStatus {
	if branch == "" {
		return SyncUnknown
	}

	localRef, err := r.repo.Reference(plumbing.NewBranchReferenceName(branch), true)
	if err != nil {
		return SyncUnknown
	}

	remoteRef, err := r.repo.Reference(plumbing.NewRemoteReferenceName("origin", branch), true)
	if err != nil {
		return SyncNoUpstream
	}

	localHash := localRef.Hash()
	remoteHash := remoteRef.Hash()

	if localHash == remoteHash {
		return SyncUpToDate
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

// linkedWorktree returns info about a linked worktree by name.
func (r *Repository) linkedWorktree(name string) (*Worktree, error) {
	wtMetaDir := filepath.Join(r.repoRoot, ".git", "worktrees", name)

	// Read the actual worktree path from the gitdir file.
	// The gitdir file contains the path to the worktree's .git file.
	wtPath, err := readWorktreePath(wtMetaDir)
	if err != nil {
		// Fallback: assume sibling directory.
		wtPath = filepath.Join(filepath.Dir(r.repoRoot), name)
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

// CreateWorktree creates a new linked worktree with a new branch.
func (r *Repository) CreateWorktree(branchName string) error {
	wtPath := filepath.Join(filepath.Dir(r.repoRoot), branchName)
	wtFS := osfs.New(wtPath)
	return r.wt.Add(wtFS, branchName)
}

// CreateWorktreeAtCommit creates a new linked worktree at a specific commit.
func (r *Repository) CreateWorktreeAtCommit(branchName string, commit plumbing.Hash) error {
	wtPath := filepath.Join(filepath.Dir(r.repoRoot), branchName)
	wtFS := osfs.New(wtPath)
	return r.wt.Add(wtFS, branchName, xworktree.WithCommit(commit))
}

// Pull fetches from the remote and merges into the main worktree's current branch.
// Credentials are obtained from the user's configured git credential helpers.
func (r *Repository) Pull() error {
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

	urls := remotes[0].Config().URLs
	if len(urls) == 0 {
		return nil, fmt.Errorf("remote has no URLs")
	}

	return credentialFill(urls[0])
}

// RemoveWorktree fully removes a linked worktree: removes the worktree directory,
// removes the worktree metadata, deletes the branch, and prunes stale entries.
func (r *Repository) RemoveWorktree(name string) error {
	// Read the worktree path before removing metadata.
	wtMetaDir := filepath.Join(r.repoRoot, ".git", "worktrees", name)
	wtPath, err := readWorktreePath(wtMetaDir)
	if err != nil {
		// Fallback: assume sibling directory.
		wtPath = filepath.Join(filepath.Dir(r.repoRoot), name)
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

// WorktreePaths returns all worktree absolute paths for agent matching.
func (r *Repository) WorktreePaths() ([]string, error) {
	wts, err := r.ListWorktrees()
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(wts))
	for i, wt := range wts {
		paths[i] = wt.Path
	}
	return paths, nil
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

// NewStorageFromPath creates a filesystem storage for a repo path, useful for testing.
func NewStorageFromPath(path string) *filesystem.Storage {
	fs := osfs.New(path)
	dotgitFS, _ := fs.Chroot(".git")
	return filesystem.NewStorage(dotgitFS, cache.NewObjectLRUDefault())
}
