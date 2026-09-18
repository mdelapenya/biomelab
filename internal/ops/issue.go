package ops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/github"
	"github.com/mdelapenya/biomelab/internal/notes"
)

const (
	maxIssueBranchLength = 100
	maxIssueSlugLength   = 60
)

// CreateIssueWorktreeResult reports worktree creation separately from saving
// its issue context. A non-empty WtPath with NotesErr means creation succeeded
// and the worktree was deliberately preserved for repair.
type CreateIssueWorktreeResult struct {
	BranchName string
	WtPath     string
	Err        error
	NotesErr   error
}

// SuggestedIssueBranch returns a safe, deterministic branch name for an issue.
func SuggestedIssueBranch(issue github.IssueInfo) string {
	prefix := "issue-" + strconv.Itoa(issue.Number)
	var slug strings.Builder
	separator := false
	for _, r := range issue.Title {
		switch {
		case r >= 'A' && r <= 'Z':
			if separator && slug.Len() > 0 && slug.Len() < maxIssueSlugLength {
				slug.WriteByte('-')
			}
			separator = false
			if slug.Len() < maxIssueSlugLength {
				slug.WriteByte(byte(r + ('a' - 'A')))
			}
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if separator && slug.Len() > 0 && slug.Len() < maxIssueSlugLength {
				slug.WriteByte('-')
			}
			separator = false
			if slug.Len() < maxIssueSlugLength {
				slug.WriteByte(byte(r))
			}
		default:
			separator = slug.Len() > 0
		}
	}
	s := strings.TrimRight(slug.String(), "-")
	if s == "" {
		return prefix
	}
	return prefix + "-" + s
}

// ValidateIssueBranch checks branch names accepted by the issue workflow.
func ValidateIssueBranch(branch string) error {
	if branch == "" {
		return errors.New("branch name is required")
	}
	if len(branch) > maxIssueBranchLength {
		return fmt.Errorf("branch name must be at most %d characters", maxIssueBranchLength)
	}
	if branch == "HEAD" {
		return errors.New("branch name HEAD is reserved by Git")
	}
	for i := 0; i < len(branch); i++ {
		c := branch[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			continue
		}
		if c == '-' && i > 0 {
			continue
		}
		return errors.New("branch name must start with a letter or digit and contain only letters, digits, and hyphens")
	}
	return nil
}

// CreateWorktreeFromIssue creates a normal local-HEAD worktree and seeds its
// editable task notes. Note failures never remove the successfully-created
// worktree.
func CreateWorktreeFromIssue(repo *git.Repository, issue github.IssueInfo, branch string) CreateIssueWorktreeResult {
	result := CreateIssueWorktreeResult{BranchName: branch}
	if err := ValidateIssueBranch(branch); err != nil {
		result.Err = err
		return result
	}
	created := CreateWorktree(repo, branch)
	if created.Err != nil {
		result.Err = created.Err
		return result
	}
	result.WtPath = worktreePathForBranch(repo, branch)
	if result.WtPath == "" {
		result.WtPath = filepath.Join(repo.Root(), ".biomelab-worktrees", branch)
		result.NotesErr = errors.New("worktree was created but its path could not be confirmed")
	}
	notesErr := writeIssueNotes(result.WtPath, issue)
	result.NotesErr = errors.Join(result.NotesErr, notesErr)
	if notesErr == nil {
		result.NotesErr = errors.Join(result.NotesErr, notes.EnsureAgentBootstrap(result.WtPath))
	}
	return result
}

func writeIssueNotes(wtPath string, issue github.IssueInfo) error {
	noteDir := filepath.Dir(notes.Path(wtPath))
	if info, err := os.Lstat(noteDir); err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
		return fmt.Errorf("notes conflict: %s is not a regular directory", noteDir)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect notes directory: %w", err)
	}
	// Check every artifact before creating any of them. A checked-out context
	// file belongs to the repository and must not be overwritten by issue setup.
	for _, path := range []string{notes.TitlePath(wtPath), notes.Path(wtPath), notes.IssuePath(wtPath), notes.ProgressPath(wtPath)} {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("notes conflict: %s already exists", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect note artifact %s: %w", path, err)
		}
	}

	body := fmt.Sprintf("# Issue %d: %s\n\nSource: %s", issue.Number, issue.Title, issue.URL)
	if issue.Body != "" {
		body += "\n\n" + issue.Body
	}
	var errs []error
	if err := notes.WriteTitle(wtPath, issue.Title); err != nil {
		errs = append(errs, fmt.Errorf("save issue title: %w", err))
	}
	if err := notes.Write(wtPath, body); err != nil {
		errs = append(errs, fmt.Errorf("save issue note: %w", err))
	}
	if len(errs) == 0 {
		if err := notes.InitializeIssueContext(wtPath, body); err != nil {
			errs = append(errs, fmt.Errorf("initialize issue context: %w", err))
		}
	}
	return errors.Join(errs...)
}
