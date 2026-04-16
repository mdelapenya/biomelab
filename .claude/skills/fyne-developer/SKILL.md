---
name: fyne-developer
description: >
  Technical reference for building desktop GUI applications with Fyne (v2.7+).
  Use when implementing UI features, debugging rendering/layout issues, handling
  keyboard/mouse input, creating custom widgets, or working with dialogs and themes.
  Do NOT use for business logic or non-GUI questions.
metadata:
  author: mdelapenya
  version: 1.0.0
  category: framework-reference
---

# Fyne Developer Reference (v2.7)

Reference for building desktop apps with `fyne.io/fyne/v2`. Derived from source
code analysis of Fyne v2.7.3.

## Official Documentation

- **Main docs:** https://docs.fyne.io/
- **Getting started:** https://docs.fyne.io/started/
- **Widget list:** https://docs.fyne.io/explore/widgets
- **Container/layout:** https://docs.fyne.io/explore/container
- **Canvas:** https://docs.fyne.io/explore/canvas
- **Shortcuts:** https://docs.fyne.io/explore/shortcuts
- **Key handling:** https://docs.fyne.io/explore/keys
- **Dialogs:** https://docs.fyne.io/explore/dialogs
- **Themes:** https://docs.fyne.io/explore/themes
- **Custom widgets:** https://docs.fyne.io/extend/custom-widget
- **Testing:** https://docs.fyne.io/started/testing
- **Goroutines:** https://docs.fyne.io/started/goroutines
- **API reference (v2.7):** https://docs.fyne.io/api/v2.7/
- **GitHub:** https://github.com/fyne-io/fyne

---

## Core Architecture

Fyne uses a **retained-mode** rendering system. Widgets are objects in a tree;
the framework re-renders dirty subtrees on the next frame. All UI mutations
must happen on the **main thread** — use `fyne.Do(func() { ... })` to schedule
work from goroutines.

```go
import (
    "fyne.io/fyne/v2"
    fyneapp "fyne.io/fyne/v2/app"
)

a := fyneapp.NewWithID("com.example.myapp")
w := a.NewWindow("Title")
w.Resize(fyne.NewSize(800, 600))
w.SetContent(myWidget)
w.ShowAndRun() // blocks until window closes
```

---

## Widgets (`fyne.io/fyne/v2/widget`)

| Widget | Description |
|--------|-------------|
| `widget.Label` | Static text. `.Alignment`, `.Wrapping`, `.Truncation` |
| `widget.Button` | Clickable. `NewButton(text, func())` |
| `widget.Entry` | Text input. `.SetPlaceHolder()`, `.OnSubmitted`, `.OnChanged` |
| `widget.RichText` | Styled text segments. `NewRichText(segments...)` |
| `widget.Select` | Dropdown. `NewSelect(options, onChange)` |
| `widget.Check` | Checkbox. `NewCheck(label, onChange)` |
| `widget.RadioGroup` | Radio buttons. `NewRadioGroup(options, onChange)` |
| `widget.Slider` | Numeric slider. `NewSlider(min, max)` |
| `widget.ProgressBar` | Progress indicator |
| `widget.Separator` | Horizontal line divider |
| `widget.Card` | Titled card container. `NewCard(title, subtitle, content)` |
| `widget.Accordion` | Collapsible sections |
| `widget.List` | Scrollable list with recycled items |
| `widget.Tree` | Hierarchical tree (**implements Focusable — steals keyboard focus**) |
| `widget.Table` | Grid with rows/columns |
| `widget.Form` | Structured form with validation |
| `widget.Toolbar` | Action buttons bar |

### Custom Widgets

```go
type MyWidget struct {
    widget.BaseWidget
    // your fields
}

func NewMyWidget() *MyWidget {
    w := &MyWidget{}
    w.ExtendBaseWidget(w)
    return w
}

func (w *MyWidget) CreateRenderer() fyne.WidgetRenderer {
    return widget.NewSimpleRenderer(myContent)
}
```

