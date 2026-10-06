package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// newShellLayout keeps a compact project rail beside a flexible workspace.
// Construct it again after palette or zoom changes: the separator captures color.
func newShellLayout(sidebar, workspace, banner fyne.CanvasObject) fyne.CanvasObject {
	var width float32
	return newShellLayoutWithWidth(sidebar, workspace, banner, &width)
}

// Width is a logical user preference, separate from the displayed width that
// must fit the current window. App retains it when theme/zoom rebuild the shell.
func newShellLayoutWithWidth(sidebar, workspace, banner fyne.CanvasObject, width *float32) fyne.CanvasObject {
	layout := &shellLayout{width: width}
	divider := &shellDivider{}
	divider.ExtendBaseWidget(divider)
	// Last in paint/hit-test order so the generous hit area wins over adjacent
	// scrolling content without overlapping repository reorder handles.
	body := container.New(layout, sidebar, workspace, divider)
	divider.onDrag = func(dx float32) {
		*width = layout.clamp(sidebar.Size().Width+dx, body.Size().Width, workspace.MinSize().Width) / scaledSize(1)
		body.Refresh()
	}
	if banner == nil {
		return body
	}
	return container.NewBorder(banner, nil, nil, nil, body)
}

type shellLayout struct{ width *float32 }

func (*shellLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(scaledSize(160)+1+max(scaledSize(300), objects[1].MinSize().Width),
		max(objects[0].MinSize().Height, objects[1].MinSize().Height))
}
func (l *shellLayout) clamp(preferred, available, workspaceMin float32) float32 {
	return min(max(scaledSize(160), preferred), max(float32(0), available-1-max(scaledSize(300), workspaceMin)))
}
func (l *shellLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	preferred := scaledSize(190)
	if *l.width > 0 {
		preferred = scaledSize(*l.width)
	}
	side := l.clamp(preferred, size.Width, objects[1].MinSize().Width)
	objects[0].Move(fyne.Position{})
	objects[0].Resize(fyne.NewSize(side, size.Height))
	objects[1].Move(fyne.NewPos(side+1, 0))
	objects[1].Resize(fyne.NewSize(max(float32(0), size.Width-side-1), size.Height))
	hitWidth := scaledSize(8)
	objects[2].Move(fyne.NewPos(side-(hitWidth-1)/2, 0))
	objects[2].Resize(fyne.NewSize(hitWidth, size.Height))
}

// This divider is pointer-only: resizing must not take keyboard focus.
type shellDivider struct {
	widget.BaseWidget
	onDrag func(float32)
}

func (d *shellDivider) Dragged(e *fyne.DragEvent) { d.onDrag(e.Dragged.DX) }
func (*shellDivider) DragEnd()                    {}
func (*shellDivider) Cursor() desktop.Cursor      { return desktop.HResizeCursor }
func (*shellDivider) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.New(&shellDividerLayout{}, canvas.NewRectangle(colorBorder)))
}

type shellDividerLayout struct{}

func (*shellDividerLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(scaledSize(8), 1)
}
func (*shellDividerLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	objects[0].Move(fyne.NewPos((size.Width-1)/2, 0))
	objects[0].Resize(fyne.NewSize(1, size.Height))
}

// shellIcon constrains decorative icons without exposing focusable controls.
func shellIcon(obj fyne.CanvasObject, size float32) fyne.CanvasObject {
	return container.New(&shellIconLayout{scaledSize(size)}, obj)
}

type shellIconLayout struct{ size float32 }

func (l *shellIconLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(l.size, l.size) }
func (l *shellIconLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	objects[0].Move(fyne.NewPos(max(float32(0), (size.Width-l.size)/2), max(float32(0), (size.Height-l.size)/2)))
	objects[0].Resize(fyne.NewSize(min(l.size, size.Width), min(l.size, size.Height)))
}
