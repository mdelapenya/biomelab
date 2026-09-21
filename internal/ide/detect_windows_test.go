package ide

import (
	"testing"

	"github.com/mdelapenya/biomelab/internal/process"
)

func TestDetectWindowsPathSeparators(t *testing.T) {
	for _, workspace := range []string{`C:\work tree\repo`, `C:/work tree/repo`} {
		for _, argument := range []string{`C:\work tree\repo\.biomelab-worktrees\feature`, `C:/work tree/repo/.biomelab-worktrees/feature`} {
			t.Run(workspace+"_"+argument, func(t *testing.T) {
				worktree := workspace + `/.biomelab-worktrees/feature`
				procs := []process.Info{
					{PID: 100, Name: "Code.exe", Cmdline: `Code.exe "` + argument + `"`, Cwd: `C:\`},
					// Native Windows CLI launcher paths must also be excluded.
					{PID: 200, Name: "Code.exe", Cmdline: `Code.exe C:\Code\resources\app\out\cli.js "` + argument + `"`, Cwd: `C:\`},
				}
				result := NewDetector().DetectFromProcesses(procs, []string{workspace, worktree})
				if len(result[workspace]) != 0 {
					t.Fatalf("nested worktree matched parent: %+v", result)
				}
				if got := result[worktree]; len(got) != 1 || got[0].PID != 100 {
					t.Fatalf("want only IDE window 100 for worktree, got %+v", got)
				}
			})
		}
	}
}
