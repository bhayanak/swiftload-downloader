package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// ---------------------------------------------------------------------------
// sparkline — a compact real-time speed graph drawn as vertical bars.
// ---------------------------------------------------------------------------

const sparkMaxSamples = 24

type sparkline struct {
	widget.BaseWidget
	samples []float64
	peak    float64
}

func newSparkline() *sparkline {
	s := &sparkline{}
	s.ExtendBaseWidget(s)
	return s
}

// push appends a new speed sample (bytes/sec) and redraws.
func (s *sparkline) push(v float64) {
	if v < 0 {
		v = 0
	}
	s.samples = append(s.samples, v)
	if len(s.samples) > sparkMaxSamples {
		s.samples = s.samples[len(s.samples)-sparkMaxSamples:]
	}
	s.peak = 0
	for _, x := range s.samples {
		if x > s.peak {
			s.peak = x
		}
	}
	s.Refresh()
}

// clear resets the graph.
func (s *sparkline) clear() {
	s.samples = nil
	s.peak = 0
	s.Refresh()
}

func (s *sparkline) MinSize() fyne.Size { return fyne.NewSize(80, 18) }

func (s *sparkline) CreateRenderer() fyne.WidgetRenderer {
	return &sparkRenderer{spark: s}
}

type sparkRenderer struct {
	spark    *sparkline
	bars     []*canvas.Rectangle
	lastSize fyne.Size
}

func (r *sparkRenderer) sync() {
	n := len(r.spark.samples)
	for len(r.bars) < n {
		bar := canvas.NewRectangle(accent)
		bar.CornerRadius = 1
		r.bars = append(r.bars, bar)
	}
	if len(r.bars) > n {
		r.bars = r.bars[:n]
	}
}

func (r *sparkRenderer) Layout(size fyne.Size) {
	r.lastSize = size
	r.sync()
	n := len(r.bars)
	if n == 0 {
		return
	}
	gap := float32(2)
	barW := (size.Width - gap*float32(n-1)) / float32(n)
	if barW < 1 {
		barW = 1
	}
	peak := r.spark.peak
	for i, bar := range r.bars {
		h := float32(4)
		if peak > 0 {
			h = float32(r.spark.samples[i]/peak) * size.Height
		}
		if h < 2 {
			h = 2
		}
		x := float32(i) * (barW + gap)
		bar.Resize(fyne.NewSize(barW, h))
		bar.Move(fyne.NewPos(x, size.Height-h))
	}
}

func (r *sparkRenderer) MinSize() fyne.Size { return r.spark.MinSize() }

func (r *sparkRenderer) Refresh() {
	r.sync()
	r.Layout(r.lastSize)
	for _, bar := range r.bars {
		bar.FillColor = accent
		bar.Refresh()
	}
	canvas.Refresh(r.spark)
}

func (r *sparkRenderer) Objects() []fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, len(r.bars))
	for i, bar := range r.bars {
		objs[i] = bar
	}
	return objs
}

func (r *sparkRenderer) Destroy() {}

// ---------------------------------------------------------------------------
// statusBadge — a small coloured dot indicating download state.
// ---------------------------------------------------------------------------

func newStatusBadge() *canvas.Text {
	dot := canvas.NewText("●", color.NRGBA{R: 0x9E, G: 0x9E, B: 0x9E, A: 0xFF})
	dot.TextSize = 14
	return dot
}

// badgeColor maps a row status to a signal colour.
func badgeColor(s rowStatus) color.Color {
	switch s {
	case rowStatusDownloading:
		return color.NRGBA{R: 0x4F, G: 0x6B, B: 0xF6, A: 0xFF} // blue
	case rowStatusCompleted:
		return color.NRGBA{R: 0x2E, G: 0xA0, B: 0x43, A: 0xFF} // green
	case rowStatusPaused:
		return color.NRGBA{R: 0xE6, G: 0xA2, B: 0x3C, A: 0xFF} // amber
	case rowStatusFailed:
		return color.NRGBA{R: 0xD1, G: 0x3A, B: 0x3A, A: 0xFF} // red
	case rowStatusQueued:
		return color.NRGBA{R: 0x7A, G: 0x7A, B: 0x8C, A: 0xFF} // slate
	default:
		return color.NRGBA{R: 0x9E, G: 0x9E, B: 0x9E, A: 0xFF} // grey
	}
}

// ---------------------------------------------------------------------------
// hoverCard — wraps row content in a padded, rounded background that
// highlights on mouse hover.
// ---------------------------------------------------------------------------

type hoverCard struct {
	widget.BaseWidget
	content fyne.CanvasObject
	hovered bool
}

func newHoverCard(content fyne.CanvasObject) *hoverCard {
	c := &hoverCard{content: content}
	c.ExtendBaseWidget(c)
	return c
}

func (c *hoverCard) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(c.idleColor())
	bg.CornerRadius = 8
	return &hoverCardRenderer{card: c, bg: bg}
}

func (c *hoverCard) idleColor() color.Color {
	if fyne.CurrentApp().Settings().ThemeVariant() == theme.VariantLight {
		return color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	}
	return color.NRGBA{R: 0x25, G: 0x28, B: 0x30, A: 0xFF}
}

func (c *hoverCard) hoverColor() color.Color {
	if fyne.CurrentApp().Settings().ThemeVariant() == theme.VariantLight {
		return color.NRGBA{R: 0xEC, G: 0xF0, B: 0xFE, A: 0xFF}
	}
	return color.NRGBA{R: 0x2F, G: 0x34, B: 0x40, A: 0xFF}
}

func (c *hoverCard) MouseIn(*desktop.MouseEvent) {
	c.hovered = true
	c.Refresh()
}

func (c *hoverCard) MouseMoved(*desktop.MouseEvent) {}

func (c *hoverCard) MouseOut() {
	c.hovered = false
	c.Refresh()
}

type hoverCardRenderer struct {
	card *hoverCard
	bg   *canvas.Rectangle
}

func (r *hoverCardRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	padX := theme.Padding()
	padY := theme.Padding() / 2
	r.card.content.Move(fyne.NewPos(padX, padY))
	r.card.content.Resize(fyne.NewSize(size.Width-2*padX, size.Height-2*padY))
}

func (r *hoverCardRenderer) MinSize() fyne.Size {
	m := r.card.content.MinSize()
	return fyne.NewSize(m.Width+2*theme.Padding(), m.Height+theme.Padding())
}

func (r *hoverCardRenderer) Refresh() {
	if r.card.hovered {
		r.bg.FillColor = r.card.hoverColor()
	} else {
		r.bg.FillColor = r.card.idleColor()
	}
	r.bg.Refresh()
	canvas.Refresh(r.card)
}

func (r *hoverCardRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.card.content}
}

func (r *hoverCardRenderer) Destroy() {}
