package notes

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mdelapenya/biomelab/internal/git"
)

const (
	issueFile    = "issue.md"
	progressFile = "progress.md"
)

const initialProgressTemplate = `# Progress

## Completed work

No progress recorded yet. Inspect current code and Git state before planning; do not assume the issue is untouched.

## Decisions

## Remaining tasks or blockers

## Validation

## Current commit and uncommitted state
`

// IssuePath returns the path to the immutable issue requirements snapshot.
func IssuePath(worktreeDir string) string {
	return filepath.Join(worktreeDir, noteDir, issueFile)
}

// ProgressPath returns the path to the agent-maintained progress handoff.
func ProgressPath(worktreeDir string) string {
	return filepath.Join(worktreeDir, noteDir, progressFile)
}

// InitializeIssueContext creates the issue snapshot and initial progress
// handoff. Existing regular files are deliberately preserved so a retry never
// discards an agent's changes. Symlinks and non-regular targets are rejected.
func InitializeIssueContext(worktreeDir, issueMarkdown string) error {
	root, err := filepath.Abs(worktreeDir)
	if err != nil {
		return fmt.Errorf("resolve worktree path: %w", err)
	}
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("stat worktree: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("worktree path is not a regular directory")
	}

	dir := filepath.Join(root, noteDir)
	if err := ensureContextDir(dir); err != nil {
		return err
	}
	// Write the exclusion first: no context file should be left visible to
	// Git if creation subsequently succeeds.
	if err := git.EnsureExcluded(root, excludeLine); err != nil {
		return fmt.Errorf("ensure excluded: %w", err)
	}
	if err := createContextFile(IssuePath(root), []byte(issueMarkdown)); err != nil {
		return fmt.Errorf("create issue context: %w", err)
	}
	if err := createContextFile(ProgressPath(root), []byte(initialProgressTemplate)); err != nil {
		return fmt.Errorf("create progress context: %w", err)
	}
	return nil
}

func ensureContextDir(dir string) error {
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(dir, dirPerm); err != nil {
			return fmt.Errorf("create context directory: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect context directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("context directory is not a regular directory")
	}
	return nil
}

func createContextFile(path string, content []byte) error {
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("context path is not a regular file")
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect context file: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, filePerm)
	if err != nil {
		return err
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		return fmt.Errorf("write: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	return nil
}
