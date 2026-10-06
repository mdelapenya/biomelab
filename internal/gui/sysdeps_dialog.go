package gui

import (
	"image/color"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/sysdeps"
)

// sysDepsDialogSize is the preferred initial size for the dependencies modal.
// Wider so action buttons sit comfortably to the right of long version
// strings; tall enough to render every primary check plus the optional
// section without the inner VScroll having to engage.
var sysDepsDialogSize = fyne.NewSize(760, 600)

// showSysDepsDialog opens the System Dependencies modal, listing each Check
// with a status dot, version, action buttons, and an "Optional tools"
// expander for opt-in tools that aren't installed.
func (a *App) showSysDepsDialog() dialog.Dialog {
	var d *dialog.CustomDialog
	body := container.NewVBox()
	closed := false

	show := func(reps []sysdeps.Reported) {
		if closed {
			return
		}
		body.Objects = []fyne.CanvasObject{a.buildSysDepsContent(reps)}
		body.Refresh()
	}

	refresh := func(force bool) {
		a.requestSysdepsRefresh(force, show)
	}

	if raw, ok := a.sysdepsCache.Peek(); ok {
		show(a.visibleSysDeps(raw))
	} else {
		body.Objects = []fyne.CanvasObject{widget.NewLabel("Checking dependencies…")}
	}

	recheck := widget.NewButton("Re-check", func() { refresh(true) })
	recheck.Importance = widget.HighImportance
	footer := container.NewBorder(nil, nil, recheck, nil)

	// No keyCap overlay here: dialog.NewCustom already dismisses on Escape
	// via its own dismiss button, and a transparent Stack overlay would sit
	// on top of the Copy/Docs/Re-check buttons and starve them of hover
	// feedback. Buttons inside the content need a clear path to the cursor.
	content := container.NewBorder(nil, footer, nil, nil, container.NewVScroll(body))

	d = dialog.NewCustom("System Dependencies", "Close", content, a.window)
	d.SetOnClosed(func() { closed = true })
	d.Resize(sysDepsDialogSize)
	d.Show()
	refresh(false)
	return d
}

// loadConfigForSysDeps snapshots the already loaded repositories. Rendering
// dependencies must not reread the config file on the event loop.
func (a *App) loadConfigForSysDeps() *config.Config {
	cfg := &config.Config{Repos: make([]config.RepoEntry, 0, len(a.repos))}
	for _, re := range a.repos {
		if re != nil && re.group != nil {
			cfg.Repos = append(cfg.Repos, config.RepoEntry{Path: re.group.Path, Modes: re.group.Modes})
		}
	}
	return cfg
}

func (a *App) visibleSysDeps(raw []sysdeps.Reported) []sysdeps.Reported {
	required := make(map[string]bool, 2)
	for _, re := range a.repos {
		if re == nil || re.state == nil {
			continue
		}
		switch re.state.Provider {
		case provider.ProviderGitHub:
			required["gh"] = true
		case provider.ProviderGitLab:
			required["glab"] = true
		}
	}
	return sysdeps.ApplyVisibility(
		sysdeps.ApplySuppressionForRequired(raw, required),
		a.loadConfigForSysDeps(),
	)
}

// buildSysDepsContent assembles the dialog body from a probed Reported list:
// primary checks first, then a collapsed "Optional tools" accordion when
// there are missing optional tools.
func (a *App) buildSysDepsContent(reps []sysdeps.Reported) fyne.CanvasObject {
	primary, optional := sysdeps.Partition(reps)

	items := make([]fyne.CanvasObject, 0, len(primary)*2+4)
	for i, r := range primary {
		if i > 0 {
			items = append(items, widget.NewSeparator())
		}
		items = append(items, a.buildSysDepsRow(r))
	}

	if len(optional) > 0 {
		items = append(items, widget.NewSeparator())
		heading := monoText("Optional tools", colorGray, true)
		heading.TextSize = scaledSize(11)
		items = append(items, heading)
		for i, r := range optional {
			if i > 0 {
				items = append(items, widget.NewSeparator())
			}
			items = append(items, a.buildSysDepsRow(r))
		}
	}

	return container.NewVBox(items...)
}

// buildSysDepsRow renders one check as a stacked block:
//
//	● re_gent                                      [Copy install cmd] [Docs]
//	    re_gent version dev (commit: unknown)              (smaller)
//	    Surface AI agent audit trails on worktree cards.   (smaller, dim)
//
// The version sits on its own line at name-size minus two so a long version
// string never crowds the action buttons. Reason/Note is one step smaller
// still and dimmer. Both indent slightly so they read as belonging to the
// name above them.
func (a *App) buildSysDepsRow(r sysdeps.Reported) fyne.CanvasObject {
	dot := monoText(statusDot(r.Result.Status), statusColor(r.Result.Status), true)
	dot.TextSize = scaledSize(14)

	name := monoText(r.Check.DisplayName, colorForeground, true)

	header := container.NewHBox(dot, name)
	actions := a.buildSysDepsActions(r)
	top := container.NewBorder(nil, nil, header, actions)

	rows := []fyne.CanvasObject{top}

	if metaStr := rowMetaText(r); metaStr != "" {
		metaColor := colorDimGray
		if r.Result.Status == sysdeps.StatusOK {
			metaColor = colorGray
		}
		meta := monoText(metaStr, metaColor, false)
		meta.TextSize = scaledSize(12)
		rows = append(rows, indented(meta))
	}

	noteStr := r.Result.Note
	if noteStr == "" {
		noteStr = r.Check.Reason
	}
	if noteStr != "" {
		note := monoText(noteStr, colorGray, false)
		note.TextSize = scaledSize(11)
		rows = append(rows, indented(note))
	}

	return container.NewVBox(rows...)
}

