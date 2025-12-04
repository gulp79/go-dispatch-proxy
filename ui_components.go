package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// MiniGraph è un widget personalizzato per il grafico a linee
type MiniGraph struct {
	widget.BaseWidget
	Data      []float64
	LineColor color.Color
	MaxVal    float64
}

func NewMiniGraph(col color.Color) *MiniGraph {
	m := &MiniGraph{
		Data:      make([]float64, 50), // Storico di 50 punti
		LineColor: col,
		MaxVal:    100.0,
	}
	m.ExtendBaseWidget(m)
	return m
}

func (m *MiniGraph) AddValue(v float64) {
	// Shift a sinistra
	copy(m.Data, m.Data[1:])
	m.Data[len(m.Data)-1] = v
	if v > m.MaxVal {
		m.MaxVal = v * 1.2 // Auto scale up
	} else if m.MaxVal > 100 && v < m.MaxVal*0.5 {
		m.MaxVal = m.MaxVal * 0.9 // Slow decay scale
	}
	m.Refresh()
}

func (m *MiniGraph) CreateRenderer() fyne.WidgetRenderer {
	return &graphRenderer{m: m}
}

type graphRenderer struct {
	m    *MiniGraph
	line *canvas.Line
	bg   *canvas.Rectangle
}

func (r *graphRenderer) MinSize() fyne.Size {
	return fyne.NewSize(100, 30) // Più grande di prima
}

func (r *graphRenderer) Layout(s fyne.Size) {}

func (r *graphRenderer) Refresh() {
	r.bg.Refresh()
	// Il disegno della linea avviene in Objects o potremmo usare canvas.Path per performance migliori,
	// ma qui usiamo una semplice polyline ricostruita ogni frame per semplicità
}

func (r *graphRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.bg}
	w := r.m.Size().Width
	h := r.m.Size().Height
	step := w / float32(len(r.m.Data)-1)

	// Disegniamo il grafico come una serie di linee connesse
	// Nota: Per performance ottimali in Fyne si dovrebbe usare un singolo canvas.Line con punti multipli o un Path,
	// ma per un mini grafico questo è accettabile.
	for i := 0; i < len(r.m.Data)-1; i++ {
		x1 := float32(i) * step
		y1 := h - (float32(r.m.Data[i]) / float32(r.m.MaxVal) * h)
		x2 := float32(i+1) * step
		y2 := h - (float32(r.m.Data[i+1]) / float32(r.m.MaxVal) * h)
		
		line := canvas.NewLine(color.RGBA{0,0,0,0})
		line.StrokeWidth = 1.5
		line.StrokeColor = r.m.LineColor
		line.Position1 = fyne.NewPos(x1, y1)
		line.Position2 = fyne.NewPos(x2, y2)
		objs = append(objs, line)
	}
	return objs
}

func (r *graphRenderer) Destroy() {}

func (r *graphRenderer) Init() {
	r.bg = canvas.NewRectangle(theme.DisabledColor())
	r.bg.FillColor = color.RGBA{30, 30, 30, 255} // Sfondo scuro fisso per il grafico
}