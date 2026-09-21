package terminal

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/mdelapenya/biomelab/internal/process"
)

// maxPPIDHops limits how far we walk up the process tree to avoid infinite
// loops from circular PPID references.
const maxPPIDHops = 20

// Detect finds terminal shell processes and matches their CWDs to the given
// worktree paths. It fetches processes internally via the configured Lister.
func (d *Detector) Detect(worktreePaths []string) DetectionResult {
	ctx := context.Background()

	procs, err := d.lister.Processes(ctx)
	if err != nil {
		return DetectionResult{}
	}

	return d.DetectFromProcesses(procs, worktreePaths)
}

// DetectFromProcesses matches pre-fetched processes against known shell
// patterns, walks their PPID chains to identify terminal emulator ancestors,
// and matches shell CWDs to worktree paths. Use this when sharing a single
// process snapshot across multiple detectors.
func (d *Detector) DetectFromProcesses(procs []process.Info, worktreePaths []string) DetectionResult {
	return d.DetectFromProcessesContext(context.Background(), procs, worktreePaths)
}

// DetectFromProcessesContext allows refresh cancellation during process enrichment.
func (d *Detector) DetectFromProcessesContext(ctx context.Context, procs []process.Info, worktreePaths []string) DetectionResult {

	// Build PID→Info lookup for O(1) PPID walks.
	byPID := make(map[int32]process.Info, len(procs))
	for _, p := range procs {
		if ctx.Err() != nil {
			return nil
		}
		byPID[p.PID] = p
	}

	// Filter for shell processes.
	type shellProc struct {
		process.Info
		emulatorKind Kind
		emulatorPID  int32
	}
	var shells []shellProc
	for _, p := range procs {
		if ctx.Err() != nil {
			return nil
		}
		name := strings.ToLower(filepath.Base(p.Name))
		if !isShell(name) {
			continue
		}
		// Walk PPID chain to find a terminal emulator ancestor.
		kind, rootPID, found := walkToEmulator(p.PPID, byPID)
		if !found {
			continue
		}
		shells = append(shells, shellProc{
			Info:         p,
			emulatorKind: kind,
			emulatorPID:  rootPID,
		})
	}

	if len(shells) == 0 {
		return DetectionResult{}
	}

	// Enrich shells with CWD (only if not already provided).
	for i := range shells {
		if ctx.Err() != nil {
			return nil
		}
		if shells[i].Cwd == "" {
			process.Enrich(ctx, &shells[i].Info)
		}
	}

	// Clean worktree paths once for comparison.
	cleanPaths := make([]string, len(worktreePaths))
	for i, p := range worktreePaths {
		cleanPaths[i] = canonicalPath(p)
	}

	// Match shell CWD to worktree paths, deduplicating by (wtPath, rootPID).
	type treeKey struct {
		wtPath  string
		rootPID int32
	}
	seen := make(map[treeKey]bool)
	result := make(DetectionResult)

	for _, sh := range shells {
		if sh.Cwd == "" {
			continue
		}
		// Prefer the deepest containing worktree. A shell in a linked
		// worktree must not also be assigned to its parent repository.
		best := containingWorktree(canonicalPath(sh.Cwd), cleanPaths)
		if best < 0 {
			continue
		}
		wtPath := worktreePaths[best]
		tk := treeKey{wtPath: wtPath, rootPID: sh.emulatorPID}
		if seen[tk] {
			continue
		}
		seen[tk] = true
		result[wtPath] = append(result[wtPath], Info{
			Kind: sh.emulatorKind, ShellPID: sh.PID, RootPID: sh.emulatorPID,
		})
	}

	return result
}

// isShell checks if a lowercased process base name matches any shell pattern.
func isShell(name string) bool {
	for _, pat := range ShellPatterns {
		if strings.Contains(name, pat) {
			return true
		}
	}
	return false
}

// walkToEmulator walks up the PPID chain from startPPID looking for a known
// terminal emulator. Returns the emulator Kind, its PID, and whether one was found.
func walkToEmulator(startPPID int32, byPID map[int32]process.Info) (Kind, int32, bool) {
	visited := make(map[int32]bool)
	pid := startPPID
	for range maxPPIDHops {
		if pid <= 1 || visited[pid] {
			break
		}
		visited[pid] = true

		p, ok := byPID[pid]
		if !ok {
			break
		}

		name := strings.ToLower(filepath.Base(p.Name))
		for _, ep := range EmulatorPatterns {
			for _, pat := range ep.Patterns {
				if strings.Contains(name, pat) {
					return ep.Kind, p.PID, true
				}
			}
		}

		pid = p.PPID
	}
	return "", 0, false
}

// canonicalPath resolves aliases such as macOS /var -> /private/var when the
// path is accessible, retaining lexical matching for synthetic/unreadable paths.
func canonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(path)
}

func containingWorktree(cwd string, paths []string) int {
	best := -1
	for i, path := range paths {
		rel, err := filepath.Rel(path, cwd)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if best < 0 || len(path) > len(paths[best]) {
			best = i
		}
	}
	return best
}
