package gui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/kits"
	"github.com/mdelapenya/biomelab/internal/ops"
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
			walkSetupContent(requirePopup(t, win.Canvas().Overlays().Top()).Content, func(obj fyne.CanvasObject) {
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
				sel.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
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

func TestNewSandboxModeUsesRepositoryIdentity(t *testing.T) {
	root := t.TempDir()
	a := newSandboxMode(filepath.Join(root, "one", "widget"), "claude")
	b := newSandboxMode(filepath.Join(root, "two", "widget"), "claude")
	if a.SandboxName == "" || b.SandboxName == "" || a.SandboxName == b.SandboxName {
		t.Fatalf("repository sandbox names collided: %q and %q", a.SandboxName, b.SandboxName)
	}
}

func TestRegisterExistingSandboxChoiceOwnsModalAndCanCancel(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	for _, action := range []string{"hide", "escape", "confirm"} {
		win := app.NewWindow("setup")
		a := &App{window: win}
		registered, canceled, doneCount := 0, 0, 0
		done := a.openDialog()
		d := showConfirmRegisterExistingSandbox(win, "owner-repo-openclaw", "/workspace/repo", func() {
			done()
			doneCount++
		}, func() { registered++ }, func() { canceled++ })
		a.activeDialog = d
		if !a.dialogOpen {
			t.Fatal("registration choice must suppress global shortcuts")
		}
		switch action {
		case "confirm":
			d.(interface{ Confirm() }).Confirm()
		case "hide":
			d.Hide()
		case "escape":
			var keyCap *dialogKeyCapture
			walkSetupContent(requirePopup(t, win.Canvas().Overlays().Top()).Content, func(obj fyne.CanvasObject) {
				if capture, ok := obj.(*dialogKeyCapture); ok {
					keyCap = capture
				}
			})
			if keyCap == nil {
				t.Fatal("missing Escape capture in registration dialog")
			}
			keyCap.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
		}
		if a.dialogOpen || a.activeDialog != nil {
			t.Fatal("registration choice did not release modal ownership")
		}
		if doneCount != 1 || registered != btoi(action == "confirm") || canceled != btoi(action != "confirm") {
			t.Fatalf("action=%s done=%d registered=%d canceled=%d", action, doneCount, registered, canceled)
		}
		win.Close()
	}
}

func btoi(value bool) int {
	if value {
		return 1
	}
	return 0
}

func TestCustomKitModeReenrollmentDoesNotClaimSelectedKits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repo")
	selected := []kits.Kit{
		{Name: "OpenClaw", Kind: kits.KindSandbox, Directory: "openclaw"},
		{Name: "playwright", Kind: kits.KindMixin},
	}
	if got := sandboxAgentForSelection("claude", selected); got != "OpenClaw" {
		t.Fatalf("selected custom base agent = %q", got)
	}
	if got := sandboxAgentForSelection("claude", nil); got != "claude" {
		t.Fatalf("no-kit agent = %q", got)
	}
	original := registeredSandboxMode(newSandboxMode(path, "OpenClaw"), newSandboxMode(path, "OpenClaw").SandboxName, true, selected)
	if len(original.Kits) != len(selected) {
		t.Fatalf("new creation lost installed kit metadata: %+v", original.Kits)
	}
	cfg := &config.Config{}
	cfg.Add(path, "owner/repo", original)
	if !cfg.RemoveMode(path, original) {
		t.Fatal("could not unregister original mode")
	}
	// Removing the final sandbox mode with x restores host mode, while the
	// sandbox itself remains available for a later registration.
	cfg.Add(path, "owner/repo", config.ModeEntry{Type: "regular"})
	resumed := registeredSandboxMode(newSandboxMode(path, "OpenClaw"), original.SandboxName, false, selected)
	if resumed.Agent != original.Agent || resumed.SandboxName != original.SandboxName || len(resumed.Kits) != 0 {
		t.Fatalf("re-enrolled mode claimed selected kits or changed identity: %+v", resumed)
	}
	cfg.Add(path, "owner/repo", resumed)
	if got := cfg.Repos[0].Modes; len(got) != 1 || got[0].Type != "sandbox" || got[0].Agent != "OpenClaw" || len(got[0].Kits) != 0 {
		t.Fatalf("persisted re-enrollment = %+v", got)
	}

	trusted := registeredSandboxMode(original, original.SandboxName, false, nil)
	if !reflect.DeepEqual(trusted.Kits, original.Kits) {
		t.Fatal("reusing a still-registered mode should preserve trusted installed metadata")
	}
}

func TestCustomKitReenrollmentWithFakeSandboxCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake sbx executable uses a POSIX shell")
	}
	app := test.NewApp()
	defer app.Quit()
	win := app.NewWindow("setup")
	defer win.Close()

	dir := t.TempDir()
	listingFile := filepath.Join(dir, "listing.json")
	createLog := filepath.Join(dir, "create.log")
	script := `#!/bin/sh
case "$1" in
  ls) cat "$TEST_LIST_FILE" ;;
  create) printf '%s\n' "$@" >> "$TEST_CREATE_LOG" ;;
  *) exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "sbx"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_LIST_FILE", listingFile)
	t.Setenv("TEST_CREATE_LOG", createLog)
	writeListing := func(contents string) {
		t.Helper()
		if err := os.WriteFile(listingFile, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	repoPath := filepath.Join(dir, "repo")
	selected := []kits.Kit{
		{Name: "OpenClaw", Kind: kits.KindSandbox, Directory: "openclaw"},
		{Name: "playwright", Kind: kits.KindMixin},
	}
	baseRef, mixinRefs := kitSelectionRefs(selected)
	mode := newSandboxMode(repoPath, "OpenClaw")
	writeListing(`{"sandboxes":[]}`)
	name, created, err := ops.EnsureSandboxWithKit("owner/repo", repoPath, mode.SandboxName, mode.Agent, baseRef, mixinRefs)
	if err != nil || !created || name != mode.SandboxName {
		t.Fatalf("initial creation name=%q created=%v err=%v", name, created, err)
	}
	initialCreate, err := os.ReadFile(createLog)
	if err != nil || !strings.Contains(string(initialCreate), baseRef) || !strings.Contains(string(initialCreate), mixinRefs[0]) {
		t.Fatalf("initial sbx create did not install selected base/mixin: %q (%v)", initialCreate, err)
	}
	cfg := &config.Config{}
	mode = registeredSandboxMode(mode, name, true, selected)
	cfg.Add(repoPath, "owner/repo", mode)
	if !cfg.RemoveMode(repoPath, mode) {
		t.Fatal("x did not remove sandbox registration")
	}
	cfg.Add(repoPath, "owner/repo", config.ModeEntry{Type: "regular"})
	writeListing(`{"sandboxes":[{"name":"` + name + `","status":"stopped"}]}`)

	_, _, err = ops.EnsureSandboxWithKit("owner/repo", repoPath, name, "OpenClaw", baseRef, mixinRefs)
	var existing *ops.ExistingSandboxWithKitsError
	if !errors.As(err, &existing) || existing.Name != name {
		t.Fatalf("expected explicit reuse choice for existing custom base: %v", err)
	}
	if current, readErr := os.ReadFile(createLog); readErr != nil || string(current) != string(initialCreate) {
		t.Fatalf("existing lookup recreated sandbox: %q (%v)", current, readErr)
	}
	registered := false
	showChoice := func(accept bool) {
		t.Helper()
		d := showConfirmRegisterExistingSandbox(win, existing.Name, repoPath, func() {}, func() {
			resolved, resolveErr := ops.RegisterExistingSandbox(existing.Name)
			if resolveErr != nil {
				t.Fatalf("register exact matched sandbox: %v", resolveErr)
			}
			cfg.Add(repoPath, "owner/repo", registeredSandboxMode(newSandboxMode(repoPath, "OpenClaw"), resolved, false, selected))
			registered = true
		}, func() {})
		if accept {
			d.(interface{ Confirm() }).Confirm()
		} else {
			d.Hide()
		}
	}
	showChoice(false)
	if registered || len(cfg.Repos[0].Modes) != 1 || cfg.Repos[0].Modes[0].Type != "regular" {
		t.Fatalf("cancel changed config: %+v", cfg.Repos)
	}
	showChoice(true)
	if !registered || len(cfg.Repos[0].Modes) != 1 {
		t.Fatalf("registration failed: %+v", cfg.Repos)
	}
	got := cfg.Repos[0].Modes[0]
	if got.Type != "sandbox" || got.Agent != "OpenClaw" || got.SandboxName != name || len(got.Kits) != 0 {
		t.Fatalf("registered custom mode or kit metadata incorrect: %+v", got)
	}
	if current, readErr := os.ReadFile(createLog); readErr != nil || string(current) != string(initialCreate) {
		t.Fatalf("reuse invoked sbx create/install: %q (%v)", current, readErr)
	}
}
