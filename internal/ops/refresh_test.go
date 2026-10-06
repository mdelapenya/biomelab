package ops

import (
	"context"
	"testing"

	"github.com/mdelapenya/biomelab/internal/agent"
	"github.com/mdelapenya/biomelab/internal/ide"
	"github.com/mdelapenya/biomelab/internal/process"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/terminal"
)

type emptyProcessLister struct{}

func (emptyProcessLister) Processes(context.Context) ([]process.Info, error) { return nil, nil }

type detailedRefreshProvider struct {
	provider.PRProvider
	lookups provider.PRLookupResult
}

func (p detailedRefreshProvider) FetchPRsDetailedContext(context.Context, string, []string) provider.PRLookupResult {
	return p.lookups
}

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

func TestNetworkRefreshCarriesCLIAndDetailedPRLookups(t *testing.T) {
	_, repo := newOpsRepo(t, nil)
	lister := emptyProcessLister{}
	want := provider.PRLookupResult{"main": {Info: &provider.PRInfo{Number: 42}}}
	result := NetworkRefresh(context.Background(), repo,
		agent.NewDetectorWithLister(lister), ide.NewDetectorWithLister(lister),
		terminal.NewDetectorWithLister(lister), lister,
		detailedRefreshProvider{lookups: want}, provider.CLIAvailable, nil)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if !result.HasCLIAvail || result.CLIAvail != provider.CLIAvailable {
		t.Fatalf("CLI availability omitted or wrong: %+v", result)
	}
	if !result.HasPRs || !result.HasPRLookups || result.PRLookups["main"].Info == nil || result.PRLookups["main"].Info.Number != 42 {
		t.Fatalf("detailed PR result omitted: %+v", result)
	}
}
