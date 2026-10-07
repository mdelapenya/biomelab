package gui

import (
	"context"
	"fmt"
	"image/color"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/regent"
)

// regentLogWindowInitialSize is wide enough for typical prompts and full
// file paths inside tool args; the window is user-resizable.
var regentLogWindowInitialSize = fyne.NewSize(900, 640)

// regentLogLimit caps how many recent steps the window fetches. Matches
// rgt's own default; we surface a Refresh button rather than paging.
const regentLogLimit = 50

type regentLogData struct {
	sessionID string
	steps     []Step
	err       error
	missing   bool
}

// regentLogSession owns the in-flight commands for one window. Its methods
// and completion callbacks run on the UI thread; only fetch runs in a worker.
type regentLogSession struct {
	mu               sync.Mutex
	generation       uint64
	cancel           context.CancelFunc
	exportGeneration uint64
	exportCancel     context.CancelFunc
	closed           bool
	fetch            func(context.Context, string) regentLogData
	fetchRaw         func(context.Context, string) ([]byte, error)
}

func newRegentLogSession() *regentLogSession {
	return &regentLogSession{
		fetch: func(ctx context.Context, path string) regentLogData {
			if _, err := exec.LookPath("rgt"); err != nil {
				return regentLogData{missing: true}
			}
			id, steps, err := regent.LogJSONContext(ctx, path, regentLogLimit)
			return regentLogData{sessionID: id, steps: steps, err: err}
		},
		fetchRaw: func(ctx context.Context, path string) ([]byte, error) {
			return regent.LogJSONRawContext(ctx, path, regentLogLimit)
		},
	}
}

func (s *regentLogSession) load(path string, done func(regentLogData)) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.generation++
	if s.cancel != nil {
		s.cancel()
	}
	// A refresh or target switch invalidates any pending export as well.
	s.exportGeneration++
	if s.exportCancel != nil {
		s.exportCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	generation := s.generation
	s.mu.Unlock()
	go func() {
		data := s.fetch(ctx, path)
		fyne.Do(func() {
			s.mu.Lock()
			valid := !s.closed && s.generation == generation && ctx.Err() == nil
			s.mu.Unlock()
			if valid {
				done(data)
			}
		})
	}()
}

func (s *regentLogSession) export(path string, done func([]byte, error)) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.exportGeneration++
	if s.exportCancel != nil {
		s.exportCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.exportCancel = cancel
	generation := s.exportGeneration
	s.mu.Unlock()
	go func() {
		data, err := s.fetchRaw(ctx, path)
		fyne.Do(func() {
			s.mu.Lock()
			valid := !s.closed && s.exportGeneration == generation && ctx.Err() == nil
			s.mu.Unlock()
			if valid {
				done(data, err)
			}
		})
	}()
}

func (s *regentLogSession) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}
	if s.exportCancel != nil {
		s.exportCancel()
	}
}

// showRegentLogModal opens (or reuses) a single top-level window showing
// `rgt log` for the given worktree. Subsequent presses of 'l' — same or
// different card — repopulate this same window rather than spawning new
// ones, so the user always has at most one regent log window open.
//
// Implementation: App keeps a single window handle plus a closure that
// re-renders content for a different worktree. When the window is closed,
// both are cleared so the next 'l' press creates a fresh one.
func (a *App) showRegentLogModal(wt git.Worktree) {
	if a.regentLogWindow != nil && a.regentLogReload != nil {
		a.regentLogReload(wt)
		a.regentLogWindow.RequestFocus()
		return
	}

	w := a.fyneApp.NewWindow("Regent activity — " + wt.Branch)
	a.regentLogWindow = w

	body := container.NewVBox()
	scroll := container.NewVScroll(body)
	session := newRegentLogSession()

	current := wt
	render := func(target git.Worktree) {
		current = target
		w.SetTitle("Regent activity — " + target.Branch)
		body.Objects = []fyne.CanvasObject{dialogBusy("Loading regent activity…")}
		body.Refresh()
		session.load(target.Path, func(data regentLogData) {
			body.Objects = a.buildRegentLogContent(data)
			body.Refresh()
			// Recompute the scroll content's dimensions after replacing rows.
			scroll.Refresh()
			scroll.ScrollToTop()
		})
	}
	render(wt)

	refresh := widget.NewButton("Refresh", func() { render(current) })
	refresh.Importance = widget.HighImportance
	exportBtn := widget.NewButton("Export JSON…", func() {
		a.exportRegentLog(session, current, w)
	})
	closeBtn := widget.NewButton("Close", func() { w.Close() })
	w.SetContent(regentWindowContent(scroll, refresh, exportBtn, closeBtn))
	w.Resize(regentLogWindowInitialSize)
	w.CenterOnScreen()

	a.regentLogReload = render
	w.SetOnClosed(func() {
		session.close()
		a.regentLogWindow = nil
		a.regentLogReload = nil
	})

	w.Show()
	w.RequestFocus()
}

