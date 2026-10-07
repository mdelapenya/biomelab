package gui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
)

func TestAddRepositoryRequiresNonblankPath(t *testing.T) {
	for _, submission := range []string{"Enter", "Add"} {
		t.Run(submission, func(t *testing.T) {
			fa := test.NewApp()
			defer fa.Quit()
			fa.Settings().SetTheme(newBiomeTheme(VariantDark))
			w := fa.NewWindow("add repository")
			defer w.Close()
			w.SetContent(container.NewStack())
			w.Resize(fyne.NewSize(900, 700))
			w.Show()
			closed := 0
			var paths []string
			showAddRepoInput(w, func() { closed++ }, func(path string) { paths = append(paths, path) })
			entry, ok := w.Canvas().Focused().(*dialogEntry)
			if !ok {
				t.Fatal("repository path entry not focused")
			}
			var add *dialogButton
			walkPolish(w.Canvas().Overlays().Top(), func(o fyne.CanvasObject) {
				if b, ok := o.(*dialogButton); ok && b.Text == "Add" {
					add = b
				}
			})
			if add == nil {
				t.Fatal("Add button missing")
			}
			c := &keyboardCanvas{Canvas: w.Canvas()}
			for _, blank := range []string{"", " \t "} {
				entry.SetText(blank)
				if !add.Disabled() {
					t.Fatal("blank Add enabled")
				}
				test.Tap(add)
				w.Canvas().Focus(entry)
				c.press(fyne.KeyReturn)
				if closed != 0 || len(paths) != 0 || w.Canvas().Overlays().Top() == nil {
					t.Fatal("blank path submitted or dismissed the dialog")
				}
			}
			// Nonblank input reaches the existing path validation, including
			// invalid paths; this form only enforces that a path was supplied.
			entry.SetText("  /nonexistent/repository  ")
			if add.Disabled() {
				t.Fatal("nonblank Add remained disabled")
			}
			if submission == "Add" {
				test.Tap(add)
			} else {
				w.Canvas().Focus(entry)
				c.press(fyne.KeyReturn)
			}
			if closed != 1 || len(paths) != 1 || paths[0] != "/nonexistent/repository" || w.Canvas().Overlays().Top() != nil {
				t.Fatalf("nonblank submission closed=%d paths=%v", closed, paths)
			}
		})
	}
}