Key interfaces:
- `fyne.Tappable` — `Tapped(*PointEvent)` — click handling, does NOT steal focus
- `fyne.Focusable` — `FocusGained()`, `FocusLost()`, `TypedKey(*KeyEvent)`, `TypedRune(rune)` — **steals keyboard focus from canvas handlers**
- `fyne.Draggable` — `Dragged(*DragEvent)`, `DragEnd()`
- `desktop.Hoverable` — `MouseIn(*MouseEvent)`, `MouseMoved(*MouseEvent)`, `MouseOut()`
- `desktop.Keyable` — `KeyDown(*KeyEvent)`, `KeyUp(*KeyEvent)`

---

## Containers & Layouts (`fyne.io/fyne/v2/container`)

| Container | Usage |
|-----------|-------|
| `container.NewVBox(objects...)` | Vertical stack |
| `container.NewHBox(objects...)` | Horizontal stack |
| `container.NewBorder(top, bottom, left, right, center)` | Border layout (fixed edges, flexible center) |
| `container.NewStack(objects...)` | Z-order stack (all same size) |
| `container.NewGridWrap(cellSize, objects...)` | Grid with fixed cell size, wraps to fill width |
| `container.NewGridWithColumns(n, objects...)` | Grid with N fixed columns |
| `container.NewGridWithRows(n, objects...)` | Grid with N fixed rows |
| `container.NewHSplit(leading, trailing)` | Horizontal split with draggable divider. `.Offset = 0.15` |
| `container.NewVSplit(top, bottom)` | Vertical split |
| `container.NewScroll(content)` | Scrollable wrapper |
| `container.NewPadded(content)` | Adds theme padding |
| `container.NewCenter(content)` | Centers content |

### Grid Navigation

`NewGridWrap(cellSize, ...)` arranges items left-to-right, top-to-bottom.
To compute column count: `cols = int(containerWidth / cellSize.Width)`.
Navigate: left/right ±1, up/down ±cols.

---

## Canvas Primitives (`fyne.io/fyne/v2/canvas`)

```go
rect := canvas.NewRectangle(color)
rect.CornerRadius = 6           // rounded corners (v2.3+)
rect.StrokeColor = borderColor  // border
rect.StrokeWidth = 2

text := canvas.NewText("hello", color.White)
text.TextStyle.Monospace = true
text.TextStyle.Bold = true
text.TextSize = 12
text.Alignment = fyne.TextAlignCenter

img := canvas.NewImageFromFile("icon.png")
line := canvas.NewLine(color.White)
circle := canvas.NewCircle(color.Blue)
```

---

## Keyboard Input — CRITICAL KNOWLEDGE

### The Focus Problem

Fyne delivers keyboard events to the **focused widget first**. Canvas-level
handlers (`SetOnTypedKey`, `SetOnTypedRune`) only fire when
`canvas.Focused() == nil`. Any widget implementing `fyne.Focusable`
(e.g., `widget.Tree`, `widget.Entry`) steals focus on click.

**Source:** `internal/driver/glfw/window.go:715-721`
```go
focused := w.canvas.Focused()
if focused != nil {
    focused.TypedKey(keyEvent)   // widget gets it
} else if w.canvas.onTypedKey != nil {
    w.canvas.onTypedKey(keyEvent) // canvas handler gets it
}
```

### Solution: desktop.Canvas.SetOnKeyDown

`desktop.Canvas.SetOnKeyDown` fires BEFORE focus dispatch and Tab
interception (`window.go:696-701`). This is the only reliable way to
handle Tab and Escape globally.

```go
import "fyne.io/fyne/v2/driver/desktop"

if dc, ok := canvas.(desktop.Canvas); ok {
    dc.SetOnKeyDown(func(ev *fyne.KeyEvent) {
        // Fires for EVERY key press, before widget focus
        // BUT only when canvas.Focused() == nil
    })
}
```

**Caveat:** `SetOnKeyDown` also only fires when `canvas.Focused() == nil`
(line 696-701 is an if/else). When a dialog overlay has a focused Entry,
neither `SetOnKeyDown` nor `SetOnTypedKey` fire.

