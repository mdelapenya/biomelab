package ops

import (
	"context"
	"testing"

	"github.com/mdelapenya/biomelab/internal/provider"
)

func TestQuickRefreshDoesNotReplaceDetection(t *testing.T) {
	_, repo := newOpsRepo(t, nil)
	result := QuickRefresh(repo)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if len(result.Worktrees) == 0 {
		t.Fatal("missing quick snapshot")
	}
	if result.Agents != nil || result.IDEs != nil || result.Terminals != nil || result.HasPRs {
		t.Fatal("quick refresh must preserve data from a full refresh that completed first")
	}
}

func TestCancelledRefreshDoesNotStartOperations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Nil dependencies would panic if a cancelled refresh attempted work.
	local := LocalRefresh(ctx, nil, nil, nil, nil, nil, []string{"sandbox"})
	network := NetworkRefresh(ctx, nil, nil, nil, nil, nil, nil, provider.CLIAvailable, []string{"sandbox"})
	if local.Err != context.Canceled || network.Err != context.Canceled {
		t.Fatal("cancelled refresh was not rejected")
	}
}