// Shared by the live activity window and offline render evidence.
func regentWindowContent(scroll fyne.CanvasObject, refresh, export, close fyne.CanvasObject) fyne.CanvasObject {
	footer := container.NewBorder(nil, nil, container.NewHBox(refresh, export), close)
	return container.NewPadded(container.NewBorder(nil, container.NewVBox(widget.NewSeparator(), footer), nil, nil, scroll))
}

// buildRegentLogContent returns the rendered children for the scrollable
// body: a session header followed by a step row per Step. Handles all
// "no data" states (rgt missing, no .regent/, no activity) with a
// friendly inline message.
func (a *App) buildRegentLogContent(data regentLogData) []fyne.CanvasObject {
	if data.missing {
		// OS-agnostic message — install hints live in the deps dialog.
		return []fyne.CanvasObject{
			dialogHeading("re_gent (rgt) is not installed."),
			dialogHint("See install instructions: https://github.com/regent-vcs/re_gent"),
			dialogHint("Or open the Dependencies dialog from the system tray."),
		}
	}

	if data.err != nil {
		return []fyne.CanvasObject{
			dialogHeading("rgt log failed:"),
			dialogText(data.err.Error()),
		}
	}
	if len(data.steps) == 0 {
		return []fyne.CanvasObject{
			dialogHeading("No regent activity yet."),
			dialogHint("Run an agent in this worktree and the log will populate here."),
		}
	}

	items := make([]fyne.CanvasObject, 0, len(data.steps)+1)
	header := dialogHeading(fmt.Sprintf("Session %s · %d steps", data.sessionID, len(data.steps)))
	items = append(items, header)
	items = append(items, widget.NewSeparator())

	for i, s := range data.steps {
		if i > 0 {
			items = append(items, widget.NewSeparator())
		}
		items = append(items, buildRegentStepRow(s))
	}
	return items
}

// buildRegentStepRow renders one Step as a WhatsApp-style chat exchange:
//
//   - centered metadata line (sha · timestamp · origin)
//   - Human prompt right-aligned in a colored bubble (~72% width)
//   - Agent reply left-aligned in a different colored bubble (~72%),
//     containing the text + a "▶ N tools" toggle when there are tools
//   - when the toggle is pressed, a separate full-width "tools" bubble
//     appears below — tool calls hold long file paths / commands and
//     would be cramped inside the agent bubble's 72% column.
//
// Empty roles produce no bubble.
func buildRegentStepRow(s Step) fyne.CanvasObject {
	rows := []fyne.CanvasObject{}

	// Centered metadata line — small and dim so it reads as a divider
	// between turns, not as content.
	metaText := fmt.Sprintf("● %s · %s", s.ShortHash(), s.Timestamp.Format("2006-01-02 15:04:05"))
	if s.Origin != "" {
		metaText += " · " + s.Origin
	}
	meta := dialogHint(metaText)
	meta.TextStyle.Monospace = true
	meta.Alignment = fyne.TextAlignCenter
	rows = append(rows, meta)

	if s.HumanPrompt != "" {
		bubble := buildSimpleBubble(s.HumanPrompt, colorBubbleHuman, colorBubbleHumanStroke)
		rows = append(rows, alignRight(bubble, naturalBubbleWidth(s.HumanPrompt)))
	}
	if s.AgentReply != "" || len(s.Tools) > 0 {
		agent, toolsBubble := buildAgentBubble(s.AgentReply, s.Tools)
		// Agent bubble has the reply text plus, when present, a tools
		// toggle button. The toggle is wider than a very short reply
		// like "Done." — floor at 200px so the button isn't crushed.
		w := naturalBubbleWidth(s.AgentReply)
		if len(s.Tools) > 0 && w < 200 {
			w = 200
		}
		rows = append(rows, alignLeft(agent, w))
		if toolsBubble != nil {
			rows = append(rows, toolsBubble)
		}
	}

	return container.NewVBox(rows...)
}

