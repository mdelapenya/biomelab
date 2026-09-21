package terminal

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTrackedSessionIdentityAndFailureHandling(t *testing.T) {
	for _, tt := range []struct {
		name                    string
		birth                   int64
		inspectErr, activateErr error
		alive                   bool
		calls                   int
	}{
		{name: "same shell after cd or exec", birth: 100, alive: true, calls: 1},
		{name: "closed shell", birth: 0},
		{name: "reused PID", birth: 200},
		{name: "inspection denied", inspectErr: os.ErrPermission, alive: true},
		{name: "activation denied", birth: 100, activateErr: os.ErrPermission, alive: true, calls: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := &Session{info: Info{ShellPID: 42}, born: 100, ready: true, tty: "/dev/ttys003"}
			calls := 0
			alive, err := s.activate(context.Background(), func(context.Context, int32) (int64, error) { return tt.birth, tt.inspectErr }, func(context.Context, *Session) error { calls++; return tt.activateErr })
			if alive != tt.alive || calls != tt.calls || (err != nil) != (tt.inspectErr != nil || tt.activateErr != nil) {
				t.Fatalf("alive=%v calls=%d err=%v", alive, calls, err)
			}
		})
	}
}

func TestSessionRecordParsing(t *testing.T) {
	pid, tty, window, kind, err := parseSessionRecord([]byte("123\n/dev/ttys007\n0x12\nApple_Terminal\n"))
	if err != nil || pid != 123 || tty != "/dev/ttys007" || window != "18" || kind != TerminalApp {
		t.Fatalf("bad record: %d %s %s %s %v", pid, tty, window, kind, err)
	}
	_, tty, window, kind, err = parseSessionRecord([]byte("123\nnot a tty\ninvalid\niTerm.app\n"))
	if err != nil || tty != "" || window != "" || kind != ITerm2 {
		t.Fatal("invalid optional metadata not discarded")
	}
	for _, record := range []string{"1\n/dev/x\n\n\n", "bad\n/dev/x\n\n\n", "12\n/dev/x\n", strings.Repeat("x", 4097)} {
		if _, _, _, _, err := parseSessionRecord([]byte(record)); err == nil {
			t.Fatalf("accepted invalid record %q", record[:min(len(record), 30)])
		}
	}
}

func TestTrackedShellRegistersBeforeForegroundCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell handshake")
	}
	// Run the actual prelude without opening a desktop window. The host shell
	// waits on a foreground command, just as it does while sbx is attached.
	dir := filepath.Join(t.TempDir(), "space and ' quote")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	var cmd *exec.Cmd
	s, err := openTracked("cd "+shellQuote(dir)+"; read answer; exec /bin/sh -c 'read answer'", func(script string) error {
		cmd = exec.Command("/bin/sh", "-c", script)
		input, e := cmd.StdinPipe()
		if e != nil {
			return e
		}
		t.Cleanup(func() { _ = input.Close() })
		if e = cmd.Start(); e != nil {
			return e
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Cleanup)
	if s.info.ShellPID != int32(cmd.Process.Pid) || s.born == 0 {
		t.Fatalf("wrong host identity: %+v", s)
	}
	if _, err := os.Stat(s.markerDir); !os.IsNotExist(err) {
		t.Fatal("resolved handshake files retained")
	}
	alive, err := s.activate(context.Background(), processBirth, func(context.Context, *Session) error { return nil })
	if !alive || err != nil {
		t.Fatalf("foreground host session lost: %v %v", alive, err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	alive, err = s.activate(context.Background(), processBirth, func(context.Context, *Session) error { t.Fatal("activated dead shell"); return nil })
	if alive || err != nil {
		t.Fatalf("dead shell not discarded: %v %v", alive, err)
	}
}

func TestPendingSessionCanResolveLaterAndOffersRecovery(t *testing.T) {
	dir := t.TempDir()
	s := &Session{markerDir: dir, started: time.Now().Add(-time.Minute)}
	alive, err := s.activate(context.Background(), processBirth, func(context.Context, *Session) error { t.Fatal("activated unresolved session"); return nil })
	if !alive || !errors.Is(err, errSessionStarting) || !s.RecoveryNeeded() {
		t.Fatalf("pending lost: %v %v", alive, err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	record := []byte(strings.Join([]string{strconv.Itoa(os.Getpid()), "/dev/tty", "", "Apple_Terminal", ""}, "\n"))
	if err := os.WriteFile(filepath.Join(dir, "session"), record, 0600); err != nil {
		t.Fatal(err)
	}
	alive, err = s.activate(context.Background(), processBirth, func(context.Context, *Session) error { return nil })
	if !alive || err != nil || s.RecoveryNeeded() {
		t.Fatalf("late handshake lost: %v %v", alive, err)
	}
}

func TestActivateXWindowRejectsAmbiguity(t *testing.T) {
	for _, tt := range []struct {
		name, window, output string
		wantErr              bool
	}{
		{"single", "", "123", false}, {"several", "", "123\n456", true}, {"absent", "", "", true},
		{"owned WINDOWID", "123", "42", false}, {"stale WINDOWID", "123", "99", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			activated := false
			err := activateXWindow(42, tt.window, func(args ...string) (string, error) {
				if args[0] == "windowactivate" {
					activated = true
					return "", nil
				}
				return tt.output, nil
			})
			if (err != nil) != tt.wantErr || activated == tt.wantErr {
				t.Fatalf("err=%v activated=%v", err, activated)
			}
		})
	}
}

func TestDiscoveryDoesNotAdoptAnotherCardsShell(t *testing.T) {
	owned := &Session{}
	owned.claimedPID.Store(42)
	got := unclaimedSessions([]Info{{ShellPID: 42}, {ShellPID: 43}}, []*Session{owned, &Session{}})
	if len(got) != 1 || got[0].ShellPID != 43 {
		t.Fatalf("adopted another card's session: %v", got)
	}
}
