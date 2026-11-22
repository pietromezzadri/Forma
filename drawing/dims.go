package drawing

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"forma/brep"
	"forma/geom"
)

// Anchor ties a dimension to the model, so it follows an edit: an edge by
// the faces either side of it (a silhouette by its face), a model point on
// it near where it was picked, and what of it: the edge itself, the end
// of it nearest P, or its circle's centre.
type Anchor struct {
	Body int       `json:"body"`
	A    string    `json:"a"`
	B    string    `json:"b,omitempty"`
	Sil  bool      `json:"sil,omitempty"` // a silhouette of face A
	P    geom.Vec3 `json:"p"`
	At   string    `json:"at,omitempty"` // "", "end", "centre"
}

// Dim is a dimension in a view.
type Dim struct {
	ID      int       `json:"id"`
	View    int       `json:"view"`
	Kind    string    `json:"kind"` // "h", "v", "a" (aligned), "r", "d", "angle"
	Anchors []Anchor  `json:"anchors"`
	Place   geom.Vec2 `json:"place"`          // where its line and text go: sheet mm from the view's centre
	Text    string    `json:"text,omitempty"` // in place of the value; "<>" is the value
}

// DimKinds name the kinds.
var DimKinds = map[string]string{"h": "Horizontal", "v": "Vertical", "a": "Aligned", "r": "Radius", "d": "Diameter", "angle": "Angle"}

// AddDim puts a dimension on the sheet.
func (s *Sheet) AddDim(d *Dim) *Dim {
	d.ID = s.id()
	s.Dims = append(s.Dims, d)
	return d
}

// RemoveDim takes one off.
func (s *Sheet) RemoveDim(id int) {
	for i, d := range s.Dims {
		if d.ID == id {
			s.Dims = append(s.Dims[:i], s.Dims[i+1:]...)
			return
		}
	}
}

// what an anchor is on the sheet
type geo struct {
	kind int // gPoint, gLine, gCircle
	p, q geom.Vec2
	r    float64 // a circle: centre p, radius r (sheet mm)
}

const (
	gPoint = iota
	gLine
	gCircle
)

// resolve finds what an anchor means in the view as it stands.
func (b *Built) resolve(a Anchor) (geo, error) {
	if a.Sil {
		return b.resolveSil(a)
	}
	var best *brep.Edge
	bd := math.Inf(1)
	for _, bo := range b.Proj.Bodies {
		if bo.Key != a.Body {
			continue
		}
		for _, e := range bo.Body.Edges() {
			if e.HE[0] == nil || e.HE[1] == nil {
				continue
			}
			x, y := e.HE[0].Loop.Face.Name, e.HE[1].Loop.Face.Name
			if !(x == a.A && y == a.B) && !(x == a.B && y == a.A) {
				continue
			}
			if d := edgeDist(e, a.P); d < bd {
				best, bd = e, d
			}
		}
	}
	if best == nil {
		return geo{}, fmt.Errorf("its edge is gone")
	}
	return b.edgeGeo(best, a)
}

func (b *Built) edgeGeo(e *brep.Edge, a Anchor) (geo, error) {
	switch a.At {
	case "end":
		p := e.Verts[0].P
		if e.Verts[1].P.Dist(a.P) < p.Dist(a.P) {
			p = e.Verts[1].P
		}
		return geo{kind: gPoint, p: b.SheetOf(p)}, nil
	case "centre":
		switch c := e.Curve.(type) {
		case geom.Circle:
			return geo{kind: gPoint, p: b.SheetOf(c.F.O)}, nil
		case geom.Ellipse:
			return geo{kind: gPoint, p: b.SheetOf(c.F.O)}, nil
		}
		return geo{}, fmt.Errorf("its edge isn't round")
	}
	if c, ok := e.Curve.(geom.Circle); ok && math.Abs(c.F.Z.Dot(b.Frame.D)) > 1-1e-9 {
		return geo{kind: gCircle, p: b.SheetOf(c.F.O), r: c.R * b.Scale}, nil
	}
	// anything else: a line if it projects onto one, else the point nearest
	ts := geom.SampleParams(e.Curve, e.T0, e.T1, 0.01, 1e9)
	pts := make([]geom.Vec2, len(ts))
	for i, t := range ts {
		pts[i] = b.SheetOf(e.Curve.Eval(t))
	}
	return lineOrPoint(pts, b.SheetOf(a.P)), nil
}