### The Real Solution: No Focusable Children

**Remove all `fyne.Focusable` widgets from the content tree.** Replace
`widget.Tree` with tappable VBox items. Use `tappableCard` (implements
`Tappable` only, NOT `Focusable`). Then `canvas.Focused()` is always nil
and all canvas handlers fire.

### Tab Key

Tab is consumed by Fyne for focus navigation (`window.go:710`):
```go
if keyName == fyne.KeyTab && !w.capturesTab(modifier) {
    return // consumed, never reaches TypedKey
}
```

`capturesTab` calls `canvas.FocusNext()` then returns false. The `!false`
makes the condition true → early return. **Tab never reaches
`SetOnTypedKey`.**

Fix: use `desktop.Canvas.SetOnKeyDown` which fires at line 700, before
the Tab check at line 710.

### Double-Fire Bug

`SetOnKeyDown` (line 700) and `SetOnTypedKey` (line 719) BOTH fire for
the initial key press. Do NOT register both for the same handler — it
will double-fire. Use only `SetOnKeyDown`.

Key repeat (holding a key) only fires through `SetOnTypedKey` (line 703:
`default` case). If you need key repeat, track state to skip the
duplicate initial press.

### AddShortcut Limitation

`Canvas.AddShortcut` with `desktop.CustomShortcut{Modifier: 0}` does NOT
work. Fyne's `triggersShortcut` (`window.go:817`) requires `modifier != 0`:
```go
if shortcut == nil && modifier != 0 && ... {
    shortcut = &desktop.CustomShortcut{...}
}
```
Plain letter keys without modifier are never dispatched as shortcuts.

---

## Dialogs (`fyne.io/fyne/v2/dialog`)

```go
// Confirm dialog (OK/Cancel)
d := dialog.NewConfirm("Title", "Message", func(ok bool) {
    // ok=true for confirm, ok=false for cancel
}, parent)
d.Resize(fyne.NewSize(450, 200)) // set minimum size
d.Show()

// Custom content dialog
d := dialog.NewCustomConfirm("Title", "OK", "Cancel", content, func(ok bool) {
    // fires for BOTH confirm and cancel
}, parent)
d.Resize(fyne.NewSize(450, 200))
d.Show()

// Custom dialog (dismiss button only)
d := dialog.NewCustom("Title", "Close", content, parent)
d.SetOnClosed(func() { /* cleanup */ })
d.Resize(fyne.NewSize(450, 200))
d.Show()

// Error / Info
dialog.ShowError(err, parent)
dialog.ShowInformation("Title", "Message", parent)
```

### Dialog Focus and Escape

