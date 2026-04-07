package ide

import (
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// KillProcesses sends SIGTERM to the given PIDs, waits briefly, then
// sends SIGKILL to any survivors. Best-effort: errors are silently ignored
// (the process may have already exited).
func KillProcesses(pids []int32) {
	if len(pids) == 0 {
		return
	}

	// Send SIGTERM to all processes.
	var procs []*process.Process
	for _, pid := range pids {
		p, err := process.NewProcess(pid)
		if err != nil {
			continue
		}
		_ = p.Terminate()
		procs = append(procs, p)
	}

	// Give processes a moment to exit gracefully.
	time.Sleep(500 * time.Millisecond)

	// SIGKILL any survivors.
	for _, p := range procs {
		running, err := p.IsRunning()
		if err != nil || !running {
			continue
		}
		_ = p.Kill()
	}
}