// indented returns o wrapped with a left gutter so secondary row lines
// (version, reason) read as belonging to the name above them. The gutter
// width roughly matches the dot+space prefix on the header line.
func indented(o fyne.CanvasObject) fyne.CanvasObject {
	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(scaledSize(18), 0))
	return container.NewBorder(nil, nil, spacer, nil, o)
}

// rowMetaText picks the right hint to display after the tool name. Version
// wins when known; otherwise we fall back to a short status word so the row
// is informative even for tools that don't ship a version probe.
func rowMetaText(r sysdeps.Reported) string {
	if r.Result.Version != "" {
		return r.Result.Version
	}
	switch r.Result.Status {
	case sysdeps.StatusMissing:
		return "not installed"
	case sysdeps.StatusNA:
		return "n/a"
	case sysdeps.StatusDegraded:
		return "needs attention"
	default:
		return ""
	}
}

// buildSysDepsActions renders the per-row affordances: a "Copy" button for
// the install hint and a "Docs" hyperlink. Returns an empty container when
// neither applies so the row layout stays consistent.
func (a *App) buildSysDepsActions(r sysdeps.Reported) fyne.CanvasObject {
	var btns []fyne.CanvasObject

	wantsCopy := r.Check.InstallHint != "" &&
		(r.Result.Status == sysdeps.StatusMissing || r.Result.Status == sysdeps.StatusDegraded)
	if wantsCopy {
		hint := r.Check.InstallHint
		copyBtn := widget.NewButton("Copy install cmd", func() {
			if a.fyneApp != nil && a.fyneApp.Clipboard() != nil {
				a.fyneApp.Clipboard().SetContent(hint)
			}
		})
		btns = append(btns, copyBtn)
	}

	if r.Check.DocsURL != "" {
		if u, err := url.Parse(r.Check.DocsURL); err == nil {
			btns = append(btns, widget.NewHyperlink("Docs", u))
		}
	}

	if len(btns) == 0 {
		return container.NewHBox()
	}
	return container.NewHBox(btns...)
}

// statusDot returns the glyph for a given status, mapping to the dot palette
// used elsewhere in the GUI (● filled, ◐ half, ○ outline, – em-dash for N/A).
func statusDot(s sysdeps.Status) string {
	switch s {
	case sysdeps.StatusOK:
		return "●"
	case sysdeps.StatusDegraded:
		return "◐"
	case sysdeps.StatusMissing:
		return "○"
	case sysdeps.StatusNA:
		return "–"
	default:
		return "?"
	}
}

// statusColor returns the palette color for a given status. Kept in sync
// with the rest of the GUI: green=ok, yellow=degraded, red=missing,
// dimGray=n/a.
func statusColor(s sysdeps.Status) color.Color {
	switch s {
	case sysdeps.StatusOK:
		return colorGreen
	case sysdeps.StatusDegraded:
		return colorYellow
	case sysdeps.StatusMissing:
		return colorRed
	case sysdeps.StatusNA:
		return colorDimGray
	default:
		return colorGray
	}
}

// buildDepsBanner returns a banner that stays hidden until an asynchronous
// probe finds a missing or degraded primary dependency. Optional tools never
// trigger it; they live in the dialog's expander.
func (a *App) buildDepsBanner() fyne.CanvasObject {
	a.sysdepsBanner = container.NewVBox()
	a.sysdepsBanner.Hide()
	if raw, ok := a.sysdepsCache.Peek(); ok {
		a.updateSysdepsBanner(a.visibleSysDeps(raw))
	}
	return a.sysdepsBanner
}

func (a *App) updateSysdepsBanner(reps []sysdeps.Reported) {
	if a.sysdepsBanner == nil {
		return
	}
	primary, _ := sysdeps.Partition(reps)

	c := sysdeps.Summarize(primary)
	if c.Missing == 0 && c.Degraded == 0 {
		a.sysdepsBanner.Hide()
		return
	}

	var bits []string
	if s := namesByStatus(primary, sysdeps.StatusMissing, "Missing"); s != "" {
		bits = append(bits, s)
	}
	if s := namesByStatus(primary, sysdeps.StatusDegraded, "Needs attention"); s != "" {
		bits = append(bits, s)
	}
	msg := "⚠ " + strings.Join(bits, "; ") + " — some biomelab features will be limited."

	text := monoText(msg, colorYellow, false)
	text.TextSize = scaledSize(11)

	open := widget.NewButton("Open Dependencies", func() {
		a.showSysDepsDialog()
	})

	row := container.NewBorder(nil, nil, nil, open, container.NewPadded(text))
	a.sysdepsBanner.Objects = []fyne.CanvasObject{container.NewPadded(row)}
	a.sysdepsBanner.Show()
	a.sysdepsBanner.Refresh()
}

// namesByStatus returns "<label>: a, b, c" for entries whose status matches
// want. Empty string when no entries match — caller uses that to skip the
// segment in the banner.
func namesByStatus(reps []sysdeps.Reported, want sysdeps.Status, label string) string {
	var names []string
	for _, r := range reps {
		if r.Result.Status == want {
			names = append(names, r.Check.Name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	return label + ": " + strings.Join(names, ", ")
}
