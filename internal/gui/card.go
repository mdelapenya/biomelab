package gui

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/mdelapenya/biomelab/internal/agent"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/ide"
	"github.com/mdelapenya/biomelab/internal/notes"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/sandbox"
	"github.com/mdelapenya/biomelab/internal/terminal"
)

// maxLinkedPathChars is the max characters for a path in a linked card.
const maxLinkedPathChars = 42

// maxMainPathChars is the max characters for a path in the main card.
const maxMainPathChars = 90

// tappableCard is a custom widget that wraps card content and responds to taps.
type tappableCard struct {
	widget.BaseWidget
	content        fyne.CanvasObject
	onTap          func()
	onSecondaryTap func()
}

func newTappableCard(content fyne.CanvasObject, onTap func()) *tappableCard {
	tc := &tappableCard{content: content, onTap: onTap}
	tc.ExtendBaseWidget(tc)
	return tc
}

func (tc *tappableCard) SetOnSecondaryTap(fn func()) { tc.onSecondaryTap = fn }

func (tc *tappableCard) Tapped(_ *fyne.PointEvent) {
	if tc.onTap != nil {
		tc.onTap()
	}
}

func (tc *tappableCard) TappedSecondary(_ *fyne.PointEvent) {
	if tc.onSecondaryTap != nil {
		tc.onSecondaryTap()
	}
}

func (tc *tappableCard) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(tc.content)
}

// makeCard wraps content in a bordered, tappable card container.
// Selection uses a restrained accent border on the shared card surface.
func makeCard(content fyne.CanvasObject, selected bool, isMain bool, onTap func()) *tappableCard {
	bg := canvas.NewRectangle(colorCardBg)
	bg.CornerRadius = scaledSize(radiusCard)
	bg.StrokeColor = colorBorder
	bg.StrokeWidth = 1
	if selected {
		bg.StrokeColor = colorSelected
		bg.StrokeWidth = 1.5
	}
	return newTappableCard(container.NewStack(bg, inset(content, spaceMD, spaceMD)), onTap)
}

// buildCardContent builds the visual content for a single worktree card.
// Legacy pathMax and selection parameters remain compatible with callers;
// technical values now truncate by measured width at layout time.
func buildCardContent(
	wt git.Worktree,
	agents []agent.Info,
	ides []ide.Info,
	terminals []terminal.Info,
	pr *provider.PRInfo,
	cliAvail provider.CLIAvailability,
	prov provider.Provider,
	sbx *SandboxCardInfo,
	pathMax int,
	selected bool,
) fyne.CanvasObject {
	var items []fyne.CanvasObject
	branch := newMeasuredText(wt.Branch, colorBranch, true, true, false)
	var badge fyne.CanvasObject
	if wt.IsMain {
		badge = makeBadge("Main")
	} else {
		badge = makeStagePill(kanbanStageOf(pr))
	}
	items = append(items, container.NewBorder(nil, nil, nil, badge, branch))
	path := newMeasuredText(wt.Path, colorDimGray, false, true, true)
	path.txt.TextSize = scaledSize(textSecondarySize)
	items = append(items, path)
	if pr != nil {
		items = append(items, buildPRLine(pr, prov))
	} else if cliAvail != provider.CLIAvailable && cliAvail != 0 {
		items = append(items, buildCLIWarning(cliAvail, prov))
	}
	if sbx != nil && sbx.Name != "" {
		status := "not found"
		c := colorRed
		switch sbx.Status {
		case sandbox.StatusRunning:
			status, c = "running", colorGreen
		case sandbox.StatusStopped:
			status, c = "stopped", colorYellow
		}
		items = append(items, newMeasuredText("Sandbox "+sbx.Name+" · "+status, c, false, false, false))
		if sbx.ClientVersion != "" {
			items = append(items, secondaryText("Client "+sbx.ClientVersion+" · Server "+sbx.ServerVersion))
		}
		for _, kit := range sbx.Kits {
			items = append(items, newMeasuredText("Kit "+kit.Name+"@"+kit.Ref, colorGray, false, false, false))
		}
	}
	for _, a := range agents {
		line := newMeasuredText(fmt.Sprintf("%s · PID %s · %s %s", agentDisplayName(a), a.PID, a.State, a.Started), colorGreen, false, false, false)
		line.txt.TextSize = scaledSize(textSecondarySize)
		items = append(items, line)
	}
	if len(agents) == 0 && sbx != nil && sbx.Agent != "" && sbx.Status == sandbox.StatusRunning {
		items = append(items, secondaryText(sbx.Agent+" · sandbox"))
	}
	for _, i := range ides {
		items = append(items, secondaryText(fmt.Sprintf("%s · PID %d", i.Kind, i.PID)))
	}
	for _, t := range terminals {
		items = append(items, secondaryText(fmt.Sprintf("%s · PID %d", t.Kind, t.ShellPID)))
	}
	status := "Clean"
	if wt.IsDirty {
		status = "Uncommitted changes"
	}
	if wt.Detached {
		status += " · Detached HEAD"
	}
	if notes.Exists(wt.Path) {
		status += " · Notes"
	}
	items = append(items, secondaryText(status+" · "+syncLabel(wt.Sync)))
	return container.NewVBox(items...)
}

