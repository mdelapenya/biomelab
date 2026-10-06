package gui

import (
	"testing"

	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/ops"
)

// TestRepoStateApply_DropsStaleSnapshot is the regression guard for the
// "deleted card briefly reappears" race: a long-running refresh that
// snapshotted before a delete (lower Generation) must NOT overwrite the
// state with its pre-delete worktree list.
func TestRepoStateApply_DropsStaleSnapshot(t *testing.T) {
	s := &RepoState{}

	// Apply a "post-mutation" snapshot at gen=5 with only the main wt.
	post := ops.RefreshResult{
		Worktrees:  []git.Worktree{{Path: "/repo", Branch: "main", IsMain: true}},
		Generation: 5,
	}
	if !s.Apply(post) {
		t.Fatal("Apply(post) returned false; expected the fresh result to apply")
	}
	if s.LastAppliedGen != 5 {
		t.Errorf("LastAppliedGen = %d, want 5", s.LastAppliedGen)
	}
	if got := len(s.Worktrees); got != 1 {
		t.Fatalf("Worktrees after post-apply: %d, want 1", got)
	}

	// Now apply the stale snapshot (gen=3) carrying the doomed branch.
	stale := ops.RefreshResult{
		Worktrees: []git.Worktree{
			{Path: "/repo", Branch: "main", IsMain: true},
			{Name: "doomed", Path: "/wt/doomed", Branch: "doomed"},
		},
		Generation: 3,
	}
	if s.Apply(stale) {
		t.Fatal("Apply(stale) returned true; expected pre-mutation snapshot to be dropped")
	}
	if got := len(s.Worktrees); got != 1 {
		t.Errorf("Worktrees after stale drop: %d, want 1 (stale snapshot must not overwrite)", got)
	}
	if s.LastAppliedGen != 5 {
		t.Errorf("LastAppliedGen moved after stale drop: %d, want 5", s.LastAppliedGen)
	}
}

// TestRepoStateApply_SameGenerationApplies confirms that snapshots taken at
// the same generation as the last applied one are still applied — they
// represent a fresh observation of the same logical state and may bring
// updated agent/IDE/PR detail.
func TestRepoStateApply_SameGenerationApplies(t *testing.T) {
	s := &RepoState{}

	r1 := ops.RefreshResult{
		Worktrees:  []git.Worktree{{Path: "/repo", Branch: "main", IsMain: true}},
		Generation: 0,
	}
	if !s.Apply(r1) {
		t.Fatal("first Apply dropped")
	}
	r2 := ops.RefreshResult{
		Worktrees: []git.Worktree{
			{Path: "/repo", Branch: "main", IsMain: true},
			{Name: "wt1", Path: "/wt/wt1", Branch: "wt1"},
		},
		Generation: 0,
	}
	if !s.Apply(r2) {
		t.Fatal("second Apply at same gen dropped; expected apply")
	}
	if got := len(s.Worktrees); got != 2 {
		t.Errorf("Worktrees: %d, want 2", got)
	}
}

// TestRepoStateApply_ErrorAlwaysPasses ensures errors surface even when the
// snapshot's generation would otherwise mark it stale, so users still see
// failure messages from in-flight refreshes that completed late.
func TestRepoStateApply_ErrorAlwaysPasses(t *testing.T) {
	s := &RepoState{LastAppliedGen: 10}

	stale := ops.RefreshResult{
		Err:        errFakeRefresh,
		Generation: 2,
	}
	if !s.Apply(stale) {
		t.Fatal("Apply dropped a stale error result; errors should always surface")
	}
	if s.StatusMessage == "" || !s.StatusIsError {
		t.Errorf("StatusMessage=%q StatusIsError=%v; want non-empty error",
			s.StatusMessage, s.StatusIsError)
	}
}

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

var errFakeRefresh = fakeErr("simulated refresh failure")

func TestRepoStateApplyPreservesSelectedWorktreeAcrossSortedRefresh(t *testing.T) {
	s := &RepoState{Worktrees: []git.Worktree{
		{Path: "/repo", Branch: "main"},
		{Path: "/wt/alpha", Branch: "alpha"},
		{Path: "/wt/beta", Branch: "beta"},
	}, SelectedCard: 2}
	if !s.Apply(ops.RefreshResult{Worktrees: []git.Worktree{
		{Path: "/repo", Branch: "main"},
		{Path: "/wt/beta", Branch: "beta"},
		{Path: "/wt/aardvark", Branch: "aardvark"},
		{Path: "/wt/alpha", Branch: "alpha"},
	}}) {
		t.Fatal("refresh was dropped")
	}
	if got := s.Worktrees[s.SelectedCard].Path; got != "/wt/beta" {
		t.Fatalf("selection moved to %q, want beta", got)
	}
	// If the selected worktree disappears, keep the index in range.
	s.SetWorktrees([]git.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/wt/alpha", Branch: "alpha"}})
	if s.SelectedCard != 1 {
		t.Fatalf("selection after removal = %d, want nearest remaining card", s.SelectedCard)
	}
}
