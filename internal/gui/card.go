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
	content fyne.CanvasObject
	onTap   func()
}

func newTappableCard(content fyne.CanvasObject, onTap func()) *tappableCard {
	tc := &tappableCard{content: content, onTap: onTap}
	tc.ExtendBaseWidget(tc)
	return tc
}

func (tc *tappableCard) Tapped(_ *fyne.PointEvent) {
	if tc.onTap != nil {
		tc.onTap()
	}
}

func (tc *tappableCard) TappedSecondary(_ *fyne.PointEvent) {}

func (tc *tappableCard) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(tc.content)
}

// makeCard wraps content in a bordered, tappable card container.
// When selected, the card gets a cyan border, thicker stroke, and a left-edge bar.
func makeCard(content fyne.CanvasObject, selected bool, isMain bool, onTap func()) *tappableCard {
	borderColor := colorBorder
	strokeWidth := float32(1)
	if isMain {
		strokeWidth = 2
	}
	if selected {
		borderColor = colorSelected
		strokeWidth += 1
	}

	bg := canvas.NewRectangle(colorBackground)
	bg.CornerRadius = 6

	border := canvas.NewRectangle(color.Transparent)
	border.StrokeColor = borderColor
	border.StrokeWidth = strokeWidth
	border.CornerRadius = 6

	padded := container.NewPadded(container.NewPadded(content))
	visual := container.NewStack(bg, border, padded)

	return newTappableCard(visual, onTap)
}

// buildCardContent builds the visual content for a single worktree card.
// selected adds a "▸ " prefix to the branch name.
// pathMax controls max path characters (use maxLinkedPathChars or maxMainPathChars).
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

	// Branch line with selection indicator.
	branchPrefix := ""
	branchColor := colorBranch
	if selected {
		branchPrefix = "▸ "
		branchColor = colorSelected
	}
	branchText := monoText(branchPrefix+wt.Branch, branchColor, true)
	branchText.TextSize = scaledSize(13)
	if wt.IsMain {
		badge := makeBadge("main")
		items = append(items, container.NewHBox(branchText, badge))
	} else if wt.Detached {
		detached := monoText(" (detached)", colorDimGray, false)
		items = append(items, container.NewHBox(branchText, detached))
	} else {
		items = append(items, branchText)
	}

	// Path: prefix-truncated dynamically so it never overflows the card,
	// regardless of how narrow the card becomes when the window is resized.
	items = append(items, newTruncMonoText(wt.Path, colorDimGray, false, true))

	// Sandbox info.
	if sbx != nil && sbx.Name != "" {
		var label string
		var c color.Color
		switch sbx.Status {
		case sandbox.StatusRunning:
			label = "\U0001F40B sandbox: " + sbx.Name + " (running)"
			c = colorGreen
		case sandbox.StatusStopped:
			label = "\U0001F40B sandbox: " + sbx.Name + " (stopped)"
			c = colorYellow
		default:
			label = "\U0001F40B sandbox: " + sbx.Name + " (not found)"
			c = colorRed
		}
		items = append(items, monoText(truncateStr(label, pathMax), c, false))
		if sbx.ClientVersion != "" {
			ver := "sbx: client " + sbx.ClientVersion
			if sbx.ServerVersion != "" {
				ver += "  server " + sbx.ServerVersion
			}
			items = append(items, monoText(ver, colorDimGray, false))
		}
	}

	// Separator before status section.
	items = append(items, widget.NewSeparator())

	// PR/MR status.
	if pr != nil {
		items = append(items, buildPRLine(pr, prov))
	} else if cliAvail != provider.CLIAvailable && cliAvail != 0 {
		items = append(items, buildCLIWarning(cliAvail, prov))
	}

	// Agent status.
	if len(agents) > 0 {
		for _, a := range agents {
			prefix := "● "
			if a.IsSubAgent {
				prefix = "  ↳ "
			}
			line := fmt.Sprintf("%s%s (PID %s)", prefix, string(a.Kind), a.PID)
			items = append(items, monoText(line, colorGreen, false))
			// Agent detail line (state + started) matching TUI.
			if a.State != "" || a.Started != "" {
				detail := fmt.Sprintf("  state: %s  started: %s", a.State, a.Started)
				detailText := monoText(detail, colorGreen, false)
				detailText.TextSize = scaledSize(10)
				items = append(items, detailText)
			}
		}
	} else if sbx != nil && sbx.Agent != "" && sbx.Status == sandbox.StatusRunning {
		line := fmt.Sprintf("● %s (sandbox)", sbx.Agent)
		items = append(items, monoText(line, colorGreen, false))
	} else {
		items = append(items, monoText("○ no agent", colorDimGray, false))
	}

	// IDE status.
	if len(ides) > 0 {
		for _, i := range ides {
			line := fmt.Sprintf("■ %s (PID %d)", string(i.Kind), i.PID)
			items = append(items, monoText(line, colorBlue, false))
		}
	} else {
		items = append(items, monoText("□ no IDE", colorDimGray, false))
	}

	// Terminal status.
	if len(terminals) > 0 {
		for _, t := range terminals {
			line := fmt.Sprintf("▶ %s (PID %d)", string(t.Kind), t.ShellPID)
			items = append(items, monoText(line, colorPurple, false))
		}
	} else {
		items = append(items, monoText("▷ no terminal", colorDimGray, false))
	}

	// Dirty + sync status on one line.
	var statusItems []fyne.CanvasObject
	if wt.IsDirty {
		statusItems = append(statusItems, monoText("~ dirty", colorYellow, false))
	} else {
		statusItems = append(statusItems, monoText("✓ clean", colorGreen, false))
	}
	statusItems = append(statusItems, monoText("  ", colorForeground, false))
	statusItems = append(statusItems, syncStatusText(wt.Sync))
	items = append(items, container.NewHBox(statusItems...))

	return container.NewVBox(items...)
}