- Fyne dialogs create an **overlay** with its own focus manager
- `dialog.NewCustomConfirm` callback fires for BOTH OK and Cancel
- Fyne dialogs do **NOT** handle Escape natively — implement it yourself
- When an Entry widget inside a dialog has focus, canvas-level key
  handlers do NOT fire (the overlay's focus manager takes priority)
- Fix: don't auto-focus Entry widgets in dialogs. When no widget in
  the overlay has focus, canvas handlers fire and can intercept Escape
- After dialog closes, call `canvas.Unfocus()` to clear stale focus state

### Overlay Management

```go
overlays := window.Canvas().Overlays()
top := overlays.Top()       // get top overlay
overlays.Remove(top)        // dismiss it (for Escape handling)
```

---

## Theming (`fyne.io/fyne/v2/theme`)

```go
type myTheme struct{}

func (t *myTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
    switch name {
    case theme.ColorNameBackground:
        return color.NRGBA{R: 28, G: 33, B: 40, A: 255}
    default:
        return theme.DefaultTheme().Color(name, variant)
    }
}

func (t *myTheme) Font(style fyne.TextStyle) fyne.Resource {
    return theme.DefaultTheme().Font(style)
}

func (t *myTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
    return theme.DefaultTheme().Icon(name)
}

func (t *myTheme) Size(name fyne.ThemeSizeName) float32 {
    return theme.DefaultTheme().Size(name)
}

// Apply:
app.Settings().SetTheme(&myTheme{})
```

Key color names: `ColorNameBackground`, `ColorNameForeground`,
`ColorNamePrimary`, `ColorNameFocus`, `ColorNameSeparator`,
`ColorNameInputBackground`, `ColorNameButton`, `ColorNameScrollBar`,
`ColorNameShadow`, `ColorNameOverlayBackground`, `ColorNameSelection`,
`ColorNameHover`, `ColorNamePlaceHolder`, `ColorNameDisabled`.

Key size names: `SizeNameText`, `SizeNamePadding`, `SizeNameInnerPadding`,
`SizeNameScrollBar`, `SizeNameSeparatorThickness`.

---

## Async / Goroutines

**All UI mutations must happen on the main thread.**

```go
// From a goroutine:
go func() {
    result := doExpensiveWork()
    fyne.Do(func() {
        // Safe to update widgets here
        label.SetText(result)
        myWidget.Refresh()
    })
}()
```

`fyne.Do()` (v2.5+) schedules a function on the next event loop tick.

### Periodic Refresh

```go
go func() {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            data := fetchData()
            fyne.Do(func() {
                updateUI(data)
            })
        }
    }
}()
```

---

## Testing (`fyne.io/fyne/v2/test`)

```go
import "fyne.io/fyne/v2/test"

app := test.NewApp()
defer app.Quit()
w := app.NewWindow("test")

// Tap a widget
test.Tap(button)

// Type text
test.Type(entry, "hello")

// Get rendered image
img := w.Canvas().Capture()
```

---

## Build Requirements

Fyne requires **CGo** and system graphics libraries:

```bash
# Linux
sudo apt install libgl1-mesa-dev xorg-dev gcc

# Build
CGO_ENABLED=1 go build -tags fyne ./cmd/myapp
```

Build tags: use `//go:build fyne` / `//go:build !fyne` to separate
GUI code from non-GUI code in the same package.

---

## Zoom / Font Scaling

The biomelab theme supports runtime font scaling via `biomeTheme.ZoomIn()`,
`ZoomOut()`, `ZoomReset()`. After changing the size, call
`app.Settings().SetTheme(t)` to force a full re-render. Custom widgets
using `canvas.Text` with hardcoded `TextSize` won't scale automatically —
they need to be rebuilt (call `Rebuild()` on dashboard and `rebuildList()`
on repo panel).

Zoom shortcuts use `Canvas.AddShortcut` with `desktop.CustomShortcut`
and modifier keys (Ctrl/Cmd), which works because Fyne's
`triggersShortcut` requires `modifier != 0`.

Register both `Ctrl` and `Super` (Cmd) variants for cross-platform, and
also `Ctrl+Shift+=` since `+` is `Shift+=` on most keyboards.

---

## Common Pitfalls

1. **widget.Tree steals keyboard focus** — Replace with tappable VBox if
   you need global key handling
2. **Tab never reaches TypedKey** — Use `desktop.Canvas.SetOnKeyDown`
3. **SetOnKeyDown + SetOnTypedKey double-fires** — Use only SetOnKeyDown
4. **AddShortcut with Modifier:0 doesn't work** — Fyne requires modifier != 0
5. **Dialog Entry steals focus** — Don't auto-focus Entry in dialogs if
   you need canvas-level Escape handling
6. **canvas.Text doesn't clip** — It expands to full text width. Truncate
   strings manually before creating the Text object
7. **Dialogs are tiny by default** — Always call `d.Resize(minSize)`
8. **Dialogs don't handle Escape** — Implement Escape dismissal yourself
   via overlay removal
9. **fyne.Do required from goroutines** — All widget mutations from
   background goroutines must use `fyne.Do(func() { ... })`
10. **canvas.Focused() checks overlay first** — When a dialog is showing,
    `Focused()` returns the overlay's focused widget, not the content's
