package gui

import (
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/mdelapenya/biomelab/internal/notes"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/sandbox"
)

func inspectorValue(value string, mono bool) fyne.CanvasObject { return newWrappedValue(value, mono) }
func inspectorProperty(label, value string, mono bool) fyne.CanvasObject {
	return container.New(&inspectorPropertyLayout{}, secondaryText(label), inspectorValue(value, mono))
}
func inspectorGroup(title string, rows ...fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(colorSecondaryBg)
	bg.CornerRadius = scaledSize(radiusCard)
	return container.NewVBox(secondaryText(title), container.NewStack(bg, inset(container.NewVBox(rows...), spaceSM, spaceXS)))
}
func inspectorAction(label string, icon fyne.Resource, primary bool, fn func()) *actionControl {
	c := newActionControl(label, icon, primary, fn)
	c.disabled = fn == nil
	return c
}
func (d *Dashboard) buildInspector() fyne.CanvasObject {
	idx := d.state.SelectedCard
	if idx < 0 || idx >= len(d.state.Worktrees) {
		return container.NewCenter(secondaryText("Select a worktree"))
	}
	wt := d.state.Worktrees[idx]
	title := wt.Branch
	if pr := d.prFor(wt.Branch); pr != nil && pr.Title != "" {
		title = pr.Title
	}
	heading := newMeasuredText(title, colorForeground, true, false, false)
	heading.txt.TextSize = scaledSize(14)
	close := newActionControl("Close", theme.CancelIcon(), false, func() { d.inspectorOpen = false; d.Rebuild() })
	top := container.NewVBox(container.NewBorder(nil, nil, secondaryText("Worktree"), close, heading))
	terminal := inspectorAction("Open Terminal", theme.ComputerIcon(), true, d.OnOpenTerminal)
	editor := inspectorAction("Editor", theme.DocumentCreateIcon(), false, d.OnOpenEditor)
	note := inspectorAction("Notes", theme.DocumentIcon(), false, d.OnEditNotes)
	activity := inspectorAction("Activity", theme.HistoryIcon(), false, d.OnActivity)
	if d.keyboardActive {
		terminal.keyHint, editor.keyHint, note.keyHint, activity.keyHint = "Enter", "e", "m", "l"
		if d.state.MainWorktree() != nil {
			close.keyHint = platformShortcut("I")
		}
	}
	more := newActionControl("More", theme.MoreHorizontalIcon(), false, nil)
	more.onTap = func() { d.showInspectorMenu(more) }
	controls := container.NewVBox(container.NewHBox(terminal, editor), container.NewHBox(note, activity, more))
	status := "Clean"
	if wt.IsDirty {
		status = "Uncommitted changes"
	}
	if wt.Detached {
		status += " · Detached HEAD"
	}
	checkout := inspectorGroup("Checkout", inspectorProperty("Branch", wt.Branch, true), inspectorProperty("Path", wt.Path, true), inspectorProperty("Status", status, false), inspectorProperty("Remote", syncLabel(wt.Sync), false))
	rows := []fyne.CanvasObject{controls, checkout}
	if pr := d.prFor(wt.Branch); pr != nil {
		kind := "Pull request"
		if d.state.Provider == provider.ProviderGitLab {
			kind = "Merge request"
		}
		state := pr.State
		if pr.Draft {
			state += " · Draft"
		}
		properties := []fyne.CanvasObject{newPRLink(fmt.Sprintf("%s #%d", kind, pr.Number), pr.URL, colorBlue), inspectorProperty("Title", pr.Title, false), inspectorProperty("State", state, false)}
		properties = append(properties, prStatusLabels(pr)...)
		rows = append(rows, inspectorGroup(kind, properties...))
	} else if d.state.CLIAvail != provider.CLIAvailable && d.state.CLIAvail != 0 {
		rows = append(rows, buildCLIWarning(d.state.CLIAvail, d.state.Provider))
	}
	var processes []fyne.CanvasObject
	for _, a := range d.agentsFor(wt.Path) {
		processes = append(processes, inspectorProperty(agentDisplayName(a), fmt.Sprintf("PID %s · %s · %s", a.PID, a.State, a.Started), false))
	}
	for _, i := range d.idesFor(wt.Path) {
		processes = append(processes, inspectorProperty(string(i.Kind), fmt.Sprintf("PID %d", i.PID), false))
	}
	for _, t := range d.terminalsFor(wt.Path) {
		processes = append(processes, inspectorProperty(string(t.Kind), fmt.Sprintf("PID %d", t.ShellPID), false))
	}
	if len(processes) > 0 {
		rows = append(rows, inspectorGroup("Activity", processes...))
	}
	if sb := d.sandboxInfo(); sb != nil {
		state := "Not found"
		switch sb.Status {
		case sandbox.StatusRunning:
			state = "Running"
		case sandbox.StatusStopped:
			state = "Stopped"
		}
		props := []fyne.CanvasObject{inspectorProperty("Sandbox", sb.Name, true), inspectorProperty("Status", state, false)}
		if sb.Agent != "" {
			props = append(props, inspectorProperty("Agent", sb.Agent, false))
		}
		if sb.ClientVersion != "" {
			props = append(props, inspectorProperty("Client / server", sb.ClientVersion+" / "+sb.ServerVersion, true))
		}
		for _, k := range sb.Kits {
			props = append(props, inspectorProperty("Kit", k.Name+"@"+k.Ref, true))
		}
		rows = append(rows, inspectorGroup("Sandbox", props...))
	}
	noteStatus := "No task notes yet. Use Notes to add them."
	_, titleExists, _ := notes.ReadTitle(wt.Path)
	if notes.Exists(wt.Path) || titleExists {
		noteStatus = "Task notes available. Open Notes to view or edit."
	}
	rows = append(rows, inspectorGroup("Notes", inspectorValue(noteStatus, false)))
	content := container.NewVBox(rows...)
	if d.inspectorPath != wt.Path {
		d.inspectorPath = wt.Path
		d.inspectorOffset = fyne.Position{}
	}
	if d.inspectorScroll == nil {
		d.inspectorScroll = container.NewVScroll(content)
	} else {
		d.inspectorScroll.Content = content
	}
	d.inspectorScroll.Offset = d.inspectorOffset
	return container.New(&inspectorPaneLayout{}, inset(container.NewBorder(top, nil, nil, nil, d.inspectorScroll), spaceSM, spaceSM))
}
func (d *Dashboard) showInspectorMenu(owner fyne.CanvasObject) {
	cnv := fyne.CurrentApp().Driver().CanvasForObject(owner)
	if cnv == nil {
		return
	}
	item := func(label string, fn func(), disabled bool) *fyne.MenuItem {
		m := fyne.NewMenuItem(label, fn)
		m.Disabled = disabled || fn == nil
		return m
	}
	main := d.state.SelectedCard == 0
	entries := []*fyne.MenuItem{item("Pull", d.OnPull, false), item("Send PR / MR", d.OnSendPR, main), item("Delete worktree", d.OnDelete, main)}
	if d.state.ActiveMode != nil && d.state.ActiveMode.Type == "sandbox" {
		entries = append(entries, fyne.NewMenuItemSeparator(), item("Start sandbox", d.OnStartSandbox, false), item("Stop sandbox", d.OnStopSandbox, false), item("Create / enroll sandbox", d.OnSetupSandbox, false))
	}
	popup := widget.NewPopUpMenu(fyne.NewMenu("", entries...), cnv)
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(owner)
	popup.ShowAtPosition(pos.Add(fyne.NewPos(0, owner.Size().Height)))
}

func (d *Dashboard) createMenu() *fyne.Menu {
	issue := fyne.NewMenuItem("Create from GitHub issue", func() {
		if d.OnCreateFromIssue != nil {
			d.OnCreateFromIssue()
		}
	})
	fetch := fyne.NewMenuItem("Fetch GitHub PR", func() {
		if d.OnFetchPR != nil {
			d.OnFetchPR()
		}
	})
	issue.Disabled = d.state.Provider != provider.ProviderGitHub || d.OnCreateFromIssue == nil
	fetch.Disabled = d.state.Provider != provider.ProviderGitHub || d.OnFetchPR == nil
	return fyne.NewMenu("", issue, fetch)
}
func (d *Dashboard) showCreateMenu(owner fyne.CanvasObject) {
	cnv := fyne.CurrentApp().Driver().CanvasForObject(owner)
	if cnv == nil {
		return
	}
	popup := widget.NewPopUpMenu(d.createMenu(), cnv)
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(owner)
	popup.ShowAtPosition(pos.Add(fyne.NewPos(0, owner.Size().Height)))
}
