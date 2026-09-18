// Package ops contains business operations used by the desktop GUI,
// independent of widget rendering and input handling.
package ops

import (
	"context"

	"github.com/mdelapenya/biomelab/internal/agent"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/ide"
	"github.com/mdelapenya/biomelab/internal/process"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/sandbox"
	"github.com/mdelapenya/biomelab/internal/terminal"
)

// RefreshResult carries updated worktree, agent, IDE, terminal, and PR data.
type RefreshResult struct {
	Worktrees      []git.Worktree
	Agents         agent.DetectionResult
	IDEs           ide.DetectionResult
	Terminals      terminal.DetectionResult
	PRs            provider.PRResult
	HasPRs         bool
	Err            error
	FetchErr       error
	SandboxStatus  sandbox.Status
	HasSbxStatus   bool // true when sandbox status was actually checked
	AllSbxStatuses map[string]sandbox.Status
	SbxClientVer   string
	SbxServerVer   string
	// SbxMatchedName is the candidate that matched a running/stopped sandbox
	// in sbx ls (empty if none matched). Lets callers reconcile config when
	// the stored sandbox name differs from what sbx actually reports.
	SbxMatchedName string
	// Generation is the repo mutation counter captured under the same lock
	// that produced Worktrees. Apply-time staleness checks drop results
	// whose Generation predates the last applied snapshot — this prevents
	// a long-running refresh from resurrecting a card the user just deleted.
	Generation uint64
}

// CLICheckResult carries the CLI availability check result.
type CLICheckResult struct {
	Avail provider.CLIAvailability
}

// QuickRefresh loads branch names only — no dirty status, no agents, no network.
// Used for instant first render.
func QuickRefresh(repo *git.Repository) RefreshResult {
	snap, err := repo.SnapshotQuick()
	if err != nil {
		return RefreshResult{Err: err}
	}
	return RefreshResult{
		Worktrees:  snap.Worktrees,
		Agents:     agent.DetectionResult{},
		PRs:        provider.PRResult{},
		Generation: snap.Generation,
	}
}

// LocalRefresh reads dirty status and detects agents and IDEs — no network I/O.
// sbxCandidates is the ordered list of sandbox names to check (first match
// wins); pass nil or empty to skip the sandbox status check.
func LocalRefresh(
	repo *git.Repository,
	detector *agent.Detector,
	ideDetector *ide.Detector,
	termDetector *terminal.Detector,
	procLister process.Lister,
	sbxCandidates []string,
) RefreshResult {
	snap, err := repo.Snapshot()
	if err != nil {
		return RefreshResult{Err: err}
	}
	wts := snap.Worktrees

	paths := make([]string, len(wts))
	for i, wt := range wts {
		paths[i] = wt.Path
	}

	// Fetch processes once and share across all detectors.
	ctx := context.Background()
	procs, procErr := procLister.Processes(ctx)
	var agents agent.DetectionResult
	var ides ide.DetectionResult
	var terms terminal.DetectionResult
	if procErr == nil {
		agents = detector.DetectFromProcesses(procs, paths)
		ides = ideDetector.DetectFromProcesses(procs, paths)
		terms = termDetector.DetectFromProcesses(procs, paths)
	}

	// Check all sandbox statuses with one sbx ls call, then match against
	// every candidate name so the GUI detects sandboxes created under either
	// biomelab's naming ("<owner>-<repo>-<agent>") or sbx's default
	// ("<repo-dir>-<agent>").
	var sbxStatus sandbox.Status
	var sbxMatched string
	var sbxVer sandbox.VersionInfo
	var allStatuses map[string]sandbox.Status
	if len(sbxCandidates) > 0 {
		statusMap := sandbox.CheckAllStatuses()
		if statusMap != nil {
			allStatuses = make(map[string]sandbox.Status, len(statusMap))
			for k, v := range statusMap {
				allStatuses[k] = v
			}
			if name, s, ok := sandbox.MatchStatus(statusMap, sbxCandidates); ok {
				sbxStatus = s
				sbxMatched = name
			}
		}
		sbxVer = sandbox.Version()
	}

	return RefreshResult{
		Worktrees:      wts,
		Agents:         agents,
		IDEs:           ides,
		Terminals:      terms,
		SandboxStatus:  sbxStatus,
		HasSbxStatus:   len(sbxCandidates) > 0,
		AllSbxStatuses: allStatuses,
		SbxClientVer:   sbxVer.Client,
		SbxServerVer:   sbxVer.Server,
		SbxMatchedName: sbxMatched,
		Generation:     snap.Generation,
	}
}

