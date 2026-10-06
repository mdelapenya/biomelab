package gui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/fyne-io/oksvg"

	"github.com/mdelapenya/biomelab/internal/kits"
	"github.com/mdelapenya/biomelab/internal/resourcecache"
)

const kitPageSize = 8

// kitStatusLayout reserves the same footer height for both validation text
// and an empty status, without introducing a scrollbar into the dialog.
type kitStatusLayout struct{}

func (kitStatusLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, object := range objects {
		object.Move(fyne.NewPos(0, 0))
		object.Resize(size)
	}
}

func (kitStatusLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(200, 42) }

// kitPicker keeps selection independent of the currently rendered page.
type kitPicker struct {
	all      []kits.Kit
	selected map[string]bool
	page     int
}

func newKitPicker(sandboxKits, mixins []kits.Kit) *kitPicker {
	all := make([]kits.Kit, 0, len(sandboxKits)+len(mixins))
	all = append(all, sandboxKits...)
	all = append(all, mixins...)
	return &kitPicker{all: all, selected: make(map[string]bool)}
}

func (p *kitPicker) pages() int {
	if len(p.all) == 0 {
		return 1
	}
	return (len(p.all) + kitPageSize - 1) / kitPageSize
}

func (p *kitPicker) visible() []kits.Kit {
	start := p.page * kitPageSize
	end := start + kitPageSize
	if end > len(p.all) {
		end = len(p.all)
	}
	return p.all[start:end]
}

func (p *kitPicker) selectKit(k kits.Kit, checked bool) {
	key := k.OCIReference()
	if checked && k.Kind == kits.KindSandbox {
		for _, other := range p.all {
			if other.Kind == kits.KindSandbox {
				delete(p.selected, other.OCIReference())
			}
		}
	}
	if checked {
		p.selected[key] = true
	} else {
		delete(p.selected, key)
	}
}

func (p *kitPicker) selection() []kits.Kit {
	selected := make([]kits.Kit, 0, len(p.selected))
	for _, k := range p.all {
		if p.selected[k.OCIReference()] {
			selected = append(selected, k)
		}
	}
	return selected
}

func (p *kitPicker) validation() string {
	selected := p.selection()
	var base *kits.Kit
	for i := range selected {
		if selected[i].Kind == kits.KindSandbox {
			base = &selected[i]
			break
		}
	}
	if base == nil {
		return "Choose one sandbox kit to continue."
	}
	agentNames := p.sandboxLineage(*base)
	for _, k := range selected {
		if k.Kind != kits.KindMixin {
			continue
		}
		if (k.Requires.Agent != "" && !agentNames[k.Requires.Agent]) ||
			(k.Extends != "" && !agentNames[k.Extends]) {
			return fmt.Sprintf("%s requires a different sandbox agent; deselect it or choose a compatible sandbox.", kitTitle(k))
		}
	}
	return ""
}