// bubble colors are looked up at render time (not at package init) so
// they pick up theme changes — the palette globals (colorBlue,
// colorPurple, colorPanelBg) are swapped wholesale by applyDarkPalette
// and applyLightPalette, so a literal capturing the previous values
// would render stale.
//
// A restrained tint distinguishes the two speakers while keeping the
// conversation on the same neutral surfaces as the workspace inspector.
//
// Tools is technical content, not a third speaker, so it uses
// colorPanelBg (the existing "panel surface" color). Neutral surface
// keeps emphasis on the conversation bubbles and avoids the saturated
// look a third hue produced under both light and dark themes.
const (
	bubbleFillAlpha   = 24
	bubbleStrokeAlpha = 48
)

func colorBubbleHuman() color.Color { return tintAlpha(colorBlue, bubbleFillAlpha) }
func colorBubbleAgent() color.Color { return tintAlpha(colorPurple, bubbleFillAlpha) }
func colorBubbleTools() color.Color { return colorPanelBg }

// colorBubbleToolInner is the fill for the per-tool mini-bubble that
// nests inside the tools bubble. Using colorBackground gives a tone
// distinct from the outer surface in both themes (lighter than panel
// in dark mode, slightly tinted in light), creating a subtle nested
// look.
func colorBubbleToolInner() color.Color { return colorBackground }

func colorBubbleHumanStroke() color.Color     { return tintAlpha(colorBlue, bubbleStrokeAlpha) }
func colorBubbleAgentStroke() color.Color     { return tintAlpha(colorPurple, bubbleStrokeAlpha) }
func colorBubbleToolsStroke() color.Color     { return colorBorder }
func colorBubbleToolInnerStroke() color.Color { return colorBorder }

// tintAlpha returns base with its alpha replaced by a (so the same RGB
// can be reused at different opacities for fill vs stroke).
func tintAlpha(base color.NRGBA, a uint8) color.NRGBA {
	return color.NRGBA{R: base.R, G: base.G, B: base.B, A: a}
}

// alignRight returns o pinned to the right edge of its row, sized to
// min(naturalWidth, chatBubbleMaxRatio * row width). Used for Human
// bubbles.
func alignRight(o fyne.CanvasObject, naturalWidth float32) fyne.CanvasObject {
	return container.New(&chatRowLayout{alignRight: true, naturalWidth: naturalWidth}, o)
}

// alignLeft returns o pinned to the left edge with the same width rule.
// Used for Agent bubbles.
func alignLeft(o fyne.CanvasObject, naturalWidth float32) fyne.CanvasObject {
	return container.New(&chatRowLayout{alignRight: false, naturalWidth: naturalWidth}, o)
}

// chatBubbleMaxRatio is the upper bound on bubble width as a fraction
// of row width. Short messages stay narrow; long ones cap here and
// wrap inside the bubble.
const chatBubbleMaxRatio = 0.72

