package main

import (
	"bytes"
	"flag"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"github.com/mdelapenya/biomelab/internal/agent"
	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/gui"
	"github.com/mdelapenya/biomelab/internal/provider"
)

// This helper renders documentation images from the GUI widgets using sample data.
// Fyne's test app provides an offscreen renderer; no desktop window is opened.
func main() {
	outputDir := flag.String("output-dir", "website/img", "Directory for dashboard-dark.png and dashboard-light.png (existing files are overwritten)")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "screenshot-generator: unexpected positional arguments; use -help for usage")
		os.Exit(2)
	}
	// The offscreen driver expects theme updates from a worker goroutine,
	// just as when it is used by Go's test runner.
	result := make(chan error, 1)
	go func() { result <- generate(*outputDir) }()
	if err := <-result; err != nil {
		fmt.Fprintln(os.Stderr, "screenshot-generator:", err)
		os.Exit(1)
	}
}

func generate(outputDir string) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	app := test.NewApp()
	defer app.Quit()
	root := "/projects/biomelab"
	mode := config.ModeEntry{Type: "regular"}
	state := &gui.RepoState{
		ActiveMode: &mode, Provider: provider.ProviderGitHub,
		Worktrees: []git.Worktree{{Path: root, Branch: "main", IsMain: true, Sync: git.SyncUpToDate}},
		PRs:       provider.PRResult{}, Agents: agent.DetectionResult{},
		LastLocalRefresh:   time.Date(2026, 9, 18, 10, 30, 0, 0, time.UTC),
		LastNetworkRefresh: time.Date(2026, 9, 18, 10, 30, 0, 0, time.UTC),
	}
	branches := []string{"fix/old-approach", "feat/task-notes", "feat/sandbox-kits", "fix/terminal-focus", "docs/getting-started"}
	states := []string{"closed", "", "open", "open", "merged"}
	for i, branch := range branches {
		path := root + "/.biomelab-worktrees/" + branch
		state.Worktrees = append(state.Worktrees, git.Worktree{Path: path, Branch: branch, IsDirty: i == 1, Sync: git.SyncUpToDate})
		if states[i] != "" {
			state.PRs[branch] = &provider.PRInfo{Number: 41 + i, Title: branch, State: states[i], CheckStatus: "success", URL: "https://github.com/example/project/pull/41"}
		}
		if i == 3 {
			state.PRs[branch].ReviewStatus = "approved"
		}
		if i == 1 || i == 2 {
			state.Agents[path] = []agent.Info{{Kind: agent.Claude, PID: "1234", State: "S", Started: "10:20"}}
		}
	}
	for _, variant := range []gui.ThemeVariant{gui.VariantDark, gui.VariantLight} {
		app.Settings().SetTheme(gui.NewTheme(variant))
		dashboard := gui.NewDashboard(state)
		repos := gui.NewRepoPanel([]*gui.RepoGroup{{Path: root, Name: "example/biomelab", Modes: []config.ModeEntry{mode}, LinkedWorktreeCount: 5}}, nil)
		split := container.NewHSplit(repos.Content(), dashboard.Content())
		split.Offset = .20
		w := app.NewWindow("BiomeLab")
		w.SetContent(split)
		w.Resize(fyne.NewSize(1440, 720))
		w.Show()
		var encoded bytes.Buffer
		err := png.Encode(&encoded, w.Canvas().Capture())
		w.Close()
		if err != nil {
			return fmt.Errorf("encode %s dashboard: %w", variant, err)
		}
		path := filepath.Join(outputDir, "dashboard-"+string(variant)+".png")
		if err := os.WriteFile(path, encoded.Bytes(), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		fmt.Println(path)
	}
	return nil
}