func (b *Built) resolveSil(a Anchor) (geo, error) {
	var best *Curve
	bd := math.Inf(1)
	for i := range b.Proj.Curves {
		c := &b.Proj.Curves[i]
		if c.Body != a.Body || c.Edge != nil || c.Face == nil || c.Face.Name != a.A {
			continue
		}
		for _, p := range c.P3 {
			if d := p.Dist(a.P); d < bd {
				best, bd = c, d
			}
		}
	}
	if best == nil {
		return geo{}, fmt.Errorf("its outline is gone")
	}
	pts := make([]geom.Vec2, len(best.Pts))
	for i, p := range best.Pts {
		pts[i] = b.Sheet(p)
	}
	if a.At == "end" {
		p := pts[0]
		if b.SheetOf(a.P).Dist(pts[len(pts)-1]) < b.SheetOf(a.P).Dist(p) {
			p = pts[len(pts)-1]
		}
		return geo{kind: gPoint, p: p}, nil
	}
	return lineOrPoint(pts, b.SheetOf(a.P)), nil
}

// lineOrPoint is a polyline as a straight line (its two furthest points)
// if it is one, else its point nearest near.
func lineOrPoint(pts []geom.Vec2, near geom.Vec2) geo {
	if len(pts) == 0 {
		return geo{kind: gPoint, p: near}
	}
	a, b := pts[0], pts[len(pts)-1]
	for _, p := range pts {
		if p.Dist(a) > b.Dist(a) {
			b = p
		}
	}
	if a.Dist(b) < 1e-6 {
		return geo{kind: gPoint, p: a}
	}
	straight := true
	for _, p := range pts {
		if distSeg(p, a, b) > 0.02 {
			straight = false
			break
		}
	}
	if straight {
		return geo{kind: gLine, p: a, q: b}
	}
	best := pts[0]
	for _, p := range pts {
		if p.Dist(near) < best.Dist(near) {
			best = p
		}
	}
	return geo{kind: gPoint, p: best}
}

func edgeDist(e *brep.Edge, p geom.Vec3) float64 {
	d := math.Inf(1)
	ts := geom.SampleParams(e.Curve, e.T0, e.T1, 0.05, 1e9)
	for i := 0; i+1 < len(ts); i++ {
		a, b := e.Curve.Eval(ts[i]), e.Curve.Eval(ts[i+1])
		ab := b.Sub(a)
		s := 0.0
		if l2 := ab.Dot(ab); l2 > 0 {
			s = math.Max(0, math.Min(1, p.Sub(a).Dot(ab)/l2))
		}
		d = math.Min(d, a.Add(ab.Scale(s)).Dist(p))
	}
	return d
}