func buildPRLine(pr *provider.PRInfo, prov provider.Provider) fyne.CanvasObject {
	label := "PR"
	if prov == provider.ProviderGitLab {
		label = "MR"
	}

	title := truncateStr(pr.Title, 30)
	stateLabel := pr.State
	c := colorBlue

	switch {
	case pr.Draft:
		c = colorDimGray
		stateLabel = "draft"
	case pr.State == "merged":
		c = colorPurple
	case pr.State == "closed":
		c = colorRed
	}

	text := fmt.Sprintf("%s #%d %s (%s)", label, pr.Number, title, stateLabel)
	parts := []fyne.CanvasObject{monoText(truncateStr(text, 36), c, false)}

	// Review status icon (shown before the CI icon).
	switch pr.ReviewStatus {
	case "approved":
		parts = append(parts, monoText(" ✓", colorGreen, false))
	case "changes_requested":
		parts = append(parts, monoText(" !", colorRed, false))
	case "commented":
		parts = append(parts, monoText(" ●", colorYellow, false))
	}

	if pr.CheckStatus != "" {
		icon := provider.StatusIcon(pr.CheckStatus)
		var iconColor color.Color
		switch pr.CheckStatus {
		case "success":
			iconColor = colorGreen
		case "failure":
			iconColor = colorRed
		case "pending":
			iconColor = colorYellow
		}
		if iconColor != nil {
			parts = append(parts, monoText(" "+icon, iconColor, false))
		}
	}

	return container.NewHBox(parts...)
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
	return monoText(msg, colorDimGray, false)
}

func syncStatusText(sync git.SyncStatus) fyne.CanvasObject {
	switch sync {
	case git.SyncUpToDate:
		return monoText("↕ up-to-date", colorGreen, false)
	case git.SyncAhead:
		return monoText("↑ ahead", colorYellow, false)
	case git.SyncBehind:
		return monoText("↓ behind", colorYellow, false)
	case git.SyncDiverged:
		return monoText("↕ diverged", colorRed, false)
	case git.SyncNoUpstream:
		return monoText("- no upstream", colorGray, false)
	default:
		return monoText("? unknown", colorDimGray, false)
	}
}

// makeBadge creates a small colored badge (e.g., "main" tag).
func makeBadge(text string) fyne.CanvasObject {
	bg := canvas.NewRectangle(colorBorder)
	bg.CornerRadius = 3

	label := monoText(text, colorGray, false)
	label.TextSize = scaledSize(9)

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

func truncateStr(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

// truncatePath trims a path from the left (prefix), keeping the unique suffix.
// Example: "/Users/foo/src/github.com/org/repo/.biomelab-worktrees/feat" → "…rg/repo/.biomelab-worktrees/feat"
func truncatePath(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return "…" + string(runes[len(runes)-max+1:])
}

// truncMonoText is a canvas.Text wrapper that re-truncates its content to fit
// the width assigned by the parent layout. canvas.Text alone does not clip, so
// long paths spill past their card's border; this widget recomputes how many
// runes fit each time the renderer is laid out.
//
// prefixTrunc chooses the truncation style: true for prefix truncation
// (truncatePath — keeps the suffix), false for suffix truncation (truncateStr).
type truncMonoText struct {
	widget.BaseWidget
	fullText    string
	prefixTrunc bool
	txt         *canvas.Text
}

func newTruncMonoText(full string, c color.Color, bold, prefixTrunc bool) *truncMonoText {
	t := &truncMonoText{
		fullText:    full,
		prefixTrunc: prefixTrunc,
		txt:         monoText(full, c, bold),
	}
	t.ExtendBaseWidget(t)
	return t
}

func (t *truncMonoText) CreateRenderer() fyne.WidgetRenderer {
	return &truncMonoTextRenderer{t: t}
}

type truncMonoTextRenderer struct {
	t *truncMonoText
}

// charWidth is an approximation of a monospace glyph width as a fraction of
// the font size. 0.6 matches most monospace faces; using a conservative value
// guarantees the truncated string fits inside size.Width.
const charWidth = 0.6

func (r *truncMonoTextRenderer) Layout(size fyne.Size) {
	cw := r.t.txt.TextSize * charWidth
	if cw <= 0 {
		cw = 8
	}
	maxChars := int(size.Width / cw)
	if maxChars < 2 {
		maxChars = 2
	}
	if r.t.prefixTrunc {
		r.t.txt.Text = truncatePath(r.t.fullText, maxChars)
	} else {
		r.t.txt.Text = truncateStr(r.t.fullText, maxChars)
	}
	r.t.txt.Resize(size)
	r.t.txt.Refresh()
}

// MinSize reports a tiny width so the parent VBox/Border can assign any width
// it likes without being forced to grow to the full string. Height tracks the
// text size.
func (r *truncMonoTextRenderer) MinSize() fyne.Size {
	return fyne.NewSize(20, r.t.txt.TextSize*1.4)
}

func (r *truncMonoTextRenderer) Refresh()                     { r.t.txt.Refresh() }
func (r *truncMonoTextRenderer) Destroy()                     {}
func (r *truncMonoTextRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.t.txt} }
