package agent

import (
	"context"
	"testing"
	"time"
)

type mockLister struct {
	procs []ProcessInfo
}

func (m *mockLister) Processes(_ context.Context) ([]ProcessInfo, error) {
	return m.procs, nil
}

func TestDetect_MatchesCWDToWorktree(t *testing.T) {
	lister := &mockLister{
		procs: []ProcessInfo{
			{PID: 12345, Name: "claude", Cwd: "/home/user/project", Status: "S", Created: time.Date(2026, 3, 4, 17, 53, 7, 0, time.UTC)},
			{PID: 67890, Name: "kiro", Cwd: "/home/user/other", Status: "S+", Created: time.Date(2026, 3, 10, 18, 13, 23, 0, time.UTC)},
			{PID: 111, Name: "bash", Cwd: "/home/user/project"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/home/user/project", "/home/user/other", "/home/user/unrelated"})

	if len(result["/home/user/project"]) != 1 {
		t.Fatalf("expected 1 agent for project, got %d", len(result["/home/user/project"]))
	}
	if len(result["/home/user/other"]) != 1 {
		t.Fatalf("expected 1 agent for other, got %d", len(result["/home/user/other"]))
	}
	if len(result["/home/user/unrelated"]) != 0 {
		t.Errorf("expected 0 agents for unrelated, got %d", len(result["/home/user/unrelated"]))
	}

	proj := result["/home/user/project"][0]
	if proj.Kind != Claude {
		t.Errorf("expected claude, got %s", proj.Kind)
	}
	if proj.PID != "12345" {
		t.Errorf("expected PID 12345, got %s", proj.PID)
	}
	if proj.State != "S" {
		t.Errorf("expected state S, got %s", proj.State)
	}
	if proj.Started == "" {
		t.Error("expected non-empty started")
	}
}

func TestDetect_NoAgents(t *testing.T) {
	lister := &mockLister{
		procs: []ProcessInfo{
			{PID: 111, Name: "bash", Cwd: "/home/user/project"},
			{PID: 222, Name: "vim", Cwd: "/home/user/project"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/home/user/project"})

	if len(result) != 0 {
		t.Errorf("expected empty result, got %d entries", len(result))
	}
}

func TestDetect_EmptyCWDSkipped(t *testing.T) {
	lister := &mockLister{
		procs: []ProcessInfo{
			{PID: 12345, Name: "claude", Cwd: ""},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/home/user/project"})

	if len(result["/home/user/project"]) != 0 {
		t.Errorf("expected 0 agents when CWD is empty, got %d", len(result["/home/user/project"]))
	}
}

func TestDetect_ParentChildDeduped(t *testing.T) {
	// A CLI agent spawns a child with the same name and CWD.
	// Only the parent should be kept.
	lister := &mockLister{
		procs: []ProcessInfo{
			{PID: 100, Name: "copilot", Cwd: "/project", Status: "S"},
			{PID: 200, PPID: 100, Name: "copilot", Cwd: "/project", Status: "S"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project"})

	if len(result["/project"]) != 1 {
		t.Errorf("expected 1 agent (parent-child deduped), got %d", len(result["/project"]))
	}
	if result["/project"][0].PID != "100" {
		t.Errorf("expected parent PID 100, got %s", result["/project"][0].PID)
	}
}

func TestDetect_IndependentSessionsSameKind(t *testing.T) {
	// Two independent sessions of the same agent (not parent-child) should both appear.
	lister := &mockLister{
		procs: []ProcessInfo{
			{PID: 100, PPID: 1, Name: "claude", Cwd: "/project", Status: "S"},
			{PID: 200, PPID: 1, Name: "claude", Cwd: "/project", Status: "S"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project"})

	if len(result["/project"]) != 2 {
		t.Errorf("expected 2 agents (independent sessions), got %d", len(result["/project"]))
	}
}

func TestDetect_DifferentKindsSameWorktree(t *testing.T) {
	// Two different agent kinds in the same worktree should both appear.
	lister := &mockLister{
		procs: []ProcessInfo{
			{PID: 100, Name: "claude", Cwd: "/project", Status: "S"},
			{PID: 200, Name: "copilot", Cwd: "/project", Status: "S"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/project"})

	if len(result["/project"]) != 2 {
		t.Errorf("expected 2 agents (different kinds), got %d", len(result["/project"]))
	}
}

func TestDetect_CopilotPattern(t *testing.T) {
	lister := &mockLister{
		procs: []ProcessInfo{
			{PID: 999, Name: "copilot-agent", Cwd: "/work"},
		},
	}

	d := NewDetectorWithLister(lister)
	result := d.Detect([]string{"/work"})

	if len(result["/work"]) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(result["/work"]))
	}
	if result["/work"][0].Kind != Copilot {
		t.Errorf("expected copilot, got %s", result["/work"][0].Kind)
	}
}
