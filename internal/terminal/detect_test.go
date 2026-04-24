package terminal

import (
	"context"
	"testing"

	"github.com/mdelapenya/biomelab/internal/process"
)

type mockLister struct {
	procs []process.Info
}

func (m *mockLister) Processes(_ context.Context) ([]process.Info, error) {
	return m.procs, nil
}

func TestDetect_ShellWithTerminalParent(t *testing.T) {
	lister := &mockLister{
		procs: []process.Info{
			{PID: 1, Name: "launchd"},
			{PID: 100, PPID: 1, Name: "Terminal"},
			{PID: 150, PPID: 100, Name: "login"},
			{PID: 200, PPID: 150, Name: "zsh", Cwd: "/project"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project"})

	if len(result["/project"]) != 1 {
		t.Fatalf("expected 1 terminal, got %d", len(result["/project"]))
	}

	info := result["/project"][0]
	if info.Kind != TerminalApp {
		t.Errorf("expected kind %q, got %q", TerminalApp, info.Kind)
	}
	if info.ShellPID != 200 {
		t.Errorf("expected ShellPID 200, got %d", info.ShellPID)
	}
	if info.RootPID != 100 {
		t.Errorf("expected RootPID 100, got %d", info.RootPID)
	}
}

func TestDetect_ShellWithoutTerminalParent(t *testing.T) {
	// A shell spawned by an IDE (code / VS Code) should NOT be detected.
	lister := &mockLister{
		procs: []process.Info{
			{PID: 1, Name: "launchd"},
			{PID: 100, PPID: 1, Name: "code"},
			{PID: 200, PPID: 100, Name: "bash", Cwd: "/project"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project"})

	if len(result) != 0 {
		t.Errorf("expected 0 terminals, got %d", len(result))
	}
}

func TestDetect_MultipleShellsSameEmulator(t *testing.T) {
	// Two shells under the same Terminal.app should deduplicate to one entry.
	lister := &mockLister{
		procs: []process.Info{
			{PID: 1, Name: "launchd"},
			{PID: 100, PPID: 1, Name: "Terminal"},
			{PID: 200, PPID: 100, Name: "zsh", Cwd: "/project"},
			{PID: 300, PPID: 100, Name: "bash", Cwd: "/project"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project"})

	if len(result["/project"]) != 1 {
		t.Fatalf("expected 1 terminal (deduplicated), got %d", len(result["/project"]))
	}
}

func TestDetect_DifferentWorktrees(t *testing.T) {
	lister := &mockLister{
		procs: []process.Info{
			{PID: 1, Name: "launchd"},
			{PID: 100, PPID: 1, Name: "Terminal"},
			{PID: 200, PPID: 100, Name: "zsh", Cwd: "/project-a"},
			{PID: 50, PPID: 1, Name: "iTerm2"},
			{PID: 300, PPID: 50, Name: "fish", Cwd: "/project-b"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project-a", "/project-b"})

	if len(result["/project-a"]) != 1 {
		t.Fatalf("expected 1 terminal for project-a, got %d", len(result["/project-a"]))
	}
	if result["/project-a"][0].Kind != TerminalApp {
		t.Errorf("expected Terminal for project-a, got %s", result["/project-a"][0].Kind)
	}

	if len(result["/project-b"]) != 1 {
		t.Fatalf("expected 1 terminal for project-b, got %d", len(result["/project-b"]))
	}
	if result["/project-b"][0].Kind != ITerm2 {
		t.Errorf("expected iTerm2 for project-b, got %s", result["/project-b"][0].Kind)
	}
}

func TestDetect_EmptyCWDSkipped(t *testing.T) {
	lister := &mockLister{
		procs: []process.Info{
			{PID: 1, Name: "launchd"},
			{PID: 100, PPID: 1, Name: "Terminal"},
			{PID: 200, PPID: 100, Name: "zsh", Cwd: ""},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project"})

	if len(result) != 0 {
		t.Errorf("expected 0 terminals, got %d", len(result))
	}
}

func TestDetect_NoShells(t *testing.T) {
	lister := &mockLister{
		procs: []process.Info{
			{PID: 1, Name: "launchd"},
			{PID: 100, PPID: 1, Name: "Terminal"},
			{PID: 200, PPID: 100, Name: "node"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project"})

	if len(result) != 0 {
		t.Errorf("expected empty result, got %d entries", len(result))
	}
}

func TestDetect_DeepPPIDChain(t *testing.T) {
	// Shell → login → Terminal (2 hops up).
	lister := &mockLister{
		procs: []process.Info{
			{PID: 1, Name: "launchd"},
			{PID: 100, PPID: 1, Name: "Terminal"},
			{PID: 200, PPID: 100, Name: "login"},
			{PID: 300, PPID: 200, Name: "bash", Cwd: "/project"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project"})

	if len(result["/project"]) != 1 {
		t.Fatalf("expected 1 terminal via deep chain, got %d", len(result["/project"]))
	}
	if result["/project"][0].RootPID != 100 {
		t.Errorf("expected RootPID 100, got %d", result["/project"][0].RootPID)
	}
}

func TestDetect_PPIDCycleProtection(t *testing.T) {
	// Process with PPID pointing to itself should not cause infinite loop.
	lister := &mockLister{
		procs: []process.Info{
			{PID: 100, PPID: 100, Name: "zsh", Cwd: "/project"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project"})

	// Should not panic; no terminal emulator found.
	if len(result) != 0 {
		t.Errorf("expected 0 terminals for self-referencing PPID, got %d", len(result))
	}
}

func TestDetectFromProcesses(t *testing.T) {
	procs := []process.Info{
		{PID: 1, Name: "launchd"},
		{PID: 100, PPID: 1, Name: "Terminal"},
		{PID: 200, PPID: 100, Name: "zsh", Cwd: "/project"},
		{PID: 300, PPID: 100, Name: "bash", Cwd: "/other"},
	}

	d := NewDetector() // lister not used when calling DetectFromProcesses
	result := d.DetectFromProcesses(procs, []string{"/project", "/other"})

	if len(result["/project"]) != 1 {
		t.Fatalf("expected 1 terminal for /project, got %d", len(result["/project"]))
	}
	if len(result["/other"]) != 1 {
		t.Fatalf("expected 1 terminal for /other, got %d", len(result["/other"]))
	}
}

func TestDetect_ITerm2BeforeTerminal(t *testing.T) {
	// Ensure iTerm2 is matched before the broad "terminal" pattern.
	lister := &mockLister{
		procs: []process.Info{
			{PID: 1, Name: "launchd"},
			{PID: 100, PPID: 1, Name: "iTerm2"},
			{PID: 200, PPID: 100, Name: "zsh", Cwd: "/project"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project"})

	if len(result["/project"]) != 1 {
		t.Fatalf("expected 1 terminal, got %d", len(result["/project"]))
	}
	if result["/project"][0].Kind != ITerm2 {
		t.Errorf("expected iTerm2, got %s", result["/project"][0].Kind)
	}
}

func TestDetect_MultipleEmulatorsForSameWorktree(t *testing.T) {
	// Two different terminal emulators both with shells in the same worktree.
	lister := &mockLister{
		procs: []process.Info{
			{PID: 1, Name: "launchd"},
			{PID: 100, PPID: 1, Name: "Terminal"},
			{PID: 200, PPID: 100, Name: "zsh", Cwd: "/project"},
			{PID: 50, PPID: 1, Name: "iTerm2"},
			{PID: 300, PPID: 50, Name: "bash", Cwd: "/project"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project"})

	// Two different root PIDs = two entries.
	if len(result["/project"]) != 2 {
		t.Fatalf("expected 2 terminals (different emulators), got %d", len(result["/project"]))
	}
}

func TestIsShell(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"bash", true},
		{"zsh", true},
		{"fish", true},
		{"pwsh", true},
		{"powershell", true},
		{"-bash", true},  // login shell (macOS style)
		{"-zsh", true},   // login shell (macOS style)
		{"node", false},
		{"code", false},
		{"claude", false},
		{"vim", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isShell(tt.name); got != tt.want {
				t.Errorf("isShell(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestWalkToEmulator(t *testing.T) {
	byPID := map[int32]process.Info{
		1:   {PID: 1, PPID: 0, Name: "launchd"},
		100: {PID: 100, PPID: 1, Name: "Terminal"},
		200: {PID: 200, PPID: 100, Name: "login"},
	}

	// Walk from login → Terminal (1 hop).
	kind, pid, found := walkToEmulator(200, byPID)
	if !found {
		t.Fatal("expected to find emulator")
	}
	if kind != TerminalApp {
		t.Errorf("expected Terminal, got %s", kind)
	}
	if pid != 100 {
		t.Errorf("expected PID 100, got %d", pid)
	}

	// Walk from a PID with no terminal ancestor.
	_, _, found = walkToEmulator(1, byPID)
	if found {
		t.Error("expected no emulator from launchd ancestry")
	}
}

func TestWalkToEmulator_UnknownPPID(t *testing.T) {
	// Shell whose parent PID doesn't exist in the map.
	byPID := map[int32]process.Info{
		1: {PID: 1, PPID: 0, Name: "launchd"},
	}

	_, _, found := walkToEmulator(999, byPID)
	if found {
		t.Error("expected no emulator for unknown PPID")
	}
}

func TestDetect_CWDMismatch(t *testing.T) {
	// Shell under a terminal emulator but CWD doesn't match any worktree.
	lister := &mockLister{
		procs: []process.Info{
			{PID: 1, Name: "launchd"},
			{PID: 100, PPID: 1, Name: "Terminal"},
			{PID: 200, PPID: 100, Name: "zsh", Cwd: "/other-project"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project"})

	if len(result) != 0 {
		t.Errorf("expected 0 terminals (CWD mismatch), got %d", len(result))
	}
}

func TestDetect_LinuxEmulators(t *testing.T) {
	tests := []struct {
		emulatorName string
		wantKind     Kind
	}{
		{"gnome-terminal-server", GnomeTerminal},
		{"konsole", Konsole},
		{"alacritty", Alacritty},
		{"kitty", Kitty},
		{"wezterm-gui", WezTerm},
		{"tilix", Tilix},
		{"xfce4-terminal", Xfce4Terminal},
	}
	for _, tt := range tests {
		t.Run(tt.emulatorName, func(t *testing.T) {
			lister := &mockLister{
				procs: []process.Info{
					{PID: 1, Name: "systemd"},
					{PID: 100, PPID: 1, Name: tt.emulatorName},
					{PID: 200, PPID: 100, Name: "bash", Cwd: "/project"},
				},
			}

			d := NewDetectorWithLister(lister)
			result := d.Detect([]string{"/project"})

			if len(result["/project"]) != 1 {
				t.Fatalf("expected 1 terminal for %s, got %d", tt.emulatorName, len(result["/project"]))
			}
			if result["/project"][0].Kind != tt.wantKind {
				t.Errorf("expected kind %s, got %s", tt.wantKind, result["/project"][0].Kind)
			}
		})
	}
}

func TestDetect_NoProcesses(t *testing.T) {
	lister := &mockLister{
		procs: []process.Info{},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project"})

	if len(result) != 0 {
		t.Errorf("expected empty result for no processes, got %d", len(result))
	}
}

func TestDetect_NoWorktreePaths(t *testing.T) {
	lister := &mockLister{
		procs: []process.Info{
			{PID: 1, Name: "launchd"},
			{PID: 100, PPID: 1, Name: "Terminal"},
			{PID: 200, PPID: 100, Name: "zsh", Cwd: "/project"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{})

	if len(result) != 0 {
		t.Errorf("expected empty result for no worktree paths, got %d", len(result))
	}
}

func TestDetect_PathNormalization(t *testing.T) {
	// Worktree path with trailing slash should still match.
	lister := &mockLister{
		procs: []process.Info{
			{PID: 1, Name: "launchd"},
			{PID: 100, PPID: 1, Name: "Terminal"},
			{PID: 200, PPID: 100, Name: "zsh", Cwd: "/project"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project/"})

	if len(result["/project/"]) != 1 {
		t.Fatalf("expected 1 terminal with path normalization, got %d", len(result["/project/"]))
	}
}