// chatRowLayout sizes the single child to min(naturalWidth, maxRatio*
// row width) and pins it to one edge. naturalWidth comes from
// pre-measuring the bubble's text — without that hint, a Label with
// Wrapping=Word reports MinSize.Width equal to its longest word, which
// would collapse the bubble to a vanishingly narrow strip.
type chatRowLayout struct {
	alignRight   bool
	naturalWidth float32
}

// Layout resizes the bubble and positions it. Two Resize calls are
// intentional: the first sets the width so the inner Label wraps and
// updates its MinSize.Height; the second uses that fresh height to
// finalize the bubble's vertical size. Without the second pass the
// bubble would clip when wrap added more lines than MinSize predicted.
func (l *chatRowLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) == 0 {
		return
	}
	child := objects[0]
	maxW := size.Width * chatBubbleMaxRatio
	w := l.naturalWidth
	if w <= 0 || w > maxW {
		w = maxW
	}
	if w < 80 {
		w = 80 // floor so a one-word bubble still reads as a bubble
	}
	child.Resize(fyne.NewSize(w, child.MinSize().Height))
	h := child.MinSize().Height
	child.Resize(fyne.NewSize(w, h))
	if l.alignRight {
		child.Move(fyne.NewPos(size.Width-w, 0))
	} else {
		child.Move(fyne.NewPos(0, 0))
	}
}

// MinSize reports zero width (so the parent VBox doesn't expand) and the
// child's current wrapped height. First-render the height may be off by
// one wrap pass; the explicit scroll.Refresh() after rendering kicks
// Fyne to call Layout, which corrects subsequent reads.
func (l *chatRowLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) == 0 {
		return fyne.Size{}
	}
	return fyne.NewSize(0, objects[0].MinSize().Height)
}

// naturalBubbleWidth measures how wide a bubble needs to be to fit body
// on a single (unwrapped) line per paragraph. fyne.MeasureText gives
// us the rendered pixel width of the longest line; we pad to account
// for the bubble's internal padding (NewPadded ≈ 8px each side) plus
// the colored stroke. Returns 0 for empty input so the caller can fall
// back to the layout's maxRatio cap.
func naturalBubbleWidth(body string) float32 {
	if body == "" {
		return 0
	}
	size := theme.TextSize()
	var maxW float32
	for line := range strings.SplitSeq(body, "\n") {
		w := fyne.MeasureText(line, size, fyne.TextStyle{}).Width
		if w > maxW {
			maxW = w
		}
	}
	// Internal padding + stroke + room for Label's own insets.
	return maxW + 32
}

// buildSimpleBubble wraps a body text in a rounded surface with padding
// — the bubble used for Human prompts. Label.Selectable + Wrapping lets
// the user click-drag to highlight and Cmd/Ctrl+C to copy; the bubble's
// width is governed by chatRowLayout.
func buildSimpleBubble(body string, bg, stroke func() color.Color) fyne.CanvasObject {
	msg := widget.NewLabel(body)
	msg.Wrapping = fyne.TextWrapWord
	msg.Selectable = true
	return wrapBubble(msg, bg, stroke)
}

