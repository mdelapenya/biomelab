package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mdelapenya/biomelab/internal/process"
	ps "github.com/shirou/gopsutil/v4/process"
)

// Session identifies the host shell occupying a terminal. Its PID and birth
// time survive exec and directory changes, including a foreground sbx command.
// One caller at a time may Resolve/Activate a Session (the GUI's per-target guard).
type Session struct {
	info          Info
	born          int64
	tty, windowID string
	markerDir     string // immutable; retained only while the handshake is pending
	ready         bool
	started       time.Time
	claimedPID    atomic.Int32 // published identity for discovery from other cards
}

var errSessionStarting = errors.New("terminal is still starting or did not register its session; switch to it manually and try again")

// OpenTracked starts a terminal and records the host identity before any remote
// sandbox command runs. A non-nil session must be retained even when an error is
// returned: the terminal may have opened, so retrying must not blindly open more.
func OpenTracked(dir, command, identifier string) (*Session, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("terminal launch on Windows is not supported yet; open a terminal in the worktree manually")
	}
	base, err := buildShellCmdWithTitle(dir, command, identifier)
	if err != nil {
		return nil, err
	}
	return openTracked(base, openRaw)
}

func openTracked(base string, launch func(string) error) (*Session, error) {
	dir, err := os.MkdirTemp("", "biomelab-session-")
	if err != nil {
		return nil, err
	}
	session := &Session{markerDir: dir, started: time.Now()}
	if err := launch(sessionPrelude(dir) + base); err != nil {
		session.Cleanup()
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := session.resolve(ctx)
		if err == nil {
			return session, nil
		}
		if !errors.Is(err, errSessionStarting) {
			return session, err
		}
		select {
		case <-ctx.Done():
			return session, errSessionStarting
		case <-ticker.C:
		}
	}
}

func sessionPrelude(dir string) string {
	// A subshell isolates umask; POSIX $$ still names the host parent shell.
	// Atomic rename prevents the GUI from seeing a partial record. Values are
	// printf arguments, not code, and the destination is private to this launch.
	return "(umask 077; printf '%s\\n%s\\n%s\\n%s\\n' \"$$\" \"$(tty 2>/dev/null)\" \"${WINDOWID-}\" \"${TERM_PROGRAM-}\" > " +
		shellQuote(filepath.Join(dir, "pending")) + " && mv " + shellQuote(filepath.Join(dir, "pending")) + " " + shellQuote(filepath.Join(dir, "session")) + "); "
}

// Cleanup removes handshake files only; it never terminates the user's shell.
// markerDir is immutable, so cleanup is also safe during application shutdown.
func (s *Session) Cleanup() {
	if s != nil && s.markerDir != "" {
		_ = os.RemoveAll(s.markerDir)
	}
}

// RecoveryNeeded lets the UI offer an explicit reset after a failed handshake.
// It never authorizes an automatic replacement: a terminal may still be open.
func (s *Session) RecoveryNeeded() bool {
	return s != nil && !s.ready && !s.started.IsZero() && time.Since(s.started) > 30*time.Second
}