// NetworkRefresh fetches remote refs and looks up PR status.
// sbxCandidates is the ordered list of sandbox names to check (first match
// wins); pass nil or empty to skip the sandbox status check.
func NetworkRefresh(
	repo *git.Repository,
	detector *agent.Detector,
	ideDetector *ide.Detector,
	termDetector *terminal.Detector,
	procLister process.Lister,
	prProv provider.PRProvider,
	cliAvail provider.CLIAvailability,
	sbxCandidates []string,
) RefreshResult {
	fetchErr := repo.Fetch()

	snap, err := repo.Snapshot()
	if err != nil {
		return RefreshResult{Err: err}
	}
	wts := snap.Worktrees

	paths := make([]string, len(wts))
	branches := make([]string, len(wts))
	for i, wt := range wts {
		paths[i] = wt.Path
		branches[i] = wt.Branch
	}

	ctx := context.Background()
	procs, procErr := procLister.Processes(ctx)
	var agents agent.DetectionResult
	var ides ide.DetectionResult
	var terms terminal.DetectionResult
	if procErr == nil {
		agents = detector.DetectFromProcesses(procs, paths)
		ides = ideDetector.DetectFromProcesses(procs, paths)
		terms = termDetector.DetectFromProcesses(procs, paths)
	}

	var prs provider.PRResult
	if cliAvail == provider.CLIAvailable {
		prs = prProv.FetchPRs(repo.Root(), branches)
	} else {
		prs = make(provider.PRResult)
	}

	var sbxStatus sandbox.Status
	var sbxMatched string
	if len(sbxCandidates) > 0 {
		statusMap := sandbox.CheckAllStatuses()
		if statusMap != nil {
			if name, s, ok := sandbox.MatchStatus(statusMap, sbxCandidates); ok {
				sbxStatus = s
				sbxMatched = name
			}
		}
	}

	return RefreshResult{
		Worktrees:      wts,
		Agents:         agents,
		IDEs:           ides,
		Terminals:      terms,
		PRs:            prs,
		HasSbxStatus:   len(sbxCandidates) > 0,
		HasPRs:         true,
		FetchErr:       fetchErr,
		SandboxStatus:  sbxStatus,
		SbxMatchedName: sbxMatched,
		Generation:     snap.Generation,
	}
}

// CardRefresh refreshes a single worktree card: fetches remotes, detects
// agents/IDEs for the target path only, and looks up PRs for the target branch.
func CardRefresh(
	repo *git.Repository,
	detector *agent.Detector,
	ideDetector *ide.Detector,
	termDetector *terminal.Detector,
	procLister process.Lister,
	prProv provider.PRProvider,
	cliAvail provider.CLIAvailability,
	wtPath, branch string,
) RefreshResult {
	fetchErr := repo.Fetch()

	snap, err := repo.Snapshot()
	if err != nil {
		return RefreshResult{Err: err}
	}

	ctx := context.Background()
	procs, procErr := procLister.Processes(ctx)
	var agents agent.DetectionResult
	var ides ide.DetectionResult
	var terms terminal.DetectionResult
	if procErr == nil {
		agents = detector.DetectFromProcesses(procs, []string{wtPath})
		ides = ideDetector.DetectFromProcesses(procs, []string{wtPath})
		terms = termDetector.DetectFromProcesses(procs, []string{wtPath})
	}

	var prs provider.PRResult
	if cliAvail == provider.CLIAvailable {
		prs = prProv.FetchPRs(repo.Root(), []string{branch})
	} else {
		prs = make(provider.PRResult)
	}

	return RefreshResult{
		Worktrees:  snap.Worktrees,
		Agents:     agents,
		IDEs:       ides,
		Terminals:  terms,
		PRs:        prs,
		HasPRs:     true,
		FetchErr:   fetchErr,
		Generation: snap.Generation,
	}
}

// CheckCLI performs a pre-flight check for the provider's CLI tool.
func CheckCLI(prProv provider.PRProvider) provider.CLIAvailability {
	return prProv.CheckCLI()
}
