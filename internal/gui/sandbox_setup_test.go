package gui

import (
	"encoding/json"
	"reflect"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/kits"
)

// Walk only structural containers, preserving the actual input widgets rather
// than descending into their renderer implementation.
func walkSetupContent(obj fyne.CanvasObject, visit func(fyne.CanvasObject)) {
	visit(obj)
	if c, ok := obj.(*fyne.Container); ok {
		for _, child := range c.Objects {
			walkSetupContent(child, visit)
		}
	}
}

func TestSandboxPromptAgentAndOptionalKits(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	for _, choice := range []string{"No", "Yes", "Cancel", "Escape"} {
		t.Run(choice, func(t *testing.T) {
			win := app.NewWindow("setup")
			defer win.Close()
			var submitted, cleaned bool
			var gotAgent string
			var gotKits bool
			d := showAgentInput(win, func() { cleaned = true }, func(agent string, addKits bool) {
				submitted, gotAgent, gotKits = true, agent, addKits
			})
			var sel *dialogSelect
			var addKits *dialogSelect
			walkSetupContent(win.Canvas().Overlays().Top().(*widget.PopUp).Content, func(obj fyne.CanvasObject) {
				switch w := obj.(type) {
				case *dialogSelect:
					if sel == nil {
						sel = w
					} else {
						addKits = w
					}
				}
			})
			if sel == nil || addKits == nil {
				t.Fatal("missing agent or kits choice")
			}
			if addKits.Selected != "No" {
				t.Fatal("kit loading should be opt-in")
			}
			sel.SetSelected("claude")
			switch choice {
			case "Cancel":
				d.Hide()
			case "Escape":
				addKits.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
			default:
				addKits.SetSelected(choice)
				d.(*dialog.ConfirmDialog).Confirm()
			}
			if !cleaned {
				t.Fatal("dialog did not clean up")
			}
			if choice == "Cancel" || choice == "Escape" {
				if submitted {
					t.Fatal("cancel must not create or load kits")
				}
			} else if !submitted || gotAgent != "claude" || gotKits != (choice == "Yes") {
				t.Fatalf("submitted=%v agent=%q kits=%v", submitted, gotAgent, gotKits)
			}
		})
	}
}

func TestDialogCheckHandlesSpaceAndEscape(t *testing.T) {
	escaped := false
	check := newDialogCheck("Code Server", nil, func() { escaped = true })
	check.TypedRune(' ')
	if !check.Checked {
		t.Fatal("Space should toggle the focused kit checkbox")
	}
	check.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if !escaped {
		t.Fatal("Escape should invoke the kit dialog cancellation callback")
	}
}

func TestKitInstallReferencesMatchCreationAndPersist(t *testing.T) {
	selected := []kits.Kit{{Name: "web-editor", Directory: "code-server"}, {Name: "playwright"}}
	refs := kitURLs(selected)
	want := []string{"docker.io/sbx/code-server-kit:latest", "docker.io/sbx/playwright-kit:latest"}
	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("references = %v", refs)
	}
	installs := buildKitInstalls(selected)
	data, err := json.Marshal(installs)
	if err != nil {
		t.Fatal(err)
	}
	var loaded []config.KitInstall
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	for i, k := range loaded {
		if k.Reference != refs[i] || k.Ref != "latest" {
			t.Fatalf("persisted kit = %+v", k)
		}
	}
}