// buildAgentBubble builds the Agent reply bubble plus an optional
// detached tools bubble. The reply bubble contains the assistant's
// text and (when there are tools) a "▶ N tools" toggle button. The
// tools bubble is full-width and hidden by default; clicking the
// toggle shows/hides it. The two bubbles are separate so the tools'
// long file paths and commands aren't cramped into the 72% chat column.
//
// Tools content is built lazily on first expansion: a step with 20
// tool calls × 3 args each = 60 disabled Entry widgets, and
// constructing all of them up front for every step makes the dialog
// sluggish at scale. We keep an empty placeholder container in the
// bubble and populate it the first time the user clicks the toggle.
// Subsequent toggles are pure Show/Hide.
//
// Returns (replyBubble, toolsBubble). toolsBubble is nil (typed *fyne.
// Container, not interface) when there are no tools — callers can
// nil-check directly without the staticcheck SA4023 false-negative
// that hits when an interface wraps a nil pointer.
func buildAgentBubble(reply string, tools []ToolCall) (fyne.CanvasObject, *fyne.Container) {
	var content []fyne.CanvasObject
	if reply != "" {
		msg := widget.NewLabel(reply)
		msg.Wrapping = fyne.TextWrapWord
		msg.Selectable = true
		content = append(content, msg)
	}

	var toolsBubble *fyne.Container
	if len(tools) > 0 {
		// Empty placeholder list; rows are added on first expand. The
		// tools bubble has no scroll — arg lines wrap into multiple
		// lines when they're too long. Stacked horizontal scrollbars
		// looked broken; vertical wrap is the cleaner trade.
		listVBox := container.NewVBox()
		toolsBubble = wrapBubble(listVBox, colorBubbleTools, colorBubbleToolsStroke)
		toolsBubble.Hide()

		count := len(tools)
		plural := "tool"
		if count != 1 {
			plural = "tools"
		}
		built := false
		var toggle *widget.Button
		expanded := false
		toggle = widget.NewButton(fmt.Sprintf("▶ %d %s", count, plural), func() {
			expanded = !expanded
			if expanded {
				if !built {
					for _, t := range tools {
						listVBox.Add(buildToolRow(t))
					}
					built = true
					listVBox.Refresh()
				}
				toggle.SetText(fmt.Sprintf("▼ %d %s", count, plural))
				toolsBubble.Show()
			} else {
				toggle.SetText(fmt.Sprintf("▶ %d %s", count, plural))
				toolsBubble.Hide()
			}
		})
		toggle.Alignment = widget.ButtonAlignLeading
		content = append(content, toggle)
	}

	inner := container.NewVBox(content...)
	return wrapBubble(inner, colorBubbleAgent, colorBubbleAgentStroke), toolsBubble
}

// wrapBubble layers a content object on top of a rounded-rectangle
// background with a 1px tinted stroke and padding inside. Returned type
// is *fyne.Container so callers can Show / Hide it.
func wrapBubble(content fyne.CanvasObject, bg, stroke func() color.Color) *fyne.Container {
	surface := &dialogBubbleSurface{fill: bg, stroke: stroke}
	surface.ExtendBaseWidget(surface)
	return container.NewStack(surface, container.NewPadded(content))
}

// A widget surface reads the active palette on each theme refresh instead of
// retaining the colors with which a long-lived activity window was opened.
type dialogBubbleSurface struct {
	widget.BaseWidget
	fill, stroke func() color.Color
}

func (s *dialogBubbleSurface) CreateRenderer() fyne.WidgetRenderer {
	renderer := &dialogBubbleRenderer{surface: s, rect: canvas.NewRectangle(s.fill())}
	renderer.Refresh()
	return renderer
}

type dialogBubbleRenderer struct {
	surface *dialogBubbleSurface
	rect    *canvas.Rectangle
}

func (r *dialogBubbleRenderer) Layout(size fyne.Size)        { r.rect.Resize(size) }
func (r *dialogBubbleRenderer) MinSize() fyne.Size           { return fyne.Size{} }
func (r *dialogBubbleRenderer) Destroy()                     {}
func (r *dialogBubbleRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.rect} }
func (r *dialogBubbleRenderer) Refresh() {
	r.rect.FillColor = r.surface.fill()
	r.rect.StrokeColor = r.surface.stroke()
	r.rect.CornerRadius = scaledSize(radiusCard)
	r.rect.StrokeWidth = 1
	r.rect.Refresh()
}

