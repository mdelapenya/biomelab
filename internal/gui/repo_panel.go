package gui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/sandbox"
)

// RepoGroup represents a registered repository with its modes.
type RepoGroup struct {
	Path       string
	Name       string
	Modes      []config.ModeEntry
	ActiveMode int
}

// RepoPanel is the left panel showing the repository/mode list.
// It does NOT use widget.Tree (which implements Focusable and steals
// keyboard focus). Instead it uses plain tappable containers so the
// canvas-level key handlers always work.
type RepoPanel struct {
	groups      []*RepoGroup
	activeGrp   int
	activeMode  int
	sbxStatuses map[string]sandbox.Status

	content *fyne.Container
	list    *fyne.Container // the scrollable mode list

	OnModeSelected func(groupIdx, modeIdx int)
}

// NewRepoPanel creates a repo panel from config entries.
func NewRepoPanel(groups []*RepoGroup, sbxStatuses map[string]sandbox.Status) *RepoPanel {
	rp := &RepoPanel{
		groups:      groups,
		sbxStatuses: sbxStatuses,
	}
	rp.build()
	return rp
}

// Content returns the renderable panel.
func (rp *RepoPanel) Content() fyne.CanvasObject {
	return rp.content
}

// SetActive highlights the given group and mode.
func (rp *RepoPanel) SetActive(groupIdx, modeIdx int) {
	rp.activeGrp = groupIdx
	rp.activeMode = modeIdx
	rp.rebuildList()
}

// UpdateStatuses refreshes sandbox status dots.
func (rp *RepoPanel) UpdateStatuses(statuses map[string]sandbox.Status) {
	rp.sbxStatuses = statuses
	rp.rebuildList()
}

func (rp *RepoPanel) build() {
	rp.list = container.NewVBox()
	rp.rebuildList()

	helpLabel := monoText("[a]dd  [n]ew sandbox  [x]rm", colorDimGray, false)
	helpLabel.TextSize = scaledSize(9)

	scroll := container.NewScroll(rp.list)
	bg := canvas.NewRectangle(colorPanelBg)

	inner := container.NewBorder(nil, container.NewPadded(helpLabel), nil, nil, scroll)

	if rp.content == nil {
		rp.content = container.NewStack(bg, inner)
	} else {
		// Replace content in-place so the parent layout keeps the same object.
		rp.content.Objects = []fyne.CanvasObject{bg, inner}
		rp.content.Refresh()
	}
}

// RebuildFull rebuilds the entire panel including help labels (for zoom).
func (rp *RepoPanel) RebuildFull() {
	rp.build()
}

func (rp *RepoPanel) rebuildList() {
	rp.list.Objects = nil

	for gi, group := range rp.groups {
		// Compact visual separator between repo groups.
		if gi > 0 {
			gap := canvas.NewRectangle(colorPanelBg)
			gap.SetMinSize(fyne.NewSize(0, 4))
			sep := canvas.NewRectangle(colorBorder)
			sep.SetMinSize(fyne.NewSize(0, 1))
			rp.list.Add(gap)
			rp.list.Add(sep)
		}

		// Repo header (not clickable) — bold + foreground color for
		// strong visual hierarchy over the mode sub-items.
		header := monoText(group.Name, colorForeground, true)
		header.TextSize = scaledSize(12)
		topGap := canvas.NewRectangle(colorPanelBg)
		topGap.SetMinSize(fyne.NewSize(0, 4))
		rp.list.Add(container.NewVBox(topGap, header))

		// Mode entries (clickable).
		for mi, mode := range group.Modes {
			groupIdx, modeIdx := gi, mi
			isActive := gi == rp.activeGrp && mi == rp.activeMode

			line := rp.buildModeLine(mode, isActive)
			tap := newTappableCard(line, func() {
				if rp.OnModeSelected != nil {
					rp.OnModeSelected(groupIdx, modeIdx)
				}
			})
			rp.list.Add(tap)
		}
	}
	rp.list.Refresh()
}

func (rp *RepoPanel) buildModeLine(mode config.ModeEntry, isActive bool) fyne.CanvasObject {
	prefix := "  "
	if isActive {
		prefix = "▸ "
	}

	icon := "\U0001F4C2" // folder for regular
	if mode.Type == "sandbox" {
		icon = "\U0001F433" // whale for sandbox
	}

	modeLabel := "host"
	if mode.Agent != "" {
		modeLabel = mode.Agent
	}

	text := fmt.Sprintf("%s%s [%s]", prefix, icon, modeLabel)

	var labelColor = colorGray
	if isActive {
		labelColor = colorSelected
	}

	label := monoText(text, labelColor, isActive)
	label.TextSize = scaledSize(11)

	// Status dot for sandbox modes.
	var dot *canvas.Text
	if mode.Type == "sandbox" && mode.SandboxName != "" {
		if status, ok := rp.sbxStatuses[mode.SandboxName]; ok {
			dotText := " ●"
			var dotColor = colorRed
			switch status {
			case sandbox.StatusRunning:
				dotColor = colorGreen
			case sandbox.StatusStopped:
				dotColor = colorYellow
			}
			dot = monoText(dotText, dotColor, false)
		}
	}

	var row fyne.CanvasObject
	if dot != nil {
		row = container.NewHBox(label, dot)
	} else {
		row = label
	}

	// Active row gets a highlighted background so it stands out.
	if isActive {
		bg := canvas.NewRectangle(colorSelection)
		bg.CornerRadius = 4
		return container.NewStack(bg, container.NewPadded(row))
	}
	return row
}
