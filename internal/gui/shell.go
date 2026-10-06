package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
)

// newShellLayout keeps a compact project rail beside a flexible workspace.
// Construct it again after palette or zoom changes: the separator captures color.
func newShellLayout(sidebar, workspace, banner fyne.CanvasObject) fyne.CanvasObject {
	separator := canvas.NewRectangle(colorBorder)
	body := container.New(&shellLayout{}, sidebar, separator, workspace)
	if banner == nil {
		return body
	}
	return container.NewBorder(banner, nil, nil, nil, body)
}

type shellLayout struct{}

func (*shellLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(scaledSize(160)+1+objects[2].MinSize().Width,
		max(objects[0].MinSize().Height, objects[2].MinSize().Height))
}
func (*shellLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	// Give narrow windows more content room; clamp the rail independently of
	// long repository names and the board's scrollable minimum width.
	side := min(scaledSize(190), max(scaledSize(160), size.Width*0.18))
	side = min(side, max(float32(0), size.Width-1))
	objects[0].Move(fyne.Position{})
	objects[0].Resize(fyne.NewSize(side, size.Height))
	objects[1].Move(fyne.NewPos(side, 0))
	objects[1].Resize(fyne.NewSize(1, size.Height))
	objects[2].Move(fyne.NewPos(side+1, 0))
	objects[2].Resize(fyne.NewSize(max(float32(0), size.Width-side-1), size.Height))
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
