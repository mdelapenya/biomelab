package gui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestFetchPRRequiresNonblankReference(t *testing.T) {
	for _, action := range []string{"Enter", "Fetch"} {
		t.Run(action, func(t *testing.T) {
			fa := test.NewApp()
			defer fa.Quit()
			w := fa.NewWindow("fetch PR")
			defer w.Close()
			w.SetContent(container.NewStack())
			w.Resize(fyne.NewSize(900, 700))
			w.Show()
			closed := 0
			var references []string
			showFetchPRInput(w, func() { closed++ }, func(reference string) { references = append(references, reference) })
			entry, ok := w.Canvas().Focused().(*dialogEntry)
			if !ok {
				t.Fatal("PR reference entry not focused")
			}
			var fetch *dialogButton
			walkPolish(w.Canvas().Overlays().Top(), func(o fyne.CanvasObject) {
				if b, ok := o.(*dialogButton); ok && b.Text == "Fetch" {
					fetch = b
				}
			})
			if fetch == nil {
				t.Fatal("Fetch button missing")
			}
			c := &keyboardCanvas{Canvas: w.Canvas()}
			for _, blank := range []string{"", " \t "} {
				entry.SetText(blank)
				if !fetch.Disabled() {
					t.Fatal("blank Fetch enabled")
				}
				test.Tap(fetch)
				w.Canvas().Focus(entry)
				c.press(fyne.KeyReturn)
				if closed != 0 || len(references) != 0 || w.Canvas().Overlays().Top() == nil {
					t.Fatal("blank reference submitted or dismissed the dialog")
				}
			}
			entry.SetText("  owner/repo#93  ")
			if fetch.Disabled() {
				t.Fatal("nonblank Fetch remained disabled")
			}
			if action == "Fetch" {
				test.Tap(fetch)
			} else {
				w.Canvas().Focus(entry)
				c.press(fyne.KeyReturn)
			}
			if closed != 1 || len(references) != 1 || references[0] != "owner/repo#93" || w.Canvas().Overlays().Top() != nil {
				t.Fatalf("nonblank submission closed=%d references=%v", closed, references)
			}
		})
	}
}

func TestSandboxRequiresAgentOrKitsBeforeContinuing(t *testing.T) {
	for _, action := range []string{"agent Enter", "kits Enter", "Continue"} {
		for _, choice := range []string{"agent", "kits"} {
			t.Run(action+"/"+choice, func(t *testing.T) {
				fa := test.NewApp()
				defer fa.Quit()
				w := fa.NewWindow("new sandbox")
				defer w.Close()
				w.SetContent(container.NewStack())
				w.Resize(fyne.NewSize(900, 700))
				w.Show()
				closed, submitted := 0, 0
				var gotAgent string
				var gotKits bool
				showAgentInput(w, func() { closed++ }, func(agent string, kits bool) {
					submitted++
					gotAgent, gotKits = agent, kits
				})
				var agent, kits *dialogSelect
				var next *dialogButton
				walkSetupContent(w.Canvas().Overlays().Top().(*widget.PopUp).Content, func(o fyne.CanvasObject) {
					switch control := o.(type) {
					case *dialogSelect:
						if agent == nil {
							agent = control
						} else {
							kits = control
						}
					case *dialogButton:
						if control.Text == "Continue" {
							next = control
						}
					}
				})
				if agent == nil || kits == nil || next == nil {
					t.Fatal("agent, kits choice, or Continue missing")
				}
				if !next.Disabled() || kits.Selected != "No" {
					t.Fatal("missing agent enabled Continue")
				}
				test.Tap(next)
				agent.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
				kits.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
				if closed != 0 || submitted != 0 || w.Canvas().Overlays().Top() == nil {
					t.Fatal("missing agent submitted or dismissed the dialog")
				}
				kits.SetSelected("Yes")
				if next.Disabled() {
					t.Fatal("kit setup requires an unnecessary built-in agent")
				}
				kits.SetSelected("No")
				if !next.Disabled() {
					t.Fatal("clearing kit setup left Continue enabled")
				}
				if choice == "agent" {
					agent.SetSelected("shell")
				} else {
					kits.SetSelected("Yes")
				}
				if next.Disabled() {
					t.Fatal("valid choice left Continue disabled")
				}
				switch action {
				case "agent Enter":
					agent.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
				case "kits Enter":
					kits.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
				case "Continue":
					test.Tap(next)
				}
				wantAgent := ""
				if choice == "agent" {
					wantAgent = "shell"
				}
				if closed != 1 || submitted != 1 || gotAgent != wantAgent || gotKits != (choice == "kits") {
					t.Fatalf("closed=%d submitted=%d agent=%q kits=%v", closed, submitted, gotAgent, gotKits)
				}
			})
		}
	}
}