// Pick is the anchor under sheet point p (within tol mm): near an end of
// an edge, that end; near a circle's centre, the centre; else the edge or
// outline nearest.
func (b *Built) Pick(p geom.Vec2, tol float64) (Anchor, bool) {
	if b.Proj == nil {
		return Anchor{}, false
	}
	var best *Curve
	bi := 0
	bd := math.Inf(1)
	for i := range b.Proj.Curves {
		c := &b.Proj.Curves[i]
		for k := 0; k+1 < len(c.Pts); k++ {
			if d := distSeg(p, b.Sheet(c.Pts[k]), b.Sheet(c.Pts[k+1])); d < bd {
				best, bi, bd = c, k, d
			}
		}
	}
	// circles' centres first: a hole is dimensioned to its centre
	for i := range b.Proj.Curves {
		c := &b.Proj.Curves[i]
		if c.Arc != nil && c.Edge != nil && b.Sheet(c.Arc.C).Dist(p) <= tol {
			if a, ok := edgeAnchor(c.Body, c.Edge, c.Edge.Curve.(geom.Circle).F.O); ok {
				a.At = "centre"
				return a, true
			}
		}
	}
	if best == nil || bd > tol {
		return Anchor{}, false
	}
	// the model point under p
	s0, s1 := b.Sheet(best.Pts[bi]), b.Sheet(best.Pts[bi+1])
	t := 0.0
	if l2 := s1.Sub(s0).Dot(s1.Sub(s0)); l2 > 0 {
		t = math.Max(0, math.Min(1, p.Sub(s0).Dot(s1.Sub(s0))/l2))
	}
	m := best.P3[bi].Lerp(best.P3[bi+1], t)
	if best.Edge == nil {
		a := Anchor{Body: best.Body, A: best.Face.Name, Sil: true, P: m}
		ends := []geom.Vec2{b.Sheet(best.Pts[0]), b.Sheet(best.Pts[len(best.Pts)-1])}
		for _, e := range ends {
			if e.Dist(p) <= tol {
				a.At = "end"
			}
		}
		return a, true
	}
	a, ok := edgeAnchor(best.Body, best.Edge, m)
	if !ok {
		return Anchor{}, false
	}
	for _, v := range best.Edge.Verts {
		if best.Edge.Verts[0] == best.Edge.Verts[1] {
			break // a closed edge (a hole's rim) has no ends, only a seam
		}
		if b.SheetOf(v.P).Dist(p) <= tol {
			a.At = "end"
			a.P = v.P
		}
	}
	return a, true
}

func edgeAnchor(body int, e *brep.Edge, p geom.Vec3) (Anchor, bool) {
	if e.HE[0] == nil || e.HE[1] == nil {
		return Anchor{}, false
	}
	return Anchor{Body: body, A: e.HE[0].Loop.Face.Name, B: e.HE[1].Loop.Face.Name, P: p}, true
}

// Value is what a dimension measures (mm, or degrees for an angle).
func (d *Dim) Value(b *Built) (float64, error) {
	m, err := d.measure(b)
	if err != nil {
		return 0, err
	}
	return m.value, nil
}

type measured struct {
	value  float64
	p1, p2 geom.Vec2 // linear: what is measured between; circle: centre, -
	u      geom.Vec2 // linear: the direction measured along
	r      float64
	l1, l2 [2]geom.Vec2 // angle: the two lines
	prefix string
}