func (s *Session) resolve(ctx context.Context) error {
	if s.ready {
		return nil
	}
	f, err := os.Open(filepath.Join(s.markerDir, "session"))
	if os.IsNotExist(err) {
		return errSessionStarting
	}
	if err != nil {
		return fmt.Errorf("read terminal session: %w", err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil {
		return fmt.Errorf("read terminal session: %w", err)
	}
	pid, tty, windowID, kind, err := parseSessionRecord(data)
	if err != nil {
		return err
	}
	born, err := processBirth(ctx, pid)
	if err != nil {
		return fmt.Errorf("inspect terminal session: %w", err)
	}
	s.info, s.born, s.tty, s.windowID = Info{ShellPID: pid, Kind: kind}, born, tty, windowID
	s.claimedPID.Store(pid)
	s.ready = true
	s.Cleanup()
	return nil
}

func parseSessionRecord(data []byte) (int32, string, string, Kind, error) {
	if len(data) > 4096 {
		return 0, "", "", "", fmt.Errorf("terminal session record is too large")
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 4 {
		return 0, "", "", "", fmt.Errorf("invalid terminal session record")
	}
	pid, err := strconv.ParseInt(lines[0], 10, 32)
	if err != nil || pid <= 1 {
		return 0, "", "", "", fmt.Errorf("invalid terminal session PID")
	}
	tty := lines[1]
	if !strings.HasPrefix(tty, "/dev/") {
		tty = ""
	}
	windowID := lines[2]
	if windowID != "" {
		id, err := strconv.ParseUint(windowID, 0, 64)
		if err != nil || id == 0 {
			windowID = ""
		} else {
			windowID = strconv.FormatUint(id, 10)
		}
	}
	var kind Kind
	switch lines[3] {
	case "Apple_Terminal":
		kind = TerminalApp
	case "iTerm.app":
		kind = ITerm2
	}
	return int32(pid), tty, windowID, kind, nil
}

// processBirth returns zero only when the PID is known to have exited. Errors
// such as permission denial must not be interpreted as permission to open again.
func processBirth(ctx context.Context, pid int32) (int64, error) {
	p, err := ps.NewProcessWithContext(ctx, pid)
	if errors.Is(err, ps.ErrorProcessNotRunning) || errors.Is(err, syscall.ESRCH) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	birth, err := p.CreateTimeWithContext(ctx)
	if errors.Is(err, ps.ErrorProcessNotRunning) || errors.Is(err, syscall.ESRCH) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if birth <= 0 {
		return 0, fmt.Errorf("terminal process creation time is unavailable")
	}
	return birth, nil
}

// FindSession does a fresh lookup before opening, rather than relying on the
// periodic UI snapshot. All worktrees are supplied to disambiguate nested paths.
// Unmanaged sandbox sessions cannot be associated reliably from host CWD alone.
func FindSession(paths []string, worktree string, claimed []*Session) (*Session, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	lister := &process.OSLister{}
	procs, err := lister.Processes(ctx)
	if err != nil {
		return nil, err
	}
	detected := NewDetectorWithLister(lister).DetectFromProcessesContext(ctx, procs, paths)
	terms := unclaimedSessions(sessionsForWorktree(detected, worktree), claimed)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, info := range terms {
		born, err := processBirth(ctx, info.ShellPID)
		if err != nil {
			return nil, err
		}
		if born == 0 {
			continue
		}
		tty := ""
		if runtime.GOOS == "darwin" {
			tty = ttyForPID(info.ShellPID)
		}
		session := &Session{info: info, born: born, tty: tty, ready: true}
		session.claimedPID.Store(info.ShellPID)
		return session, nil
	}
	return nil, nil
}

// A shell remembered by another card/mode must not be adopted merely because
// its CWD moved into this worktree. Atomic publication avoids reading a pending
// session while its owning action finishes registration.
func unclaimedSessions(infos []Info, claimed []*Session) []Info {
	var found []Info
	for _, info := range infos {
		owned := false
		for _, session := range claimed {
			if session.claimedPID.Load() == info.ShellPID {
				owned = true
				break
			}
		}
		if !owned {
			found = append(found, info)
		}
	}
	return found
}

func sessionsForWorktree(detected DetectionResult, worktree string) []Info {
	canonical := canonicalPath(worktree)
	var found []Info
	for path, sessions := range detected {
		if canonicalPath(path) == canonical {
			found = append(found, sessions...)
		}
	}
	return found
}

// Activate reports whether the recorded session still exists. An activation or
// inspection error keeps the session associated with the card, preventing a
// failed focus request from spawning a replacement terminal.
func (s *Session) Activate() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.activate(ctx, processBirth, activateSession)
}

func (s *Session) activate(ctx context.Context, birth func(context.Context, int32) (int64, error), activate func(context.Context, *Session) error) (bool, error) {
	if err := s.resolve(ctx); err != nil {
		return true, err
	}
	current, err := birth(ctx, s.info.ShellPID)
	if err != nil {
		return true, err
	}
	if current == 0 || current != s.born {
		s.Cleanup()
		return false, nil
	}
	return true, activate(ctx, s)
}

func activateSession(ctx context.Context, s *Session) error {
	switch runtime.GOOS {
	case "darwin":
		if s.tty == "" {
			return fmt.Errorf("terminal TTY unavailable; switch to the existing terminal manually")
		}
		// Target the actual tab, never just raise a generic emulator window.
		switch s.info.Kind {
		case "":
			if ok, _ := activateTerminalAppByTTY(s.tty); ok {
				return nil
			}
			if ok, _ := activateITerm2ByTTY(s.tty); ok {
				return nil
			}
		case TerminalApp, ITerm2:
			if ok, _ := activateDarwinByTTY(s.tty, s.info.Kind); ok {
				return nil
			}
		}
		return fmt.Errorf("could not activate the existing terminal tab; check macOS Automation permission and terminal support")
	case "linux":
		return activateSessionLinux(ctx, s)
	default:
		return fmt.Errorf("terminal activation is unsupported on %s", runtime.GOOS)
	}
}

// An X11 server process may own several windows. Only activate an unambiguous
// window, or the launch-time WINDOWID after checking its owner against the live
// shell ancestry. Wayland and unsupported emulators produce a visible error.
func activateSessionLinux(ctx context.Context, s *Session) error {
	procs, err := (&process.OSLister{}).Processes(ctx)
	if err != nil {
		return err
	}
	byPID := make(map[int32]process.Info, len(procs))
	for _, p := range procs {
		byPID[p.PID] = p
	}
	_, rootPID, found := walkToEmulator(byPID[s.info.ShellPID].PPID, byPID)
	if !found {
		return fmt.Errorf("could not identify the existing terminal window")
	}
	run := func(args ...string) (string, error) {
		out, err := exec.CommandContext(ctx, "xdotool", args...).Output()
		return strings.TrimSpace(string(out)), err
	}
	return activateXWindow(rootPID, s.windowID, run)
}

func activateXWindow(rootPID int32, windowID string, run func(...string) (string, error)) error {
	pid := strconv.FormatInt(int64(rootPID), 10)
	if windowID != "" {
		owner, err := run("getwindowpid", windowID)
		if err != nil {
			return fmt.Errorf("inspect existing terminal window: %w", err)
		}
		if owner != pid {
			return fmt.Errorf("existing terminal window owner changed")
		}
	} else {
		out, err := run("search", "--pid", pid)
		if err != nil {
			return fmt.Errorf("find existing terminal window (requires X11 and xdotool): %w", err)
		}
		ids := strings.Fields(out)
		if len(ids) != 1 {
			return fmt.Errorf("cannot identify a unique terminal window; switch to it manually")
		}
		windowID = ids[0]
	}
	_, err := run("windowactivate", windowID)
	return err
}