func buildPRLine(pr *provider.PRInfo, prov provider.Provider) fyne.CanvasObject {
	kind := "PR"
	if prov == provider.ProviderGitLab {
		kind = "MR"
	}
	state := pr.State
	if pr.Draft {
		state = "draft"
	}
	link := newPRLink(fmt.Sprintf("%s #%d", kind, pr.Number), pr.URL, colorBlue)
	title := newMeasuredText(pr.Title, colorForeground, false, false, false)
	title.txt.TextSize = scaledSize(textSecondarySize)
	titleRow := container.NewBorder(nil, nil, link, nil, title)
	status := secondaryText(state)
	items := []fyne.CanvasObject{titleRow, status}
	items = append(items, prStatusLabels(pr)...)
	return container.NewVBox(items...)
}

func prStatusLabels(pr *provider.PRInfo) []fyne.CanvasObject {
	var items []fyne.CanvasObject
	if pr.ReviewStatus != "" {
		label := pr.ReviewStatus
		c := colorGray
		switch label {
		case "approved":
			c = colorGreen
		case "changes_requested":
			label = "changes requested"
			c = colorRed
		case "commented":
			c = colorYellow
		}
		hint := newStatusText("Review: "+label, c)
		hint.txt.TextSize = scaledSize(textSecondarySize)
		items = append(items, hint)
	}
	if pr.CheckStatus != "" {
		c := colorGray
		switch pr.CheckStatus {
		case "success":
			c = colorGreen
		case "failure":
			c = colorRed
		case "pending":
			c = colorYellow
		}
		hint := newStatusText("CI: "+pr.CheckStatus, c)
		hint.txt.TextSize = scaledSize(textSecondarySize)
		items = append(items, hint)
	}
	return items
}

func buildCLIWarning(avail provider.CLIAvailability, prov provider.Provider) fyne.CanvasObject {
	cliName := "cli"
	switch prov {
	case provider.ProviderGitHub:
		cliName = "gh"
	case provider.ProviderGitLab:
		cliName = "glab"
	}

	var msg string
	switch avail {
	case provider.CLINotFound:
		msg = fmt.Sprintf("%s not installed — install %s CLI", cliName, cliName)
	case provider.CLINotAuthenticated:
		msg = fmt.Sprintf("%s not authenticated — run: %s auth login", cliName, cliName)
	case provider.CLIUnsupportedProvider:
		msg = fmt.Sprintf("PR status: %s not yet supported", prov.String())
	}
	return newMeasuredText(msg, colorDimGray, false, false, false)
}

func syncLabel(sync git.SyncStatus) string {
	switch sync {
	case git.SyncUpToDate:
		return "Up to date"
	case git.SyncAhead:
		return "Ahead"
	case git.SyncBehind:
		return "Behind"
	case git.SyncDiverged:
		return "Diverged"
	case git.SyncNoUpstream:
		return "No upstream"
	default:
		return "Sync unknown"
	}
}

// makeBadge creates a small colored badge (e.g., "main" tag).
func makeBadge(text string) fyne.CanvasObject {
	bg := canvas.NewRectangle(colorBorder)
	bg.CornerRadius = scaledSize(radiusControl)

	label := uiText(text, colorGray, false)
	label.TextSize = scaledSize(textSecondarySize)

	return container.NewStack(bg, container.NewPadded(label))
}

// countChip renders an integer as a pill-shaped badge with a tinted
// background. Used in the repo panel and kanban headers to surface counts at
// a glance.
func countChip(n int, bgColor color.Color) fyne.CanvasObject {
	bg := canvas.NewRectangle(bgColor)
	bg.CornerRadius = scaledSize(radiusCard)

	label := uiText(fmt.Sprintf("%d", n), colorForeground, true)
	label.TextSize = scaledSize(textSecondarySize)
	label.Alignment = fyne.TextAlignCenter

	return container.NewStack(bg, container.NewPadded(label))
}

// monoText creates a monospace canvas.Text with the given color and bold state.
func monoText(text string, c color.Color, bold bool) *canvas.Text {
	t := canvas.NewText(text, c)
	t.TextStyle.Monospace = true
	t.TextStyle.Bold = bold
	return t
}

// scaledSize returns a font size scaled relative to the theme text size.
// base is the size at the default (14pt) theme. Returns proportionally
// scaled value for the current theme setting.
func scaledSize(base float32) float32 {
	return base * theme.TextSize() / 14
}

// Board selection uses the same solid surface as the selected List item.
func makeKanbanCard(content fyne.CanvasObject, selected bool, onTap func()) *tappableCard {
	bg := canvas.NewRectangle(colorCardBg)
	bg.CornerRadius = scaledSize(radiusCard)
	bg.StrokeColor = colorBorder
	bg.StrokeWidth = 1
	if selected {
		bg.FillColor = colorActionBg
		bg.StrokeColor = colorActionBg
	}
	return newTappableCard(container.NewStack(bg, inset(content, spaceSM, spaceSM)), onTap)
}

// agentDisplayName retains the detected parent/child distinction in compact
// process rows without adding another property or control.
func agentDisplayName(info agent.Info) string {
	name := string(info.Kind)
	if info.IsSubAgent {
		name = "↳ " + name
	}
	return name
}