// buildToolRow renders one tool invocation: name in bold, primary arg
// inline next to it, secondary args one-per-line beneath.
//
// Arg values use widget.RichText with TextWrapWord so a 1KB `old_string`
// no longer drags the window to be 1KB wide. Each value is also
// soft-capped (see formatArgValue) so multi-page diffs collapse to a
// preview + length hint — the full content is still recoverable via
// `rgt show <hash>` outside biomelab.
func buildToolRow(t ToolCall) fyne.CanvasObject {
	primaryKey, primaryValue := pickPrimaryArg(t)

	head := dialogHeading("└─ " + t.Name)
	head.TextStyle.Monospace = true

	// Args go into a nested mini-bubble so each tool reads as a self-
	// contained record. The mini-bubble is on a slightly different
	// surface color than the outer tools bubble (colorBackground vs
	// colorPanelBg), producing a subtle "card inside card" depth.
	var argRows []fyne.CanvasObject
	if primaryKey != "" {
		argRows = append(argRows, wrappedArgLine(primaryKey, primaryValue))
	}
	keys := make([]string, 0, len(t.Args))
	for k := range t.Args {
		if k == primaryKey {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		argRows = append(argRows, wrappedArgLine(k, formatArgValue(t.Args[k])))
	}

	if len(argRows) == 0 {
		// Tool with no args: just the title, no empty bubble.
		return head
	}
	argsBox := container.NewVBox(argRows...)
	innerBubble := wrapBubble(argsBox, colorBubbleToolInner, colorBubbleToolInnerStroke)
	return container.NewVBox(head, innerBubble)
}

// wrappedArgLine renders a "   key: value" pair as a selectable Label
// with word-wrap. The tools bubble has no enclosing scroll anymore
// (stacked horizontal scrollbars looked broken), so long values wrap
// into multiple lines inside the bubble. The previous Label-in-scroll
// collapse bug doesn't apply here because there's no scroll wrapping
// the label.
func wrappedArgLine(key, value string) fyne.CanvasObject {
	label := widget.NewLabelWithStyle("   "+key+": "+value, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	label.Wrapping = fyne.TextWrapWord
	label.Selectable = true
	return label
}

// pickPrimaryArg chooses the most informative arg for a tool's primary
// display line: file_path for file ops, command for Bash, query for
// search-like tools. Falls back to "" so the caller can render just the
// tool name.
func pickPrimaryArg(t ToolCall) (string, string) {
	priority := []string{"file_path", "path", "command", "query", "url", "pattern", "prompt"}
	for _, k := range priority {
		if v, ok := t.Args[k]; ok {
			return k, formatArgValue(v)
		}
	}
	return "", ""
}

// formatArgValue renders an arg value as a single line. Strings pass
// through; everything else gets a short %v form. Newlines collapse to
// a single space so a multi-line arg renders as one row — the row's
// own horizontal scroll lets the user see the full content without
// dragging the window.
func formatArgValue(v any) string {
	s := fmt.Sprintf("%v", v)
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\n' || r == '\r' {
			out = append(out, ' ')
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

// Step / ToolCall aliases so this file doesn't have to import the regent
// package's types inline at every signature. These mirror the public
// types one-to-one.
type (
	Step     = regent.Step
	ToolCall = regent.ToolCall
)

// exportRegentLog fetches raw JSON in the background. The session discards
// results if the window closes, refreshes, or switches worktrees meanwhile.
func (a *App) exportRegentLog(session *regentLogSession, wt git.Worktree, parent fyne.Window) {
	session.export(wt.Path, func(data []byte, err error) {
		if err != nil {
			a.showWindowError(parent, err)
			return
		}
		if len(data) == 0 {
			a.showWindowInformation(parent, "No data", "There is no regent activity to export for this worktree.")
			return
		}
		defaultName := fmt.Sprintf("regent-log-%s-%s.json",
			sanitizeFilename(wt.Branch),
			time.Now().Format("20060102-150405"),
		)
		a.saveBytesNative(parent, defaultName, data)
	})
}

// sanitizeFilename strips characters that are awkward in filenames
// (slashes, spaces) so a branch like "feat/new-thing" doesn't try to
// create nested directories on save.
func sanitizeFilename(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch r {
		case '/', '\\', ' ', ':', '*', '?', '"', '<', '>', '|':
			out = append(out, '-')
		default:
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return "branch"
	}
	return string(out)
}
