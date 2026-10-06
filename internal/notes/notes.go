// Package notes manages per-worktree scratchpad notes stored as markdown.
//
// A note for a worktree lives at <worktreeDir>/.biomelab/note.md and is kept
// invisible to git via the per-worktree info/exclude file, so it never shows
// up in "git status" and cannot leak into a commit by accident. Notes are
// inside the worktree directory on purpose: the sandbox mounts the worktree,
// so anything stored here is reachable by an agent running in the microVM.
package notes

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mdelapenya/biomelab/internal/git"
)

const (
	noteDir     = ".biomelab"
	noteFile    = "note.md"
	prTitleFile = "pr-title.md"
	excludeLine = "/.biomelab/"
	dirPerm     = 0o755
	filePerm    = 0o644
)

// Path returns the absolute path to the note file for a worktree directory.
func Path(worktreeDir string) string {
	return filepath.Join(worktreeDir, noteDir, noteFile)
}

// TitlePath returns the absolute path to the PR title file for a worktree.
// External tools (e.g. the pr-scribe skill) write a single-line Conventional
// Commits title here; biomelab uses its content as `--title` when the user
// opts to include task notes during the PR send flow.
func TitlePath(worktreeDir string) string {
	return filepath.Join(worktreeDir, noteDir, prTitleFile)
}

// WriteTitle saves a single-line PR title to the title file. Embedded
// newlines are collapsed to spaces and surrounding whitespace is trimmed,
// so the on-disk file is always one line + trailing newline. An empty
// (post-trim) input is treated as a delete. The .biomelab/ exclude entry
// is ensured here too so the title file never leaks into git status.
func WriteTitle(worktreeDir, title string) error {
	// Collapse any whitespace runs (including embedded newlines and tabs)
	// into single spaces so the on-disk file is always one clean line.
	title = strings.Join(strings.Fields(title), " ")
	if title == "" {
		return DeleteTitle(worktreeDir)
	}
	dir := filepath.Join(worktreeDir, noteDir)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("create note dir: %w", err)
	}
	if err := ensureExcluded(worktreeDir); err != nil {
		return fmt.Errorf("ensure excluded: %w", err)
	}
	if err := os.WriteFile(TitlePath(worktreeDir), []byte(title+"\n"), filePerm); err != nil {
		return fmt.Errorf("write title: %w", err)
	}
	return nil
}

// DeleteTitle removes the PR title file. Returns nil if the file doesn't
// exist.
func DeleteTitle(worktreeDir string) error {
	err := os.Remove(TitlePath(worktreeDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// ReadTitle returns the trimmed first non-empty line of the PR title file.
// ok is false when the file does not exist or contains only whitespace; in
// either case title is "" and err is nil. Only the first line is honored
// because Git/CLI title arguments are single-line by design.
func ReadTitle(worktreeDir string) (title string, ok bool, err error) {
	data, err := os.ReadFile(TitlePath(worktreeDir))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", false, nil
	}
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false, nil
	}
	return text, true, nil
}

// EnsureDir creates the .biomelab directory for the worktree if it doesn't
// exist, and ensures it's added to the worktree's info/exclude. This allows
// other tools to write files into the directory (e.g. pr-title.md, future
// metadata files) without needing to create the directory themselves.
func EnsureDir(worktreeDir string) error {
	dir := filepath.Join(worktreeDir, noteDir)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("create note dir: %w", err)
	}
	if err := ensureExcluded(worktreeDir); err != nil {
		return fmt.Errorf("ensure excluded: %w", err)
	}
	return nil
}

// Exists reports whether a note file exists for the given worktree.
func Exists(worktreeDir string) bool {
	_, err := os.Stat(Path(worktreeDir))
	return err == nil
}

// Read returns the note content. The bool is false when no note file exists,
// in which case content is empty and err is nil.
func Read(worktreeDir string) (string, bool, error) {
	data, err := os.ReadFile(Path(worktreeDir))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return string(data), true, nil
}

// Write saves the note and ensures the worktree's info/exclude file ignores
// the .biomelab/ directory. Trailing whitespace is stripped and a single
// trailing newline is appended so accidental Enter-mashing at the end of
// the editor doesn't leave blank lines on disk. An all-whitespace content
// is treated as a delete.
func Write(worktreeDir, content string) error {
	content = strings.TrimRight(content, " \t\r\n")
	if content == "" {
		return Delete(worktreeDir)
	}
	content += "\n"
	dir := filepath.Join(worktreeDir, noteDir)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("create note dir: %w", err)
	}
	if err := ensureExcluded(worktreeDir); err != nil {
		return fmt.Errorf("ensure excluded: %w", err)
	}
	if err := os.WriteFile(Path(worktreeDir), []byte(content), filePerm); err != nil {
		return fmt.Errorf("write note: %w", err)
	}
	return nil
}

// Delete removes the note file. Returns nil if the note doesn't exist.
func Delete(worktreeDir string) error {
	err := os.Remove(Path(worktreeDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// ensureExcluded appends excludeLine to the worktree's info/exclude.
// Thin wrapper around git.EnsureExcluded so the notes package's call
// sites stay terse.
func ensureExcluded(worktreeDir string) error {
	return git.EnsureExcluded(worktreeDir, excludeLine)
}
