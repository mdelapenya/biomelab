package ide

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/mdelapenya/gwaim/internal/process"
)

// Detector finds IDE processes and matches them to worktree paths.
type Detector struct {
	lister process.Lister
}

// NewDetector creates a Detector using real system processes.
func NewDetector() *Detector {
	return &Detector{lister: &process.OSLister{}}
}

// NewDetectorWithLister creates a Detector with a custom process lister (for testing).
func NewDetectorWithLister(lister process.Lister) *Detector {
	return &Detector{lister: lister}
}

// Detect finds known IDE processes and matches them to the given worktree paths.
// It fetches processes internally via the configured Lister.
func (d *Detector) Detect(worktreePaths []string) DetectionResult {
	ctx := context.Background()

	procs, err := d.lister.Processes(ctx)
	if err != nil {
		return DetectionResult{}
	}

	return d.DetectFromProcesses(procs, worktreePaths)
}

// DetectFromProcesses matches pre-fetched processes against known IDE patterns
// and worktree paths. Use this when sharing a single process snapshot across
// multiple detectors.
func (d *Detector) DetectFromProcesses(procs []process.Info, worktreePaths []string) DetectionResult {
	ctx := context.Background()

	// Filter to IDE processes only.
	// Match against process name only (not cmdline) to avoid false positives
	// from generic patterns like "code" appearing in unrelated cmdlines.
	type ideProc struct {
		process.Info
		kind Kind
	}

	var ides []ideProc
	for _, p := range procs {
		name := strings.ToLower(filepath.Base(p.Name))
		matched := false
		for _, pp := range ProcessPatterns {
			for _, pat := range pp.Patterns {
				if strings.Contains(name, pat) {
					ides = append(ides, ideProc{Info: p, kind: pp.Kind})
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
	}

	if len(ides) == 0 {
		return DetectionResult{}
	}

	// Enrich with CWD (only if not already provided).
	for i := range ides {
		if ides[i].Cwd == "" {
			process.Enrich(ctx, &ides[i].Info)
		}
	}

	// Build a PID→kind map for all IDE processes to identify parent-child
	// relationships. Electron-based IDEs (VS Code, Cursor) spawn many helper
	// processes that share the same kind. We group them by process tree so
	// each independent IDE window is one entry. If the user opens two VS Code
	// windows for the same worktree, they see two entries.
	ideKinds := make(map[int32]Kind, len(ides)) // PID → kind
	for _, ide := range ides {
		ideKinds[ide.PID] = ide.kind
	}

	// Find the root of each IDE process tree. A root is an IDE process whose
	// parent is NOT another IDE process of the same kind.
	// rootOf maps each IDE PID → the root PID of its process tree.
	rootOf := make(map[int32]int32, len(ides))
	for _, ide := range ides {
		root := ide.PID
		cur := ide.PID
		ppid := ide.PPID
		for {
			parentKind, isIDE := ideKinds[ppid]
			if !isIDE || parentKind != ide.kind {
				break
			}
			root = ppid
			// Walk up: find the parent's PPID.
			found := false
			for _, other := range ides {
				if other.PID == ppid {
					cur = ppid
					ppid = other.PPID
					found = true
					break
				}
			}
			if !found || cur == ppid {
				break // avoid infinite loop
			}
		}
		rootOf[ide.PID] = root
	}

	// Clean worktree paths once for comparison.
	cleanPaths := make([]string, len(worktreePaths))
	for i, p := range worktreePaths {
		cleanPaths[i] = filepath.Clean(p)
	}

	// Match IDE processes to worktree paths by CWD or cmdline.
	// Group by (worktree, root PID) — each root = one IDE instance.
	type treeKey struct {
		wtPath  string
		rootPID int32
	}
	trees := make(map[treeKey]*Info)
	treePIDs := make(map[treeKey][]int32) // all PIDs in this tree for killing
	var treeOrder []treeKey               // preserves insertion order

	for _, ide := range ides {
		cwd := filepath.Clean(ide.Cwd)
		for j, wtPath := range worktreePaths {
			cleanWT := cleanPaths[j]
			cwdMatch := ide.Cwd != "" && cwd == cleanWT
			cmdlineMatch := strings.Contains(ide.Cmdline, cleanWT)
			if cwdMatch || cmdlineMatch {
				tk := treeKey{wtPath: wtPath, rootPID: rootOf[ide.PID]}
				treePIDs[tk] = append(treePIDs[tk], ide.PID)
				if _, exists := trees[tk]; !exists {
					trees[tk] = &Info{
						Kind: ide.kind,
						PID:  rootOf[ide.PID],
					}
					treeOrder = append(treeOrder, tk)
				}
			}
		}
	}

	result := make(DetectionResult)
	for _, tk := range treeOrder {
		info := trees[tk]
		info.ExtraPIDs = treePIDs[tk]
		result[tk.wtPath] = append(result[tk.wtPath], *info)
	}

	return result
}