func (d *Dim) measure(b *Built) (measured, error) {
	var gs []geo
	for _, a := range d.Anchors {
		g, err := b.resolve(a)
		if err != nil {
			return measured{}, err
		}
		gs = append(gs, g)
	}
	m := measured{}
	switch d.Kind {
	case "h", "v", "a":
		switch len(gs) {
		case 1:
			g := gs[0]
			switch g.kind {
			case gLine:
				m.p1, m.p2 = g.p, g.q
			case gCircle:
				w := geom.V2(1, 0)
				if d.Kind == "v" {
					w = geom.V2(0, 1)
				}
				m.p1, m.p2 = g.p.Sub(w.Scale(g.r)), g.p.Add(w.Scale(g.r))
				m.prefix = "Ø"
			default:
				return m, fmt.Errorf("pick a line, or two points")
			}
		case 2:
			m.p1, m.p2 = pointOf(gs[0], gs[1], d.Kind), pointOf(gs[1], gs[0], d.Kind)
			if d.Kind == "a" {
				// a line and something: square to the line
				if gs[0].kind == gLine {
					m.p1 = foot(m.p2, gs[0])
				} else if gs[1].kind == gLine {
					m.p2 = foot(m.p1, gs[1])
				}
			}
		default:
			return m, fmt.Errorf("pick one or two things")
		}
		switch d.Kind {
		case "h":
			m.u = geom.V2(1, 0)
		case "v":
			m.u = geom.V2(0, 1)
		default:
			m.u = m.p2.Sub(m.p1).Norm()
			if m.u.Len() == 0 {
				return m, fmt.Errorf("the two points are one")
			}
		}
		m.value = math.Abs(m.p2.Sub(m.p1).Dot(m.u)) / b.Scale
	case "r", "d":
		if len(gs) != 1 || gs[0].kind != gCircle {
			return m, fmt.Errorf("pick a circle seen end on")
		}
		m.p1, m.r = gs[0].p, gs[0].r
		m.value = gs[0].r / b.Scale
		m.prefix = "R"
		if d.Kind == "d" {
			m.value *= 2
			m.prefix = "Ø"
		}
	case "angle":
		if len(gs) != 2 || gs[0].kind != gLine || gs[1].kind != gLine {
			return m, fmt.Errorf("pick two straight edges")
		}
		m.l1 = [2]geom.Vec2{gs[0].p, gs[0].q}
		m.l2 = [2]geom.Vec2{gs[1].p, gs[1].q}
		d1, d2 := m.l1[1].Sub(m.l1[0]).Norm(), m.l2[1].Sub(m.l2[0]).Norm()
		if math.Abs(d1.Cross(d2)) < 1e-9 {
			return m, fmt.Errorf("the edges are parallel")
		}
		// the angle between them as their rays from where they meet go
		// toward the dimension's place (worked out when drawn)
		a := math.Acos(math.Max(-1, math.Min(1, math.Abs(d1.Dot(d2)))))
		m.value = a * 180 / math.Pi
	default:
		return m, fmt.Errorf("no dimension kind %q", d.Kind)
	}
	return m, nil
}

// pointOf is what of g a linear dimension measures from (other being the
// other end): a point itself, a circle's centre, a line's middle.
func pointOf(g, other geo, kind string) geom.Vec2 {
	switch g.kind {
	case gLine:
		return g.p.Lerp(g.q, 0.5)
	}
	return g.p
}

func foot(p geom.Vec2, l geo) geom.Vec2 {
	d := l.q.Sub(l.p).Norm()
	return l.p.Add(d.Scale(p.Sub(l.p).Dot(d)))
}