// sandboxLineage follows the v2 spec's bounded extends chain. A parent may be
// a built-in name or another published kit; cycles stop before they can loop.
func (p *kitPicker) sandboxLineage(base kits.Kit) map[string]bool {
	names := map[string]bool{base.Name: true}
	visited := make(map[string]bool)
	current := base
	for range 5 {
		parentRef := current.Extends
		if parentRef == "" || visited[parentRef] {
			break
		}
		visited[parentRef] = true
		names[parentRef] = true // built-in parent names need no catalog entry
		found := false
		for _, candidate := range p.all {
			if candidate.Kind == kits.KindSandbox &&
				(candidate.Name == parentRef || candidate.OCIReference() == parentRef) {
				names[candidate.Name] = true
				current = candidate
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	return names
}

func kitTitle(k kits.Kit) string {
	if k.DisplayName != "" {
		return k.DisplayName
	}
	return k.Name
}

var kitFallbackLogo = fyne.NewStaticResource("kit-fallback.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="48" height="48" viewBox="0 0 48 48"><rect width="48" height="48" rx="9" fill="#2563b8"/><path d="M24 8 39 16v16L24 40 9 32V16L24 8Z" fill="none" stroke="white" stroke-width="2.5" stroke-linejoin="round"/><path d="M9 16 24 24l15-8M24 24v16" fill="none" stroke="white" stroke-width="2.5" stroke-linejoin="round"/></svg>`))

// kitLogo always displays a local fallback immediately. Available HTTPS logos
// replace it after a bounded background request; network work never blocks UI.
func kitLogo(ctx context.Context, k kits.Kit) fyne.CanvasObject {
	return kitLogoWithClient(ctx, k, resourcecache.NewClient(5*time.Second))
}

// kitLogoWithClient allows the image request to be exercised without live Hub traffic.
func kitLogoWithClient(ctx context.Context, k kits.Kit, client *http.Client) fyne.CanvasObject {
	fallback := canvas.NewImageFromResource(kitFallbackLogo)
	fallback.FillMode = canvas.ImageFillContain
	box := container.NewGridWrap(fyne.NewSize(54, 48), fallback)
	parsed, err := url.Parse(k.LogoURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return box
	}
	go func() {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, k.LogoURL, nil)
		if err != nil {
			return
		}
		response, err := client.Do(request)
		if err != nil {
			return
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			return
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20+1))
		if err != nil || len(data) == 0 || len(data) > 1<<20 {
			return
		}
		// Hub media URLs redirect to the asset. The original URL has no image
		// suffix, so use the final response URL and its content type.
		assetURL := parsed
		if response.Request != nil && response.Request.URL != nil {
			assetURL = response.Request.URL
		}
		img := decodedKitLogo(k.Name, data, path.Ext(assetURL.Path), response.Header.Get("Content-Type"))
		if img == nil {
			return
		}
		fyne.Do(func() {
			if ctx.Err() == nil {
				box.Objects = []fyne.CanvasObject{img}
				box.Refresh()
			}
		})
	}()
	return box
}

// decodedKitLogo checks with the same SVG parser and raster decoders that
// Fyne's canvas.Image uses. A rejected asset never replaces the local mark.
func decodedKitLogo(name string, data []byte, extension, contentType string) *canvas.Image {
	ext := strings.ToLower(extension)
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if ext == ".svg" || (ext == "" && mediaType == "image/svg+xml") {
		icon, err := oksvg.ReadReplacingCurrentColor(bytes.NewReader(data), "#000000")
		if err != nil || icon.ViewBox.W <= 0 || icon.ViewBox.H <= 0 ||
			math.IsNaN(icon.ViewBox.W) || math.IsNaN(icon.ViewBox.H) ||
			math.IsInf(icon.ViewBox.W, 0) || math.IsInf(icon.ViewBox.H, 0) {
			return nil
		}
		ext = ".svg"
	} else {
		decoded, format, err := image.Decode(bytes.NewReader(data))
		if err != nil || decoded.Bounds().Empty() {
			return nil
		}
		switch format {
		case "png":
			ext = ".png"
		case "jpeg":
			ext = ".jpg"
		default:
			return nil
		}
	}
	sum := sha256.Sum256(data)
	resourceName := fmt.Sprintf("%s-logo-%x%s", name, sum[:8], ext)
	img := canvas.NewImageFromResource(fyne.NewStaticResource(resourceName, data))
	img.FillMode = canvas.ImageFillContain
	return img
}

func kitCard(ctx context.Context, k kits.Kit, checked bool, onChanged func(bool), onEscape func()) (fyne.CanvasObject, *dialogCheck) {
	check := newDialogCheck("Select", onChanged, onEscape)
	check.Checked = checked
	title := widget.NewLabel(kitTitle(k))
	title.Wrapping = fyne.TextWrapWord
	title.Truncation = fyne.TextTruncateEllipsis
	badge := widget.NewLabel(strings.ToUpper(k.Kind))
	badge.TextStyle = fyne.TextStyle{Bold: true}
	desc := widget.NewLabel(k.Description)
	desc.Wrapping = fyne.TextWrapWord
	desc.Truncation = fyne.TextTruncateEllipsis
	card := widget.NewCard("", "", container.NewVBox(
		container.NewBorder(nil, nil, kitLogo(ctx, k), nil,
			container.NewVBox(container.NewBorder(nil, nil, check, nil, title), badge)),
		desc,
	))
	return card, check
}

// showKitsDialog renders the full Docker Hub kit catalog as bounded pages.
// Exactly one sandbox base and any compatible mixins may be continued.
func showKitsDialog(parent fyne.Window, repoName string, sandboxKits, mixins []kits.Kit,
	onDone func(), onSubmit func([]kits.Kit)) dialog.Dialog {
	picker := newKitPicker(sandboxKits, mixins)
	ctx, cancel := context.WithCancel(context.Background())
	var d *dialog.CustomDialog
	pageLabel := widget.NewLabel("")
	selectedLabel := widget.NewLabel("")
	validation := widget.NewLabel("")
	validation.Wrapping = fyne.TextWrapWord
	validation.Truncation = fyne.TextTruncateEllipsis
	previous := newDialogButton("Previous", nil, func() { d.Hide() })
	next := newDialogButton("Next", nil, func() { d.Hide() })
	continueButton := newDialogButton("Continue", nil, func() { d.Hide() })
	cancelButton := newDialogButton("Cancel", func() { d.Hide() }, func() { d.Hide() })
	columns := 2
	windowSize := parent.Canvas().Size()
	if windowSize.Width > 0 && windowSize.Width < 700 {
		columns = 1
	}
	grid := container.NewGridWithColumns(columns)
	// Card labels truncate within their assigned width, keeping the grid's
	// minimum width stable while retaining vertical-only scrolling.
	cardsViewport := container.NewVScroll(grid)
	validationViewport := container.New(kitStatusLayout{}, validation)
	currentChecks := make(map[string]*dialogCheck)
	updateStatus := func() {
		selectedLabel.SetText(fmt.Sprintf("%d selected", len(picker.selected)))
		message := picker.validation()
		validation.SetText(message)
		if message == "" {
			continueButton.Enable()
		} else {
			continueButton.Disable()
		}
	}
	render := func() {
		objects := make([]fyne.CanvasObject, 0, kitPageSize)
		currentChecks = make(map[string]*dialogCheck)
		var first *dialogCheck
		for _, k := range picker.visible() {
			kit := k
			card, check := kitCard(ctx, kit, picker.selected[kit.OCIReference()], func(checked bool) {
				picker.selectKit(kit, checked)
				for ref, visibleCheck := range currentChecks {
					if visibleCheck.Checked != picker.selected[ref] {
						visibleCheck.Checked = picker.selected[ref]
						visibleCheck.Refresh()
					}
				}
				updateStatus()
			}, func() { d.Hide() })
			currentChecks[kit.OCIReference()] = check
			if first == nil {
				first = check
			}
			objects = append(objects, card)
		}
		if len(picker.all) == 0 {
			objects = append(objects, widget.NewLabel("No Docker Sandbox kits are available."))
		}
		// Keep four rows (or eight on a narrow window) on the final page.
		// Otherwise GridLayout stretches the last card to fill the viewport.
		for len(objects) < kitPageSize {
			objects = append(objects, layout.NewSpacer())
		}
		grid.Objects = objects
		pageLabel.SetText(fmt.Sprintf("Page %d of %d", picker.page+1, picker.pages()))
		updateStatus()
		if picker.page == 0 {
			previous.Disable()
		} else {
			previous.Enable()
		}
		if picker.page+1 >= picker.pages() {
			next.Disable()
		} else {
			next.Enable()
		}
		grid.Refresh()
		if first != nil {
			focusInDialog(parent, first)
		}
	}
	previous.OnTapped = func() {
		if picker.page > 0 {
			picker.page--
			render()
		}
	}
	next.OnTapped = func() {
		if picker.page+1 < picker.pages() {
			picker.page++
			render()
		}
	}
	continueButton.OnTapped = func() {
		if picker.validation() != "" {
			return
		}
		selected := picker.selection()
		d.Hide()
		onSubmit(selected)
	}
	header := widget.NewLabel("Choose one sandbox kit, then optional mixins for " + repoName + ". Selections remain checked across pages.")
	header.Wrapping = fyne.TextWrapWord
	bottom := container.NewVBox(validationViewport, container.NewHBox(previous, pageLabel, next, selectedLabel),
		container.NewHBox(cancelButton, continueButton))
	content := container.NewBorder(header, bottom, nil, nil, cardsViewport)
	d = dialog.NewCustomWithoutButtons("Choose Kits", content, parent)
	d.SetOnClosed(func() { cancel(); onDone() })
	dialogSize := kitsDialogSize
	if windowSize.Width > 0 {
		dialogSize.Width = min(dialogSize.Width, max(320, windowSize.Width-40))
	}
	if windowSize.Height > 0 {
		dialogSize.Height = min(dialogSize.Height, max(300, windowSize.Height-40))
	}
	d.Resize(dialogSize)
	render()
	d.Show()
	return d
}
