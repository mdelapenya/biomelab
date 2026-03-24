package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mdelapenya/gwaim/internal/agent"
	"github.com/mdelapenya/gwaim/internal/git"
	"github.com/mdelapenya/gwaim/internal/tui"
)

var version = "dev"

func main() {
	var versionFlag bool
	var refreshFlag time.Duration
	flag.BoolVar(&versionFlag, "version", false, "Print version and exit")
	flag.BoolVar(&versionFlag, "v", false, "Print version and exit (shorthand)")
	flag.DurationVar(&refreshFlag, "refresh", 0, "Network refresh interval: how often to fetch from remote and look up PRs (e.g. 30s, 1m). Local state refreshes every 5s regardless.")
	flag.DurationVar(&refreshFlag, "r", 0, "Network refresh interval (shorthand)")
	flag.Parse()

	if versionFlag {
		fmt.Println("gwaim", version)
		return
	}

	refreshInterval := resolveRefreshInterval(refreshFlag)

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	repoRoot, err := git.RepoRoot(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: not a git repository: %v\n", err)
		os.Exit(1)
	}

	repo, err := git.OpenRepository(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	detector := agent.NewDetector()

	model := tui.New(repo, detector, refreshInterval)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// resolveRefreshInterval applies the precedence: CLI flag → GWAIM_REFRESH env → default.
func resolveRefreshInterval(flagVal time.Duration) time.Duration {
	if flagVal != 0 {
		return flagVal
	}
	if val := os.Getenv("GWAIM_REFRESH"); val != "" {
		d, err := time.ParseDuration(val)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid GWAIM_REFRESH value %q: %v\n", val, err)
			os.Exit(1)
		}
		return d
	}
	return tui.DefaultNetworkRefreshInterval
}
