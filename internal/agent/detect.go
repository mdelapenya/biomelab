package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// ProcessLister abstracts process enumeration for testability.
type ProcessLister interface {
	Processes(ctx context.Context) ([]ProcessInfo, error)
}

// ProcessInfo holds the data we need from each OS process.
type ProcessInfo struct {
	PID     int32
	PPID    int32
	Name    string
	Cmdline string
	Cwd     string
	Status  string
	Created time.Time
}

// osProcessLister uses gopsutil to enumerate real processes.
type osProcessLister struct{}

func (o *osProcessLister) Processes(ctx context.Context) ([]ProcessInfo, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}

	var result []ProcessInfo
	for _, p := range procs {
		name, err := p.NameWithContext(ctx)
		if err != nil {
			continue
		}
		var cmdline string
		if cl, err := p.CmdlineWithContext(ctx); err == nil {
			cmdline = cl
		}
		result = append(result, ProcessInfo{
			PID:     p.Pid,
			Name:    name,
			Cmdline: cmdline,
		})
	}
	return result, nil
}

// enrichProcess fills in Cwd, Status, and Created for a process.
func enrichProcess(ctx context.Context, info *ProcessInfo) {
	p, err := process.NewProcess(info.PID)
	if err != nil {
		return
	}
	if cwd, err := p.CwdWithContext(ctx); err == nil {
		info.Cwd = cwd
	}
	if statuses, err := p.StatusWithContext(ctx); err == nil && len(statuses) > 0 {
		info.Status = strings.Join(statuses, ",")
	}
	if createTime, err := p.CreateTimeWithContext(ctx); err == nil {
		info.Created = time.UnixMilli(createTime)
	}
	if ppid, err := p.PpidWithContext(ctx); err == nil {
		info.PPID = ppid
	}
}

// Detector finds coding agent processes and matches them to worktree paths.
type Detector struct {
	lister ProcessLister
}

// NewDetector creates a Detector using real system processes.
func NewDetector() *Detector {
	return &Detector{lister: &osProcessLister{}}
}

// NewDetectorWithLister creates a Detector with a custom process lister (for testing).
func NewDetectorWithLister(lister ProcessLister) *Detector {
	return &Detector{lister: lister}
}

// Detect finds known coding agent processes and matches their CWDs to the given worktree paths.
func (d *Detector) Detect(worktreePaths []string) DetectionResult {
	ctx := context.Background()

	procs, err := d.lister.Processes(ctx)
	if err != nil {
		return DetectionResult{}
	}

	// Filter to agent processes only.
	// Match against both the process name and the command line, since some agents
	// (e.g. gemini) run as Node.js scripts where the process name is "node".
	var agents []ProcessInfo
	for _, p := range procs {
		name := strings.ToLower(filepath.Base(p.Name))
		cmdline := strings.ToLower(p.Cmdline)
		matched := false
		for kind, patterns := range ProcessPatterns {
			for _, pat := range patterns {
				if strings.Contains(name, pat) || strings.Contains(cmdline, pat) {
					p.Name = string(kind) // normalize to kind name
					agents = append(agents, p)
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
	}

	if len(agents) == 0 {
		return DetectionResult{}
	}

	// Enrich with CWD, state, and start time (only if not already provided).
	for i := range agents {
		if agents[i].Cwd == "" {
			enrichProcess(ctx, &agents[i])
		}
	}

	// Identify parent-child pairs: when a CLI agent spawns a child with the
	// same name, mark the child as a subagent instead of dropping it.
	// Independent sessions of the same agent kind are preserved.
	agentPIDs := make(map[int32]string, len(agents)) // PID → normalized kind name
	for _, a := range agents {
		agentPIDs[a.PID] = a.Name
	}
	subAgentPIDs := make(map[int32]bool, len(agents))
	for _, a := range agents {
		if parentKind, ok := agentPIDs[a.PPID]; ok && parentKind == a.Name {
			subAgentPIDs[a.PID] = true
		}
	}

	// Match to worktree paths.
	result := make(DetectionResult)
	for _, a := range agents {
		if a.Cwd == "" {
			continue
		}
		cwd := filepath.Clean(a.Cwd)
		for _, wtPath := range worktreePaths {
			if filepath.Clean(wtPath) == cwd {
				started := ""
				if !a.Created.IsZero() {
					started = a.Created.Format("Mon 2 Jan 15:04:05 2006")
				}
				result[wtPath] = append(result[wtPath], Info{
					Kind:       Kind(a.Name),
					PID:        fmt.Sprintf("%d", a.PID),
					State:      a.Status,
					Started:    started,
					IsSubAgent: subAgentPIDs[a.PID],
				})
			}
		}
	}

	// Sort so parents appear before subagents.
	for wtPath, infos := range result {
		slices.SortStableFunc(infos, func(a, b Info) int {
			if a.IsSubAgent != b.IsSubAgent {
				if a.IsSubAgent {
					return 1
				}
				return -1
			}
			return 0
		})
		result[wtPath] = infos
	}

	return result
}
