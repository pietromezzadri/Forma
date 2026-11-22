package drawing

import (
	"math"

	"forma/geom"
)

// Pen is a line type and width (after ISO 128: wide 0.5 mm, narrow 0.25).
type Pen int

const (
	PenVisible Pen = iota // edges and outlines: wide, continuous
	PenHidden             // hidden edges: narrow, dashed
	PenThin               // tangent edges, dimension and extension lines, leaders
	PenCentre             // centre lines and marks: narrow, long dash and dot
	PenHatch              // section hatching: narrow
	PenCut                // a cutting plane's ends: wide
	PenCutLine            // and its course: narrow chain
	PenFrame              // the title block's rules
	PenBorder             // the drawing frame
	nPens
)

// Width is the pen's line width in mm.
func (p Pen) Width() float64 {
	switch p {
	case PenVisible, PenCut:
		return 0.5
	case PenBorder:
		return 0.7
	case PenFrame:
		return 0.35
	}
	return 0.25
}

// Dash is the pen's pattern (mm: on, off, on, off…), nil for continuous.
func (p Pen) Dash() []float64 {
	switch p {
	case PenHidden:
		return []float64{3, 1.5}
	case PenCentre, PenCutLine:
		return []float64{8, 1.5, 0.5, 1.5}
	}
	return nil
}

// Name is the pen's layer name (DXF) and class (SVG).
func (p Pen) Name() string {
	return [...]string{"VISIBLE", "HIDDEN", "THIN", "CENTER", "HATCH", "CUT", "CUTLINE", "FRAME", "BORDER"}[p]
}

// Path is a polyline on the sheet. When Arc is set it is that circle's arc
// from its first point round to its last (counter-clockwise if CCW), so a
// writer that knows arcs can write one.
type Path struct {
	Tag    int // the view, dimension or note it belongs to (0: the frame)
	Pen    Pen
	Pts    []geom.Vec2
	Closed bool
	Arc    *Arc
	CCW    bool
}

// Fill is a solid black polygon (arrowheads).
type Fill struct {
	Tag int
	Pts []geom.Vec2
}

// Text is a line of text: At is its anchor, Size its font size (mm, the
// em; capitals are 0.72 of it), Angle its direction (degrees,
// counter-clockwise), HAlign 0 left, 1 centre, 2 right of At, VAlign 0 on
// the baseline, 1 middle of the capitals, 2 their top.
type Text struct {
	Tag    int
	At     geom.Vec2
	Size   float64
	Angle  float64
	HAlign int
	VAlign int
	S      string
	Bold   bool
}

// tag marks everything added since mark (lengths of Paths, Fills, Texts)
// as belonging to id.
func (pg *Page) tag(id int, mark [3]int) {
	for i := mark[0]; i < len(pg.Paths); i++ {
		pg.Paths[i].Tag = id
	}
	for i := mark[1]; i < len(pg.Fills); i++ {
		pg.Fills[i].Tag = id
	}
	for i := mark[2]; i < len(pg.Texts); i++ {
		pg.Texts[i].Tag = id
	}
}

func (pg *Page) mark() [3]int { return [3]int{len(pg.Paths), len(pg.Fills), len(pg.Texts)} }

// Page is a drawing ready to write out: sheet size and what is on it.
type Page struct {
	W, H  float64
	Title string
	Paths []Path
	Fills []Fill
	Texts []Text
}

func (pg *Page) line(pen Pen, pts ...geom.Vec2) {
	pg.Paths = append(pg.Paths, Path{Pen: pen, Pts: pts})
}

func (pg *Page) rect(pen Pen, x0, y0, x1, y1 float64) {
	pg.Paths = append(pg.Paths, Path{Pen: pen, Closed: true, Pts: []geom.Vec2{geom.V2(x0, y0), geom.V2(x1, y0), geom.V2(x1, y1), geom.V2(x0, y1)}})
}

func (pg *Page) text(t Text) { pg.Texts = append(pg.Texts, t) }

// Origin is where the text's baseline starts (the anchor moved by its
// alignment) and its direction.
func (t Text) Origin() (geom.Vec2, geom.Vec2) {
	a := t.Angle * math.Pi / 180
	d := geom.V2(math.Cos(a), math.Sin(a))
	up := d.Perp()
	w := TextWidth(t.S, t.Size, t.Bold)
	p := t.At
	switch t.HAlign {
	case 1:
		p = p.Sub(d.Scale(w / 2))
	case 2:
		p = p.Sub(d.Scale(w))
	}
	switch t.VAlign {
	case 1:
		p = p.Sub(up.Scale(t.Size * capHeight / 2))
	case 2:
		p = p.Sub(up.Scale(t.Size * capHeight))
	}
	return p, d
}

// Dashed cuts a polyline into its pen's dashes (for screens that draw
// lines only continuous). The pattern runs on along the whole polyline.
func Dashed(pts []geom.Vec2, closed bool, dash []float64, scale float64) [][]geom.Vec2 {
	if closed && len(pts) > 1 {
		pts = append(append([]geom.Vec2{}, pts...), pts[0])
	}
	if len(dash) == 0 || len(pts) < 2 {
		return [][]geom.Vec2{pts}
	}
	var out [][]geom.Vec2
	k, left := 0, dash[0]*scale
	on := true
	var cur []geom.Vec2
	if on {
		cur = []geom.Vec2{pts[0]}
	}
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		seg := a.Dist(b)
		pos := 0.0
		for seg-pos > left {
			pos += left
			p := a.Lerp(b, pos/seg)
			if on {
				cur = append(cur, p)
				out = append(out, cur)
				cur = nil
			} else {
				cur = []geom.Vec2{p}
			}
			on = !on
			k = (k + 1) % len(dash)
			left = dash[k] * scale
		}
		left -= seg - pos
		if on {
			cur = append(cur, b)
		}
	}
	if on && len(cur) >= 2 {
		out = append(out, cur)
	}
	return out
}

// arcPoints is a circle's arc as a polyline (sheet mm).
func arcPoints(c geom.Vec2, r, a0, a1 float64) []geom.Vec2 {
	n := int(math.Ceil(math.Abs(a1-a0) / (math.Pi / 90)))
	if n < 2 {
		n = 2
	}
	pts := make([]geom.Vec2, n+1)
	for i := 0; i <= n; i++ {
		a := a0 + (a1-a0)*float64(i)/float64(n)
		pts[i] = c.Add(geom.V2(math.Cos(a), math.Sin(a)).Scale(r))
	}
	return pts
}
