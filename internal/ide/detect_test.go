package ide

import (
	"context"
	"testing"

	"github.com/mdelapenya/gwaim/internal/process"
)

type mockLister struct {
	procs []process.Info
}

func (m *mockLister) Processes(_ context.Context) ([]process.Info, error) {
	return m.procs, nil
}

func TestDetect_CWDMatch(t *testing.T) {
	lister := &mockLister{
		procs: []process.Info{
			{PID: 100, Name: "nvim", Cwd: "/home/user/project"},
			{PID: 200, Name: "bash", Cwd: "/home/user/project"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/home/user/project"})

	if len(result["/home/user/project"]) != 1 {
		t.Fatalf("expected 1 IDE, got %d", len(result["/home/user/project"]))
	}
	if result["/home/user/project"][0].Kind != Neovim {
		t.Errorf("expected neovim, got %s", result["/home/user/project"][0].Kind)
	}
	if result["/home/user/project"][0].PID != 100 {
		t.Errorf("expected PID 100, got %d", result["/home/user/project"][0].PID)
	}
}

func TestDetect_CmdlineMatch(t *testing.T) {
	// VS Code often has the workspace path in cmdline, not CWD.
	lister := &mockLister{
		procs: []process.Info{
			{PID: 300, Name: "Electron", Cmdline: "/Applications/Visual Studio Code.app/Contents/MacOS/Electron /home/user/project", Cwd: "/"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/home/user/project"})

	// "Electron" doesn't match any IDE pattern by name, and "code" is not in the name.
	// This should not match because the process name is "Electron" not "code".
	if len(result["/home/user/project"]) != 0 {
		t.Errorf("expected 0 IDEs for Electron process name, got %d", len(result["/home/user/project"]))
	}
}

func TestDetect_CmdlineMatchWithCodeProcess(t *testing.T) {
	// VS Code with proper process name and workspace in cmdline.
	lister := &mockLister{
		procs: []process.Info{
			{PID: 400, Name: "code", Cmdline: "code /home/user/project", Cwd: "/"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/home/user/project"})

	if len(result["/home/user/project"]) != 1 {
		t.Fatalf("expected 1 IDE, got %d", len(result["/home/user/project"]))
	}
	if result["/home/user/project"][0].Kind != VSCode {
		t.Errorf("expected vscode, got %s", result["/home/user/project"][0].Kind)
	}
}

func TestDetect_NoIDEs(t *testing.T) {
	lister := &mockLister{
		procs: []process.Info{
			{PID: 111, Name: "bash", Cwd: "/home/user/project"},
			{PID: 222, Name: "node", Cwd: "/home/user/project"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/home/user/project"})

	if len(result) != 0 {
		t.Errorf("expected empty result, got %d entries", len(result))
	}
}

func TestDetect_MultipleIDEsSameWorktree(t *testing.T) {
	lister := &mockLister{
		procs: []process.Info{
			{PID: 100, Name: "nvim", Cwd: "/project"},
			{PID: 200, Name: "code", Cmdline: "code /project", Cwd: "/"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project"})

	if len(result["/project"]) != 2 {
		t.Errorf("expected 2 IDEs, got %d", len(result["/project"]))
	}
}

func TestDetect_DifferentWorktrees(t *testing.T) {
	lister := &mockLister{
		procs: []process.Info{
			{PID: 100, Name: "nvim", Cwd: "/project-a"},
			{PID: 200, Name: "zed", Cwd: "/project-b"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project-a", "/project-b", "/project-c"})

	if len(result["/project-a"]) != 1 {
		t.Fatalf("expected 1 IDE for project-a, got %d", len(result["/project-a"]))
	}
	if result["/project-a"][0].Kind != Neovim {
		t.Errorf("expected neovim, got %s", result["/project-a"][0].Kind)
	}
	if len(result["/project-b"]) != 1 {
		t.Fatalf("expected 1 IDE for project-b, got %d", len(result["/project-b"]))
	}
	if result["/project-b"][0].Kind != Zed {
		t.Errorf("expected zed, got %s", result["/project-b"][0].Kind)
	}
	if len(result["/project-c"]) != 0 {
		t.Errorf("expected 0 IDEs for project-c, got %d", len(result["/project-c"]))
	}
}

func TestDetect_EmptyCWDSkipped(t *testing.T) {
	lister := &mockLister{
		procs: []process.Info{
			{PID: 100, Name: "nvim", Cwd: ""},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project"})

	if len(result["/project"]) != 0 {
		t.Errorf("expected 0 IDEs when CWD is empty, got %d", len(result["/project"]))
	}
}

func TestDetect_CursorIDE(t *testing.T) {
	lister := &mockLister{
		procs: []process.Info{
			{PID: 500, Name: "cursor", Cmdline: "cursor /workspace", Cwd: "/"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/workspace"})

	if len(result["/workspace"]) != 1 {
		t.Fatalf("expected 1 IDE, got %d", len(result["/workspace"]))
	}
	if result["/workspace"][0].Kind != Cursor {
		t.Errorf("expected cursor, got %s", result["/workspace"][0].Kind)
	}
}

func TestDetect_JetBrainsGoLand(t *testing.T) {
	lister := &mockLister{
		procs: []process.Info{
			{PID: 600, Name: "goland", Cwd: "/project"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project"})

	if len(result["/project"]) != 1 {
		t.Fatalf("expected 1 IDE, got %d", len(result["/project"]))
	}
	if result["/project"][0].Kind != GoLand {
		t.Errorf("expected goland, got %s", result["/project"][0].Kind)
	}
}

func TestDetect_DeduplicatesHelperProcesses(t *testing.T) {
	// VS Code spawns multiple helper processes (Renderer, GPU, Plugin, etc.)
	// that all match "code" and carry the workspace path in cmdline.
	// Helpers have PPID pointing to the main "code" process.
	// We should see exactly one VS Code entry, with all PIDs collected.
	lister := &mockLister{
		procs: []process.Info{
			{PID: 100, PPID: 1, Name: "code", Cmdline: "code /project", Cwd: "/"},
			{PID: 101, PPID: 100, Name: "Code Helper (Renderer)", Cmdline: "/Applications/Visual Studio Code.app/Contents/Frameworks/Code Helper (Renderer).app --type=renderer /project", Cwd: "/"},
			{PID: 102, PPID: 100, Name: "Code Helper (GPU)", Cmdline: "/Applications/Visual Studio Code.app/Contents/Frameworks/Code Helper (GPU).app --type=gpu /project", Cwd: "/"},
			{PID: 103, PPID: 100, Name: "Code Helper (Plugin)", Cmdline: "/Applications/Visual Studio Code.app/Contents/Frameworks/Code Helper (Plugin).app --type=utility /project", Cwd: "/"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project"})

	if len(result["/project"]) != 1 {
		t.Fatalf("expected 1 IDE (deduplicated), got %d", len(result["/project"]))
	}
	info := result["/project"][0]
	if info.Kind != VSCode {
		t.Errorf("expected vscode, got %s", info.Kind)
	}
	if info.PID != 100 {
		t.Errorf("expected root PID 100, got %d", info.PID)
	}
	if len(info.ExtraPIDs) != 4 {
		t.Errorf("expected 4 PIDs (main + 3 helpers), got %d: %v", len(info.ExtraPIDs), info.ExtraPIDs)
	}
}

func TestDetect_TwoIndependentVSCodeWindows(t *testing.T) {
	// Two separate VS Code windows opened for the same worktree.
	// Each has its own root process with independent helper trees.
	// We should see TWO entries — one per window.
	lister := &mockLister{
		procs: []process.Info{
			// Window 1: PID 100 is the root
			{PID: 100, PPID: 1, Name: "code", Cmdline: "code /project", Cwd: "/"},
			{PID: 101, PPID: 100, Name: "Code Helper (Renderer)", Cmdline: "Code Helper /project", Cwd: "/"},
			{PID: 102, PPID: 100, Name: "Code Helper (GPU)", Cmdline: "Code Helper /project", Cwd: "/"},
			// Window 2: PID 200 is the root
			{PID: 200, PPID: 1, Name: "code", Cmdline: "code /project", Cwd: "/"},
			{PID: 201, PPID: 200, Name: "Code Helper (Renderer)", Cmdline: "Code Helper /project", Cwd: "/"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project"})

	if len(result["/project"]) != 2 {
		t.Fatalf("expected 2 IDEs (two independent windows), got %d", len(result["/project"]))
	}
	// Window 1
	if result["/project"][0].PID != 100 {
		t.Errorf("expected first window root PID 100, got %d", result["/project"][0].PID)
	}
	if len(result["/project"][0].ExtraPIDs) != 3 {
		t.Errorf("expected 3 PIDs for window 1, got %d: %v", len(result["/project"][0].ExtraPIDs), result["/project"][0].ExtraPIDs)
	}
	// Window 2
	if result["/project"][1].PID != 200 {
		t.Errorf("expected second window root PID 200, got %d", result["/project"][1].PID)
	}
	if len(result["/project"][1].ExtraPIDs) != 2 {
		t.Errorf("expected 2 PIDs for window 2, got %d: %v", len(result["/project"][1].ExtraPIDs), result["/project"][1].ExtraPIDs)
	}
}

func TestDetect_NestedHelpers(t *testing.T) {
	// Some helpers spawn sub-helpers (e.g. Renderer spawns a child Renderer).
	// They should all roll up to the same root.
	lister := &mockLister{
		procs: []process.Info{
			{PID: 100, PPID: 1, Name: "code", Cmdline: "code /project", Cwd: "/"},
			{PID: 101, PPID: 100, Name: "Code Helper (Renderer)", Cmdline: "Code Helper /project", Cwd: "/"},
			{PID: 102, PPID: 101, Name: "Code Helper (Renderer)", Cmdline: "Code Helper /project", Cwd: "/"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project"})

	if len(result["/project"]) != 1 {
		t.Fatalf("expected 1 IDE (nested helpers rolled up), got %d", len(result["/project"]))
	}
	if result["/project"][0].PID != 100 {
		t.Errorf("expected root PID 100, got %d", result["/project"][0].PID)
	}
	if len(result["/project"][0].ExtraPIDs) != 3 {
		t.Errorf("expected 3 PIDs, got %d: %v", len(result["/project"][0].ExtraPIDs), result["/project"][0].ExtraPIDs)
	}
}

func TestDetectFromProcesses(t *testing.T) {
	// Test the DetectFromProcesses method directly with pre-fetched processes.
	procs := []process.Info{
		{PID: 100, Name: "nvim", Cwd: "/project"},
		{PID: 200, Name: "code", Cmdline: "code /other", Cwd: "/"},
	}

	d := NewDetector() // lister not used when calling DetectFromProcesses
	result := d.DetectFromProcesses(procs, []string{"/project", "/other"})

	if len(result["/project"]) != 1 {
		t.Fatalf("expected 1 IDE for /project, got %d", len(result["/project"]))
	}
	if len(result["/other"]) != 1 {
		t.Fatalf("expected 1 IDE for /other, got %d", len(result["/other"]))
	}
}

func TestDetect_CmdlineMatchesMostSpecificPath(t *testing.T) {
	// When the main repo path is a prefix of a worktree path, a process
	// whose cmdline contains the worktree path should only match the
	// worktree, not the parent repo.
	lister := &mockLister{
		procs: []process.Info{
			{PID: 100, PPID: 1, Name: "code", Cmdline: "code /repo/.gwaim-worktrees/feature", Cwd: "/"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/repo", "/repo/.gwaim-worktrees/feature"})

	if len(result["/repo"]) != 0 {
		t.Errorf("expected 0 IDEs for parent /repo, got %d", len(result["/repo"]))
	}
	if len(result["/repo/.gwaim-worktrees/feature"]) != 1 {
		t.Fatalf("expected 1 IDE for worktree, got %d", len(result["/repo/.gwaim-worktrees/feature"]))
	}
	if result["/repo/.gwaim-worktrees/feature"][0].Kind != VSCode {
		t.Errorf("expected vscode, got %s", result["/repo/.gwaim-worktrees/feature"][0].Kind)
	}
}

func TestDetect_CWDMatchNotAffectedByMostSpecificCmdline(t *testing.T) {
	// A process with CWD matching the parent repo should still match,
	// even if its cmdline also contains a child worktree path.
	lister := &mockLister{
		procs: []process.Info{
			{PID: 100, PPID: 1, Name: "nvim", Cwd: "/repo"},
			{PID: 200, PPID: 1, Name: "code", Cmdline: "code /repo/.gwaim-worktrees/feature", Cwd: "/"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/repo", "/repo/.gwaim-worktrees/feature"})

	if len(result["/repo"]) != 1 {
		t.Fatalf("expected 1 IDE for /repo (CWD match), got %d", len(result["/repo"]))
	}
	if result["/repo"][0].Kind != Neovim {
		t.Errorf("expected neovim, got %s", result["/repo"][0].Kind)
	}
	if len(result["/repo/.gwaim-worktrees/feature"]) != 1 {
		t.Fatalf("expected 1 IDE for worktree, got %d", len(result["/repo/.gwaim-worktrees/feature"]))
	}
	if result["/repo/.gwaim-worktrees/feature"][0].Kind != VSCode {
		t.Errorf("expected vscode, got %s", result["/repo/.gwaim-worktrees/feature"][0].Kind)
	}
}
