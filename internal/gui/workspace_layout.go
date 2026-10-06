package gui

import "fyne.io/fyne/v2"

// workspacePaneLayout uses a compact master in List and a fixed inspector in
// Board/Grid. At small widths the inspector sits below the browser.
type workspacePaneLayout struct{ list bool }

func (*workspacePaneLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(scaledSize(300), scaledSize(240))
}
func (l *workspacePaneLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	gap := scaledSize(spaceSM)
	if size.Width < scaledSize(620) {
		h := max(float32(0), (size.Height-gap)*0.52)
		objects[0].Move(fyne.Position{})
		objects[0].Resize(fyne.NewSize(size.Width, h))
		objects[1].Move(fyne.NewPos(0, h+gap))
		objects[1].Resize(fyne.NewSize(size.Width, max(float32(0), size.Height-h-gap)))
		return
	}
	left := size.Width - scaledSize(320) - gap
	if l.list {
		left = scaledSize(320)
	}
	objects[0].Move(fyne.Position{})
	objects[0].Resize(fyne.NewSize(left, size.Height))
	objects[1].Move(fyne.NewPos(left+gap, 0))
	objects[1].Resize(fyne.NewSize(size.Width-left-gap, size.Height))
}

// NewShellLayout exposes production shell geometry to the documentation
// screenshot harness without duplicating its sidebar sizing rules.
func NewShellLayout(sidebar, workspace, banner fyne.CanvasObject) fyne.CanvasObject {
	return newShellLayout(sidebar, workspace, banner)
}

type workspaceRootLayout struct{ footer bool }

func (*workspaceRootLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(scaledSize(300), scaledSize(260))
}
func (l *workspaceRootLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	header := objects[0].(*fyne.Container)
	h := header.Layout.(*workspaceToolbarLayout).height(header.Objects, size.Width)
	header.Move(fyne.Position{})
	header.Resize(fyne.NewSize(size.Width, h))
	bodyIndex := 1
	if len(objects) == 3 && !l.footer || len(objects) == 4 {
		main := objects[1]
		main.Move(fyne.NewPos(0, h))
		main.Resize(fyne.NewSize(size.Width, main.MinSize().Height))
		mainHeight := main.MinSize().Height
		main.Resize(fyne.NewSize(size.Width, mainHeight))
		h += mainHeight
		bodyIndex = 2
	}
	footerHeight := float32(0)
	if l.footer {
		footer := objects[len(objects)-1]
		footer.Resize(fyne.NewSize(size.Width, footer.MinSize().Height))
		footerHeight = footer.MinSize().Height
		footer.Move(fyne.NewPos(0, size.Height-footerHeight))
		footer.Resize(fyne.NewSize(size.Width, footerHeight))
	}
	objects[bodyIndex].Move(fyne.NewPos(0, h))
	objects[bodyIndex].Resize(fyne.NewSize(size.Width, max(float32(0), size.Height-h-footerHeight)))
}

type workspaceToolbarLayout struct{}

func (*workspaceToolbarLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(1, scaledSize(40))
}
func (*workspaceToolbarLayout) flow(objects []fyne.CanvasObject, width float32, place bool, y float32) float32 {
	x, h := float32(0), float32(0)
	gap := scaledSize(spaceXS)
	for _, o := range objects {
		s := o.MinSize()
		if x > 0 && x+s.Width > width {
			y += h + gap
			x = 0
			h = 0
		}
		if place {
			o.Move(fyne.NewPos(scaledSize(spaceMD)+x, y))
			o.Resize(s)
		}
		x += s.Width + gap
		h = max(h, s.Height)
	}
	return y + h
}
func (l *workspaceToolbarLayout) height(objects []fyne.CanvasObject, width float32) float32 {
	width -= scaledSize(spaceMD * 2)
	controlsWidth := float32(0)
	for _, o := range objects[1:] {
		controlsWidth += o.MinSize().Width + scaledSize(spaceXS)
	}
	if controlsWidth+scaledSize(100) <= width {
		return scaledSize(44)
	}
	return l.flow(objects[1:], width, false, objects[0].MinSize().Height+scaledSize(spaceXS*2)) + scaledSize(spaceSM)
}
func (l *workspaceToolbarLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	pad := scaledSize(spaceMD)
	width := max(float32(0), size.Width-2*pad)
	controlsWidth := float32(0)
	for _, o := range objects[1:] {
		controlsWidth += o.MinSize().Width + scaledSize(spaceXS)
	}
	if controlsWidth+scaledSize(100) <= width {
		objects[0].Move(fyne.NewPos(pad, scaledSize(spaceSM)))
		objects[0].Resize(fyne.NewSize(width-controlsWidth, scaledSize(30)))
		x := size.Width - pad - controlsWidth
		for _, o := range objects[1:] {
			s := o.MinSize()
			o.Move(fyne.NewPos(x, (size.Height-s.Height)/2))
			o.Resize(s)
			x += s.Width + scaledSize(spaceXS)
		}
	} else {
		objects[0].Move(fyne.NewPos(pad, scaledSize(spaceXS)))
		titleHeight := objects[0].MinSize().Height
		objects[0].Resize(fyne.NewSize(width, titleHeight))
		l.flow(objects[1:], width, true, titleHeight+scaledSize(spaceXS*2))
	}
}