// FormatValue is a dimension's number: at most places decimals, trailing
// zeros dropped.
func FormatValue(v float64, places int) string {
	s := strconv.FormatFloat(v, 'f', places, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	if s == "-0" {
		s = "0"
	}
	return s
}

const (
	arrowLen = 3.0
	dimText  = 3.5
	extGap   = 1.0 // between the outline and an extension line
	extOver  = 2.0 // an extension line past its dimension line
)

func (d *Dim) label(v string, prefix string) string {
	if d.Text != "" {
		return strings.ReplaceAll(d.Text, "<>", v)
	}
	return prefix + v
}

// Draw puts the dimension on the page.
func (d *Dim) Draw(pg *Page, b *Built, places int) error {
	m, err := d.measure(b)
	if err != nil {
		return err
	}
	q := geom.V2(b.View.X, b.View.Y).Add(d.Place)
	switch d.Kind {
	case "h", "v", "a":
		val := FormatValue(m.value, places)
		linear(pg, m.p1, m.p2, m.u, q, d.label(val, m.prefix))
	case "r", "d":
		val := FormatValue(m.value, places)
		round(pg, m.p1, m.r, q, d.Kind == "d", d.label(val, m.prefix))
	case "angle":
		return d.drawAngle(pg, m, q, places)
	}
	return nil
}

// readable turns a text direction so it reads from the bottom or the right.
func readable(u geom.Vec2) geom.Vec2 {
	if u.X < -1e-9 || (math.Abs(u.X) <= 1e-9 && u.Y < 0) {
		return u.Scale(-1)
	}
	return u
}

func deg(v geom.Vec2) float64 { return math.Atan2(v.Y, v.X) * 180 / math.Pi }

// linear draws a dimension between p1 and p2 measured along u, its line
// through q.
func linear(pg *Page, p1, p2, u, q geom.Vec2, text string) {
	n := u.Perp()
	l1 := p1.Add(n.Scale(q.Sub(p1).Dot(n)))
	l2 := p2.Add(n.Scale(q.Sub(p2).Dot(n)))
	for _, e := range [][2]geom.Vec2{{p1, l1}, {p2, l2}} {
		v := e[1].Sub(e[0])
		if v.Len() < extGap+0.5 {
			continue
		}
		w := v.Norm()
		pg.line(PenThin, e[0].Add(w.Scale(extGap)), e[1].Add(w.Scale(extOver)))
	}
	span := l1.Dist(l2)
	dir := l2.Sub(l1).Norm()
	if span < 1e-9 {
		dir = u
	}
	tw := TextWidth(text, dimText, false)
	// the text where q is along the line (in the middle if q is between
	// the ends), the line run on out to it
	s := q.Sub(l1).Dot(dir)
	inside := s >= 0 && s <= span
	if inside {
		s = span / 2
	}
	a, bb := l1, l2
	if !inside {
		if s < 0 {
			a = l1.Add(dir.Scale(s - tw/2 - 1))
		} else {
			bb = l1.Add(dir.Scale(s + tw/2 + 1))
		}
	}
	if span >= 2*arrowLen+1.5 {
		pg.line(PenThin, a, bb)
		arrow(pg, l1, dir.Scale(-1), arrowLen)
		arrow(pg, l2, dir, arrowLen)
	} else {
		// too narrow: arrows outside, pointing in
		pg.line(PenThin, l1.Sub(dir.Scale(arrowLen+4)), l2.Add(dir.Scale(arrowLen+4)))
		if !inside {
			pg.line(PenThin, a, bb)
		}
		arrow(pg, l1, dir, arrowLen)
		arrow(pg, l2, dir.Scale(-1), arrowLen)
	}
	tdir := readable(dir)
	at := l1.Add(dir.Scale(s)).Add(tdir.Perp().Scale(1))
	pg.text(Text{At: at, Size: dimText, Angle: deg(tdir), HAlign: 1, S: text})
}

// round draws a radius or diameter of the circle at c (radius r) toward q.
func round(pg *Page, c geom.Vec2, r float64, q geom.Vec2, dia bool, text string) {
	w := q.Sub(c)
	if w.Len() < 1e-9 {
		w = geom.V2(1, 1)
	}
	w = w.Norm()
	rim := c.Add(w.Scale(r))
	tw := TextWidth(text, dimText, false)
	if q.Dist(c) <= r {
		// inside: along the radius (a diameter right across)
		from := c
		if dia {
			from = c.Sub(w.Scale(r))
			arrow(pg, from, w.Scale(-1), arrowLen)
		}
		pg.line(PenThin, from, rim)
		arrow(pg, rim, w, arrowLen)
		tdir := readable(w)
		pg.text(Text{At: q.Add(tdir.Perp().Scale(1)), Size: dimText, Angle: deg(tdir), HAlign: 1, S: text})
		return
	}
	// outside: a leader to the rim, pointing at it, and a shoulder with the
	// text on it
	pg.line(PenThin, rim, q)
	arrow(pg, rim, w.Scale(-1), arrowLen)
	if dia {
		// carried on through the centre to the far rim
		far := c.Sub(w.Scale(r))
		pg.line(PenThin, c, far)
		arrow(pg, far, w.Scale(-1), arrowLen)
	}
	side := 1.0
	if w.X < 0 {
		side = -1
	}
	end := q.Add(geom.V2(side*(tw+2), 0))
	pg.line(PenThin, q, end)
	h := 0
	if side < 0 {
		h = 2
	}
	pg.text(Text{At: q.Add(geom.V2(side, 1)), Size: dimText, HAlign: h, S: text})
}

func (d *Dim) drawAngle(pg *Page, m measured, q geom.Vec2, places int) error {
	x, ok := meet(m.l1, m.l2)
	if !ok {
		return fmt.Errorf("the edges are parallel")
	}
	ray := func(l [2]geom.Vec2) geom.Vec2 {
		mid := l[0].Lerp(l[1], 0.5)
		if mid.Dist(x) > 1e-6 {
			return mid.Sub(x).Norm()
		}
		return l[1].Sub(l[0]).Norm()
	}
	r1, r2 := ray(m.l1), ray(m.l2)
	rq := q.Sub(x)
	rho := rq.Len()
	if rho < 1e-6 {
		return fmt.Errorf("place it away from where the edges meet")
	}
	// the pair of rays (either way along each line) whose angle holds q
	var a1, a2 geom.Vec2
	found := false
	for _, s1 := range []float64{1, -1} {
		for _, s2 := range []float64{1, -1} {
			u1, u2 := r1.Scale(s1), r2.Scale(s2)
			if within(u1, u2, rq) {
				a1, a2, found = u1, u2, true
			}
		}
	}
	if !found {
		a1, a2 = r1, r2
	}
	ang := math.Acos(math.Max(-1, math.Min(1, a1.Dot(a2))))
	val := FormatValue(ang*180/math.Pi, places)
	text := d.label(val, "") + "°"
	if d.Text != "" {
		text = d.label(val, "")
	}
	// counter-clockwise from t0 to t1
	t0, t1 := a1.Angle(), a2.Angle()
	if a1.Cross(a2) < 0 {
		t0, t1 = t1, t0
	}
	if t1 < t0 {
		t1 += 2 * math.Pi
	}
	pg.Paths = append(pg.Paths, Path{Pen: PenThin, Pts: arcPoints(x, rho, t0, t1), Arc: &Arc{C: x, R: rho}, CCW: true})
	e0 := x.Add(geom.V2(math.Cos(t0), math.Sin(t0)).Scale(rho))
	e1 := x.Add(geom.V2(math.Cos(t1), math.Sin(t1)).Scale(rho))
	arrow(pg, e0, geom.V2(math.Sin(t0), -math.Cos(t0)), arrowLen)
	arrow(pg, e1, geom.V2(-math.Sin(t1), math.Cos(t1)), arrowLen)
	// extension lines where the arc lies beyond the edges
	for _, l := range [][2]geom.Vec2{m.l1, m.l2} {
		for _, e := range []geom.Vec2{e0, e1} {
			dir := e.Sub(x).Norm()
			if math.Abs(dir.Cross(l[1].Sub(l[0]).Norm())) > 1e-6 {
				continue
			}
			far := math.Max(l[0].Sub(x).Dot(dir), l[1].Sub(x).Dot(dir))
			if rho > far+extGap {
				pg.line(PenThin, x.Add(dir.Scale(far+extGap)), x.Add(dir.Scale(rho+extOver)))
			}
		}
	}
	tm := (t0 + t1) / 2
	at := x.Add(geom.V2(math.Cos(tm), math.Sin(tm)).Scale(rho + 1 + dimText*0.4))
	pg.text(Text{At: at, Size: dimText, HAlign: 1, VAlign: 1, S: text})
	return nil
}

// within: c lies in the angle (under 180°) from a to b.
func within(a, b, c geom.Vec2) bool {
	if a.Cross(b) < 0 {
		a, b = b, a
	}
	return a.Cross(c) >= 0 && c.Cross(b) >= 0
}

// meet is where two lines (through their point pairs) cross.
func meet(l1, l2 [2]geom.Vec2) (geom.Vec2, bool) {
	d1, d2 := l1[1].Sub(l1[0]), l2[1].Sub(l2[0])
	den := d1.Cross(d2)
	if math.Abs(den) < 1e-12 {
		return geom.Vec2{}, false
	}
	t := l2[0].Sub(l1[0]).Cross(d2) / den
	return l1[0].Add(d1.Scale(t)), true
}
