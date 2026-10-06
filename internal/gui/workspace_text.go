package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
	"strings"
)

// wrappedValue displays full inspector values with measured natural height.
// It has no selection or focus surface; the explicit note editor remains the
// selectable/editable route for branch and path context.
type wrappedValue struct {
	widget.BaseWidget
	value string
	mono  bool
}

func newWrappedValue(value string, mono bool) *wrappedValue {
	w := &wrappedValue{value: value, mono: mono}
	w.ExtendBaseWidget(w)
	return w
}
func (w *wrappedValue) lines(width float32) []string {
	if width <= 0 {
		width = scaledSize(200)
	}
	style := fyne.TextStyle{Monospace: w.mono}
	size := scaledSize(12)
	var lines []string
	for _, paragraph := range strings.Split(w.value, "\n") {
		remaining := []rune(paragraph)
		if len(remaining) == 0 {
			lines = append(lines, "")
			continue
		}
		for len(remaining) > 0 {
			count, lastSpace := 0, 0
			for i, r := range remaining {
				if fyne.MeasureText(string(remaining[:i+1]), size, style).Width > width {
					break
				}
				count = i + 1
				if r == ' ' {
					lastSpace = count
				}
			}
			if count == 0 {
				count = 1
			}
			if !w.mono && count < len(remaining) && lastSpace > 0 {
				count = lastSpace
			}
			line := string(remaining[:count])
			if !w.mono {
				line = strings.TrimRight(line, " ")
			}
			lines = append(lines, line)
			remaining = remaining[count:]
			if !w.mono {
				for len(remaining) > 0 && remaining[0] == ' ' {
					remaining = remaining[1:]
				}
			}
		}
	}
	return lines
}
func (w *wrappedValue) lineHeight() float32 {
	return fyne.MeasureText("Mg", scaledSize(12), fyne.TextStyle{Monospace: w.mono}).Height
}
func (w *wrappedValue) height(width float32) float32 {
	return float32(len(w.lines(width)))*w.lineHeight() + scaledSize(8)
}
func (w *wrappedValue) CreateRenderer() fyne.WidgetRenderer { return &wrappedValueRenderer{w: w} }

type wrappedValueRenderer struct {
	w     *wrappedValue
	texts []*canvas.Text
}

func (r *wrappedValueRenderer) MinSize() fyne.Size {
	return fyne.NewSize(1, r.w.height(r.w.Size().Width))
}
func (r *wrappedValueRenderer) Layout(size fyne.Size) {
	lines := r.w.lines(size.Width)
	for len(r.texts) < len(lines) {
		t := canvas.NewText("", colorForeground)
		t.TextSize = scaledSize(12)
		t.TextStyle.Monospace = r.w.mono
		r.texts = append(r.texts, t)
	}
	r.texts = r.texts[:len(lines)]
	for i, line := range lines {
		t := r.texts[i]
		t.Text = line
		t.Move(fyne.NewPos(0, scaledSize(4)+float32(i)*r.w.lineHeight()))
		t.Resize(fyne.NewSize(size.Width, r.w.lineHeight()))
		t.Refresh()
	}
}
func (r *wrappedValueRenderer) Objects() []fyne.CanvasObject {
	out := make([]fyne.CanvasObject, len(r.texts))
	for i, t := range r.texts {
		out[i] = t
	}
	return out
}
func (r *wrappedValueRenderer) Refresh() { r.Layout(r.w.Size()) }
func (*wrappedValueRenderer) Destroy()   {}
