package gui

import (
	"testing"

	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/ops"
	"github.com/mdelapenya/biomelab/internal/provider"
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

func TestRepoStateApplyCLIAvailabilityRequiresPresence(t *testing.T) {
	s := &RepoState{}
	s.Apply(ops.RefreshResult{CLIAvail: provider.CLINotAuthenticated})
	if s.HasCLIAvail {
		t.Fatal("quick/local result changed CLI availability")
	}
	s.Apply(ops.RefreshResult{CLIAvail: provider.CLINotAuthenticated, HasCLIAvail: true})
	if !s.HasCLIAvail || s.CLIAvail != provider.CLINotAuthenticated {
		t.Fatalf("CLI state = (%v, %v), want known and unauthenticated", s.HasCLIAvail, s.CLIAvail)
	}
	s.Apply(ops.RefreshResult{CLIAvail: provider.CLIAvailable, HasCLIAvail: true})
	if !s.HasCLIAvail || s.CLIAvail != provider.CLIAvailable {
		t.Fatalf("zero-valued available result was lost: (%v, %v)", s.HasCLIAvail, s.CLIAvail)
	}
}

func TestRepoStateApplyPreservesSandboxStatusWhenInventoryFails(t *testing.T) {
	s := &RepoState{SandboxStatus: 1}
	s.Apply(ops.RefreshResult{SandboxStatus: 0, HasSbxStatus: false})
	if s.SandboxStatus != 1 {
		t.Fatal("failed inventory reset prior sandbox status")
	}
	s.Apply(ops.RefreshResult{SandboxStatus: 0, HasSbxStatus: true})
	if s.SandboxStatus != 0 {
		t.Fatal("successful empty inventory did not mark sandbox missing")
	}
}

func TestRepoStateApplyDetailedPRLookupsMergeAndPrune(t *testing.T) {
	s := &RepoState{
		Worktrees: []git.Worktree{
			{Path: "/repo", Branch: "main"},
			{Path: "/wt/found", Branch: "found"},
			{Path: "/wt/absent", Branch: "absent"},
			{Path: "/wt/error", Branch: "error"},
		},
		PRs: provider.PRResult{
			"found":  {Number: 1},
			"absent": {Number: 2},
			"error":  {Number: 3},
		},
	}
	s.Apply(ops.RefreshResult{HasPRs: true, HasPRLookups: true,
		PRLookups: provider.PRLookupResult{
			"found":  {Info: &provider.PRInfo{Number: 10}},
			"absent": {},
			"error":  {Err: errFakeRefresh},
		},
	})
	if s.PRs["found"] == nil || s.PRs["found"].Number != 10 || s.PRs["absent"] != nil || s.PRs["error"] == nil || s.PRs["error"].Number != 3 {
		t.Fatalf("detailed PR merge failed: %+v", s.PRs)
	}
	// A subsequent quick snapshot can remove a branch even while its PR
	// lookup is failing; its obsolete cached status must disappear.
	s.Apply(ops.RefreshResult{Worktrees: []git.Worktree{
		{Path: "/repo", Branch: "main"},
		{Path: "/wt/found", Branch: "found"},
		{Path: "/wt/absent", Branch: "absent"},
	}})
	if s.PRs["error"] != nil {
		t.Fatal("removed worktree retained a cached PR")
	}
}
