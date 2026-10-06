package gui

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/mdelapenya/biomelab/internal/git"
)

// Native drag dispatch retains the selected Draggable after the initial hit
// test. Benchmark its synchronous production layout work, separately from
// OS-style window resizing. Headless timings exclude native GPU paint latency.
func BenchmarkWorkspaceResize(b *testing.B) {
	for _, count := range []int{5, 150} {
		for _, view := range []ViewMode{ViewKanban, ViewList, ViewGrid} {
			for _, window := range []bool{false, true} {
				kind := "divider"
				if window {
					kind = "window"
				}
				b.Run(fmt.Sprintf("cards%d/view%d/%s", count, view, kind), func(b *testing.B) {
					fa := test.NewApp()
					defer fa.Quit()
					fa.Settings().SetTheme(newBiomeTheme(VariantLight))
					defer applyDarkPalette()
					s := workspaceState()
					original := append([]git.Worktree(nil), s.Worktrees[1:]...)
					s.Worktrees = s.Worktrees[:1]
					for i := 0; i < count; i++ {
						wt := original[i%len(original)]
						branch := wt.Branch
						wt.Branch = fmt.Sprintf("%s-%d", branch, i)
						wt.Path = fmt.Sprintf("%s-%d", wt.Path, i)
						if pr, ok := s.PRs[branch]; ok {
							s.PRs[wt.Branch] = pr
						}
						s.Worktrees = append(s.Worktrees, wt)
					}
					s.ViewMode = view
					d := NewDashboard(s)
					d.inspectorOpen = view != ViewKanban
					d.Rebuild()
					rp := NewRepoPanel(nil, nil)
					shell := newShellLayout(rp.Content(), d.Content(), nil).(*fyne.Container)
					w := fa.NewWindow("Resize benchmark")
					defer w.Close()
					w.SetPadded(false)
					w.SetContent(shell)
					w.Resize(fyne.NewSize(1440, 900))
					w.Show()
					divider := shell.Objects[2].(*shellDivider)
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						delta := float32(1)
						if i%2 != 0 {
							delta = -1
						}
						if window {
							w.Resize(fyne.NewSize(1440+delta, 900))
						} else {
							divider.Dragged(&fyne.DragEvent{Dragged: fyne.Delta{DX: delta}})
						}
					}
					b.StopTimer()
				})
			}
		}
	}
}
