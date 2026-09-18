//go:build docs_screenshots

package gui

import (
	"image/png"
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"github.com/mdelapenya/biomelab/internal/agent"
	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/provider"
)

// TestDocsCapture renders documentation assets from real GUI widgets and sample data.
// Run explicitly with -tags docs_screenshots; normal test runs do not write assets.
func TestDocsCapture(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	root := "/projects/biomelab"
	mode := config.ModeEntry{Type: "regular"}
	state := &RepoState{
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
	for _, variant := range []ThemeVariant{VariantDark, VariantLight} {
		app.Settings().SetTheme(newBiomeTheme(variant))
		dashboard := NewDashboard(state)
		repos := NewRepoPanel([]*RepoGroup{{Path: root, Name: "example/biomelab", Modes: []config.ModeEntry{mode}, LinkedWorktreeCount: 5}}, nil)
		split := container.NewHSplit(repos.Content(), dashboard.Content())
		split.Offset = .20
		w := app.NewWindow("BiomeLab")
		w.SetContent(split)
		w.Resize(fyne.NewSize(1440, 720))
		w.Show()
		path := "../../website/img/dashboard-" + string(variant) + ".png"
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, w.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		w.Close()
	}
}