type inspectorPropertyLayout struct{ width float32 }

func (l *inspectorPropertyLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	width := l.width
	if width <= 0 {
		width = scaledSize(300)
	}
	key := min(scaledSize(100), width*0.35)
	value := objects[1].(*wrappedValue)
	return fyne.NewSize(1, max(scaledSize(30), value.height(width-key)))
}
func (l *inspectorPropertyLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	l.width = size.Width
	key := min(scaledSize(100), size.Width*0.35)
	objects[0].Move(fyne.NewPos(0, scaledSize(7)))
	objects[0].Resize(fyne.NewSize(key, objects[0].MinSize().Height))
	objects[1].Move(fyne.NewPos(key, 0))
	objects[1].Resize(fyne.NewSize(max(float32(0), size.Width-key), size.Height))
}

// Refresh after allocating the inspector width so wrapped property heights
// settle on the first frame as well as on later refreshes.
type inspectorPaneLayout struct{}

func (*inspectorPaneLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(1, 32) }
func (*inspectorPaneLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	objects[0].Move(fyne.Position{})
	objects[0].Resize(size)
	objects[0].Refresh()
}

// prominentMainLayout reserves a generous, fixed summary hierarchy. Controls
// wrap only when the window cannot fit their natural widths beside the heading.
type prominentMainLayout struct{ width float32 }

func (l *prominentMainLayout) headerHeight(objects []fyne.CanvasObject, width float32, place bool) float32 {
	controls := float32(0)
	gap := scaledSize(spaceXS)
	for _, o := range objects[4:] {
		controls += o.MinSize().Width + gap
	}
	if width >= controls+scaledSize(110) {
		if place {
			objects[0].Move(fyne.Position{})
			objects[0].Resize(fyne.NewSize(width-controls, scaledSize(22)))
			x := width - controls
			for _, o := range objects[4:] {
				s := o.MinSize()
				o.Move(fyne.NewPos(x, 0))
				o.Resize(s)
				x += s.Width + gap
			}
		}
		return scaledSize(28)
	}
	if place {
		objects[0].Move(fyne.Position{})
		objects[0].Resize(fyne.NewSize(width, scaledSize(20)))
	}
	x, y, h := float32(0), scaledSize(24), float32(0)
	for _, o := range objects[4:] {
		s := o.MinSize()
		if x > 0 && x+s.Width > width {
			y += h + gap
			x = 0
			h = 0
		}
		if place {
			o.Move(fyne.NewPos(x, y))
			o.Resize(s)
		}
		x += s.Width + gap
		h = max(h, s.Height)
	}
	return y + h + gap
}
func (l *prominentMainLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	width := l.width
	if width <= 0 {
		width = scaledSize(1000)
	}
	return fyne.NewSize(1, l.headerHeight(objects, width, false)+scaledSize(46))
}
func (l *prominentMainLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	l.width = size.Width
	y := l.headerHeight(objects, size.Width, true)
	for i, offset := range []float32{0, 20, 37} {
		o := objects[i+1]
		o.Move(fyne.NewPos(0, y+scaledSize(offset)))
		o.Resize(fyne.NewSize(size.Width, o.MinSize().Height))
	}
}
