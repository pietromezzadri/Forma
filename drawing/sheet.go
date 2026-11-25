package drawing

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"forma/geom"
)

// Sheet is a drawing as the design keeps it: the paper, its views,
// dimensions and notes, and the title block. Everything on it is worked
// out again from the bodies whenever they change.
type Sheet struct {
	ID         int        `json:"id"`
	Name       string     `json:"name"`
	Size       string     `json:"size"`                 // A4…A0, ANSI A…E
	Portrait   bool       `json:"portrait,omitempty"`   // else landscape
	FirstAngle bool       `json:"firstAngle,omitempty"` // else third angle
	Scale      string     `json:"scale"`                // "1:2", "2:1"
	Places     int        `json:"places"`               // decimals dimensions show at most
	Title      TitleBlock `json:"title"`
	Views      []*View    `json:"views,omitempty"`
	Dims       []*Dim     `json:"dims,omitempty"`
	Notes      []*Note    `json:"notes,omitempty"`
	Next       int        `json:"next"`
}

// TitleBlock is what the title block says.
type TitleBlock struct {
	Title    string `json:"title"`
	Number   string `json:"number,omitempty"`
	Material string `json:"material,omitempty"`
	Finish   string `json:"finish,omitempty"`
	Author   string `json:"author,omitempty"`
	Checked  string `json:"checked,omitempty"`
	Date     string `json:"date,omitempty"`
	Revision string `json:"revision,omitempty"`
	Company  string `json:"company,omitempty"`
}

// View is one view on the sheet. A base view looks along a standard
// direction; a projected view and a section are seen from a side of
// another view and stay lined up with it.
type View struct {
	ID       int     `json:"id"`
	Kind     string  `json:"kind"`             // "base", "proj", "section"
	Dir      string  `json:"dir,omitempty"`    // base: front, back, top, bottom, left, right, iso, iso-fl, iso-br, iso-bl
	Parent   int     `json:"parent,omitempty"` // proj, section: the view it is seen from
	Side     string  `json:"side,omitempty"`   // and where it stands: right, left, above, below
	X        float64 `json:"x"`                // its centre on the sheet (mm)
	Y        float64 `json:"y"`
	Scale    string  `json:"scale,omitempty"` // "" = the sheet's
	Hidden   bool    `json:"hidden,omitempty"`
	Tangent  bool    `json:"tangent,omitempty"`
	NoCentre bool    `json:"noCentre,omitempty"`
	NoThread bool    `json:"noThread,omitempty"` // threads left out
	Bodies   []int   `json:"bodies,omitempty"`   // the bodies it shows (keys); all if empty
	// a section: the cutting plane crosses the parent at At along the
	// axis it is seen along (model mm), and Label names it (A → A-A)
	At    float64 `json:"at,omitempty"`
	Label string  `json:"label,omitempty"`
}

// Note is free text on the sheet.
type Note struct {
	ID   int     `json:"id"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Text string  `json:"text"`
	Size float64 `json:"size,omitempty"`
}

// Sizes are the sheet sizes, landscape (mm).
var Sizes = []struct {
	Name string
	W, H float64
}{
	{"A4", 297, 210}, {"A3", 420, 297}, {"A2", 594, 420}, {"A1", 841, 594}, {"A0", 1189, 841},
	{"ANSI A", 279.4, 215.9}, {"ANSI B", 431.8, 279.4}, {"ANSI C", 558.8, 431.8}, {"ANSI D", 863.6, 558.8}, {"ANSI E", 1117.6, 863.6},
}

// Scales are the standard scales (ISO 5455), largest first.
var Scales = []string{"50:1", "20:1", "10:1", "5:1", "2:1", "1:1", "1:2", "1:5", "1:10", "1:20", "1:50", "1:100", "1:200", "1:500"}

// Dirs are the base views' directions.
var Dirs = []string{"front", "back", "top", "bottom", "left", "right", "iso", "iso-fl", "iso-br", "iso-bl"}

// ParseScale reads "1:2" (or "0.5") as a factor.
func ParseScale(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if a, b, ok := strings.Cut(s, ":"); ok {
		x, e1 := strconv.ParseFloat(strings.TrimSpace(a), 64)
		y, e2 := strconv.ParseFloat(strings.TrimSpace(b), 64)
		if e1 != nil || e2 != nil || x <= 0 || y <= 0 {
			return 0, fmt.Errorf("scale %q: want a ratio like 1:2", s)
		}
		return x / y, nil
	}
	x, err := strconv.ParseFloat(s, 64)
	if err != nil || x <= 0 {
		return 0, fmt.Errorf("scale %q: want a ratio like 1:2", s)
	}
	return x, nil
}

// Paper is the sheet's size as it lies (mm).
func (s *Sheet) Paper() (float64, float64) {
	w, h := 297.0, 210.0
	for _, z := range Sizes {
		if z.Name == s.Size {
			w, h = z.W, z.H
		}
	}
	if s.Portrait {
		w, h = h, w
	}
	return w, h
}

// FrameRect is the drawing frame: 20 mm from the left edge (for filing),
// 10 from the others (ISO 5457).
func (s *Sheet) FrameRect() (x0, y0, x1, y1 float64) {
	w, h := s.Paper()
	return 20, 10, w - 10, h - 10
}

// title block size (mm)
const tbW, tbH = 180.0, 40.0

// Area is where views go: the frame less the strip the title block sits in.
func (s *Sheet) Area() (x0, y0, x1, y1 float64) {
	x0, y0, x1, y1 = s.FrameRect()
	return x0, y0 + tbH, x1, y1
}

// View finds a view by ID.
func (s *Sheet) View(id int) *View {
	for _, v := range s.Views {
		if v.ID == id {
			return v
		}
	}
	return nil
}

func (s *Sheet) id() int { s.Next++; return s.Next }

// AddView puts a view on the sheet.
func (s *Sheet) AddView(v *View) *View {
	v.ID = s.id()
	s.Views = append(s.Views, v)
	return v
}

// RemoveView takes a view off, with the views seen from it and its
// dimensions.
func (s *Sheet) RemoveView(id int) {
	gone := map[int]bool{id: true}
	for changed := true; changed; {
		changed = false
		for _, v := range s.Views {
			if !gone[v.ID] && gone[v.Parent] {
				gone[v.ID], changed = true, true
			}
		}
	}
	var vs []*View
	for _, v := range s.Views {
		if !gone[v.ID] {
			vs = append(vs, v)
		}
	}
	s.Views = vs
	var ds []*Dim
	for _, d := range s.Dims {
		if !gone[d.View] {
			ds = append(ds, d)
		}
	}
	s.Dims = ds
}

// NextLabel is the next free section letter.
func (s *Sheet) NextLabel() string {
	used := map[string]bool{}
	for _, v := range s.Views {
		used[v.Label] = true
	}
	for c := 'A'; c <= 'Z'; c++ {
		if c == 'I' || c == 'O' || c == 'Q' {
			continue // read as 1, 0
		}
		if !used[string(c)] {
			return string(c)
		}
	}
	return "Z"
}

// FrameOf is a view's frame: a base view's by its direction, a projected
// view's or a section's from its parent's and its side.
func (s *Sheet) FrameOf(v *View) (Frame, error) {
	for depth := 0; ; depth++ {
		if depth > len(s.Views) {
			return Frame{}, fmt.Errorf("views seen from each other in a loop")
		}
		if v.Kind == "base" || v.Kind == "" {
			return BaseFrame(v.Dir)
		}
		p := s.View(v.Parent)
		if p == nil {
			return Frame{}, fmt.Errorf("the view it is seen from is gone")
		}
		pf, err := s.FrameOf(p)
		if err != nil {
			return Frame{}, err
		}
		return SideFrame(pf, v.Side, s.FirstAngle)
	}
}

// BaseFrame is a standard view's frame. The model's Z is up; front looks
// along +Y.
func BaseFrame(dir string) (Frame, error) {
	z := geom.ZAxis
	switch dir {
	case "front", "":
		return FrameLooking(geom.YAxis, z), nil
	case "back":
		return FrameLooking(geom.YAxis.Neg(), z), nil
	case "top":
		return FrameLooking(z.Neg(), geom.YAxis), nil
	case "bottom":
		return FrameLooking(z, geom.YAxis.Neg()), nil
	case "right":
		return FrameLooking(geom.XAxis.Neg(), z), nil
	case "left":
		return FrameLooking(geom.XAxis, z), nil
	case "iso":
		return FrameLooking(geom.V3(-1, 1, -1), z), nil
	case "iso-fl":
		return FrameLooking(geom.V3(1, 1, -1), z), nil
	case "iso-br":
		return FrameLooking(geom.V3(-1, -1, -1), z), nil
	case "iso-bl":
		return FrameLooking(geom.V3(1, -1, -1), z), nil
	}
	return Frame{}, fmt.Errorf("no view direction %q", dir)
}

// SideFrame is the view seen from side of a view framed p. In third angle
// projection a view stands on the side it is seen from (the top view
// above the front); in first angle, opposite.
func SideFrame(p Frame, side string, first bool) (Frame, error) {
	var d, u geom.Vec3
	switch side {
	case "right":
		d, u = p.R.Neg(), p.U
	case "left":
		d, u = p.R, p.U
	case "above":
		d, u = p.U.Neg(), p.D
	case "below":
		d, u = p.U, p.D.Neg()
	default:
		return Frame{}, fmt.Errorf("no side %q", side)
	}
	if first {
		d = d.Neg()
		if side == "above" || side == "below" {
			u = u.Neg()
		}
	}
	return Frame{R: d.Cross(u), U: u, D: d}, nil
}

// ScaleOf is a view's scale factor.
func (s *Sheet) ScaleOf(v *View) float64 {
	if v.Scale != "" {
		if x, err := ParseScale(v.Scale); err == nil {
			return x
		}
	}
	x, err := ParseScale(s.Scale)
	if err != nil {
		return 1
	}
	return x
}

// Align keeps projected views and sections in line with the views they
// are seen from: one beside stays at its height, one above or below at
// its x.
func (s *Sheet) Align() {
	done := map[int]bool{}
	var align func(v *View, depth int)
	align = func(v *View, depth int) {
		if done[v.ID] || depth > len(s.Views) {
			return
		}
		done[v.ID] = true
		if v.Kind != "proj" && v.Kind != "section" {
			return
		}
		p := s.View(v.Parent)
		if p == nil {
			return
		}
		align(p, depth+1)
		switch v.Side {
		case "left", "right":
			v.Y = p.Y
		case "above", "below":
			v.X = p.X
		}
	}
	for _, v := range s.Views {
		align(v, 0)
	}
}

// --- making a sheet ---

// NewSheet is an empty sheet named name.
func NewSheet(name, size string) *Sheet {
	return &Sheet{Name: name, Size: size, Scale: "1:1", Places: 2, Title: TitleBlock{Title: name}}
}

// Standard lays out the usual views of the bodies — front, top, right and
// an isometric one — at the largest standard scale they fit at.
func (s *Sheet) Standard(bodies []Body) {
	var b geom.Box
	for _, x := range bodies {
		b.Union(x.Body.Bounds())
	}
	if b.Empty() {
		return
	}
	front, _ := BaseFrame("front")
	sideA, sideB := "above", "right" // third angle: top above, right on the right
	if s.FirstAngle {
		sideA = "below" // first angle: the top view below, the left view on the right
	}
	ft, _ := SideFrame(front, sideA, s.FirstAngle)
	fr, _ := SideFrame(front, sideB, s.FirstAngle)
	iso, _ := BaseFrame("iso")
	ext := func(f Frame) (float64, float64) {
		lo, hi := extentIn(b, f)
		return hi.X - lo.X, hi.Y - lo.Y
	}
	wf, hf := ext(front)
	_, ht := ext(ft) // as wide as the front
	wr, hr := ext(fr)
	wi, hi := ext(iso)
	x0, y0, x1, y1 := s.Area()
	W, H := x1-x0, y1-y0
	const g = 18.0 // room round views for dimensions
	// the sheet's scale from the three orthographic views; the isometric
	// view goes in the corner they leave, at the sheet's scale if it fits,
	// else at its own (smaller) one, which it is labelled with
	scale := Scales[len(Scales)-1]
	for _, sc := range Scales {
		k, _ := ParseScale(sc)
		if k*(wf+wr)+3*g <= W && k*(hf+ht)+3*g <= H {
			scale = sc
			break
		}
	}
	s.Scale = scale
	k, _ := ParseScale(scale)
	cellW, cellH := W-3*g-k*wf, H-3*g-k*math.Max(hf, hr)
	ki, isoScale := 0.0, ""
	for _, sc := range Scales {
		x, _ := ParseScale(sc)
		if x <= k && x*wi <= cellW && x*hi <= cellH {
			ki, isoScale = x, sc
			break
		}
	}
	if ki == 0 {
		ki, isoScale = k, ""
	}
	if ki == k {
		isoScale = ""
	}
	// spread the room left over evenly between the views
	colW := math.Max(k*wr, ki*wi)
	rowH := math.Max(k*ht, ki*hi)
	gx := (W - k*wf - colW) / 3
	gy := (H - k*math.Max(hf, hr) - rowH) / 3
	colA := x0 + gx + k*wf/2
	colB := x0 + 2*gx + k*wf + colW/2
	rowLo := y0 + gy + k*math.Max(hf, hr)/2
	rowHi := y0 + 2*gy + k*math.Max(hf, hr) + rowH/2
	if s.FirstAngle {
		rowLo, rowHi = y0+gy+rowH/2, y0+2*gy+rowH+k*math.Max(hf, hr)/2
	}
	fv := s.AddView(&View{Kind: "base", Dir: "front", X: colA, Y: rowLo, Hidden: true, Tangent: true})
	ty := rowHi
	if s.FirstAngle {
		fv.Y, ty = rowHi, rowLo
	}
	s.AddView(&View{Kind: "proj", Parent: fv.ID, Side: sideA, X: colA, Y: ty, Hidden: true, Tangent: true})
	s.AddView(&View{Kind: "proj", Parent: fv.ID, Side: sideB, X: colB, Y: fv.Y, Hidden: true, Tangent: true})
	s.AddView(&View{Kind: "base", Dir: "iso", X: colB, Y: ty, Tangent: true, Scale: isoScale})
	s.Align()
}

// extentIn is a box's outline in a view.
func extentIn(b geom.Box, f Frame) (geom.Vec2, geom.Vec2) {
	lo := geom.V2(math.Inf(1), math.Inf(1))
	hi := geom.V2(math.Inf(-1), math.Inf(-1))
	for i := 0; i < 8; i++ {
		p := f.P(geom.V3(pick(i&1, b.Min.X, b.Max.X), pick(i&2, b.Min.Y, b.Max.Y), pick(i&4, b.Min.Z, b.Max.Z)))
		lo = geom.V2(math.Min(lo.X, p.X), math.Min(lo.Y, p.Y))
		hi = geom.V2(math.Max(hi.X, p.X), math.Max(hi.Y, p.Y))
	}
	return lo, hi
}

// --- building the page ---

// Cache keeps views' projections between builds, by what they depend on.
type Cache struct{ m map[string]*Projection }

func (c *Cache) get(key string, make func() *Projection) *Projection {
	if c == nil {
		return make()
	}
	if c.m == nil {
		c.m = map[string]*Projection{}
	}
	if p, ok := c.m[key]; ok {
		return p
	}
	p := make()
	c.m[key] = p
	return p
}

// Prune forgets projections no build asked for since the last Prune.
func (c *Cache) Keep(keys map[string]bool) {
	for k := range c.m {
		if !keys[k] {
			delete(c.m, k)
		}
	}
}

// Built is a view as built: its projection and where it sits on the sheet.
type Built struct {
	View   *View
	Frame  Frame
	Proj   *Projection
	Scale  float64
	Origin geom.Vec2 // the sheet point of the view's (0, 0)
	Lo, Hi geom.Vec2 // its linework's extent on the sheet
	Err    string
}

// Sheet is a view point on the sheet.
func (b *Built) Sheet(p geom.Vec2) geom.Vec2 { return b.Origin.Add(p.Scale(b.Scale)) }

// SheetOf is a model point on the sheet.
func (b *Built) SheetOf(p geom.Vec3) geom.Vec2 { return b.Sheet(b.Frame.P(p)) }

// Result is a sheet built from the bodies.
type Result struct {
	Page  *Page
	Views map[int]*Built
	Dims  map[int]string // dimensions that couldn't be drawn, and why
	Keys  map[string]bool
}

// Build draws the sheet from the bodies.
func (s *Sheet) Build(bodies []Body, cache *Cache) *Result {
	s.Align()
	w, h := s.Paper()
	pg := &Page{W: w, H: h, Title: s.Name}
	res := &Result{Page: pg, Views: map[int]*Built{}, Dims: map[int]string{}, Keys: map[string]bool{}}
	s.frame(pg)
	s.titleBlock(pg)
	for _, v := range s.Views {
		b := s.buildView(v, bodies, cache, res.Keys)
		res.Views[v.ID] = b
		if b.Err == "" {
			m := pg.mark()
			s.drawView(pg, b)
			pg.tag(v.ID, m)
		}
	}
	// cutting planes over the views they cross
	for _, v := range s.Views {
		if v.Kind == "section" {
			if p, ok := res.Views[v.Parent]; ok && p.Err == "" {
				m := pg.mark()
				s.drawCutLine(pg, res.Views[v.ID], p)
				pg.tag(v.ID, m)
			}
		}
	}
	for _, d := range s.Dims {
		b := res.Views[d.View]
		if b == nil || b.Err != "" {
			res.Dims[d.ID] = "its view is gone"
			continue
		}
		m := pg.mark()
		if err := d.Draw(pg, b, s.Places); err != nil {
			res.Dims[d.ID] = err.Error()
		}
		pg.tag(d.ID, m)
	}
	for _, n := range s.Notes {
		size := n.Size
		if size <= 0 {
			size = 3.5
		}
		m := pg.mark()
		for i, line := range strings.Split(n.Text, "\n") {
			pg.text(Text{At: geom.V2(n.X, n.Y-float64(i)*size*1.5), Size: size, S: line})
		}
		pg.tag(n.ID, m)
	}
	return res
}

func (s *Sheet) shown(v *View, bodies []Body) []Body {
	if len(v.Bodies) == 0 {
		return bodies
	}
	want := map[int]bool{}
	for _, k := range v.Bodies {
		want[k] = true
	}
	var out []Body
	for _, b := range bodies {
		if want[b.Key] {
			out = append(out, b)
		}
	}
	return out
}

func (s *Sheet) buildView(v *View, all []Body, cache *Cache, keys map[string]bool) *Built {
	b := &Built{View: v, Scale: s.ScaleOf(v)}
	fr, err := s.FrameOf(v)
	if err != nil {
		b.Err = err.Error()
		return b
	}
	b.Frame = fr
	bodies := s.shown(v, all)
	if len(bodies) == 0 {
		b.Err = "no bodies to show"
		return b
	}
	var box geom.Box
	for _, x := range bodies {
		box.Union(x.Body.Bounds())
	}
	// the view's centre is the bodies' middle: every view of them lines up
	c := fr.P(box.Center())
	b.Origin = geom.V2(v.X, v.Y).Sub(c.Scale(b.Scale))
	opt := Options{Hidden: v.Hidden, Tangent: v.Tangent, NoThread: v.NoThread}
	var key strings.Builder
	fmt.Fprintf(&key, "%v|%v|%v|%v|%s|%g", fr.R, fr.U, fr.D, opt, v.Kind, v.At)
	for _, x := range bodies {
		fmt.Fprintf(&key, "|%d:%d:%p", x.Key, x.Gen, x.Body)
		if !opt.NoThread {
			for _, t := range x.Threads {
				fmt.Fprintf(&key, ":t%v", t)
			}
		}
	}
	keys[key.String()] = true
	if v.Kind == "section" {
		b.Proj = cache.get(key.String(), func() *Projection {
			o := fr.D.Scale(v.At)
			return Section(bodies, o, fr.D, fr, opt)
		})
	} else {
		b.Proj = cache.get(key.String(), func() *Projection { return Project(bodies, fr, opt) })
	}
	lo, hi := b.Proj.Box[0], b.Proj.Box[1]
	b.Lo, b.Hi = b.Sheet(lo), b.Sheet(hi)
	return b
}

// SectionAt is where a new section's plane goes by default: through the
// middle of the bodies, seen along fr.D.
func SectionAt(bodies []Body, fr Frame) float64 {
	var box geom.Box
	for _, x := range bodies {
		box.Union(x.Body.Bounds())
	}
	return box.Center().Dot(fr.D)
}

func (s *Sheet) drawView(pg *Page, b *Built) {
	pr := b.Proj
	for _, c := range pr.Curves {
		pen := PenVisible
		switch {
		case c.Hidden:
			pen = PenHidden
		case c.Tangent, c.Thread:
			pen = PenThin
		}
		pts := make([]geom.Vec2, len(c.Pts))
		for i, p := range c.Pts {
			pts[i] = b.Sheet(p)
		}
		pa := Path{Pen: pen, Pts: pts}
		if c.Arc != nil && len(pts) >= 3 {
			ac := b.Sheet(c.Arc.C)
			pa.Arc = &Arc{C: ac, R: c.Arc.R * b.Scale}
			pa.CCW = pts[1].Sub(ac).Angle()-pts[0].Sub(ac).Angle() > 0
			if d := pts[1].Sub(ac).Angle() - pts[0].Sub(ac).Angle(); math.Abs(d) > math.Pi {
				pa.CCW = d < 0
			}
			if pts[0].Dist(pts[len(pts)-1]) < 1e-6 {
				pa.Closed = true
			}
		}
		pg.Paths = append(pg.Paths, pa)
	}
	// hatching: 45°, every 2.5 mm on the sheet; bodies side by side
	// alternate direction and spacing so their cut faces read apart
	for _, r := range pr.Regions {
		k := bodyIndex(pr.Bodies, r.Body)
		ang := math.Pi / 4
		if k%2 == 1 {
			ang = 3 * math.Pi / 4
		}
		sp := 2.5 + 0.7*float64(k/2)
		var loops [][]geom.Vec2
		for _, l := range r.Loops {
			sl := make([]geom.Vec2, len(l))
			for j, p := range l {
				sl[j] = b.Sheet(p)
			}
			loops = append(loops, sl)
		}
		for _, sg := range Hatch(loops, ang, sp) {
			pg.line(PenHatch, sg[0], sg[1])
		}
	}
	if !b.View.NoCentre {
		s.centreLines(pg, b)
	}
	// a section's name under it, and the scale when it isn't the sheet's
	var label []string
	if b.View.Kind == "section" && b.View.Label != "" {
		label = append(label, "SECTION "+b.View.Label+"-"+b.View.Label)
	}
	if b.View.Scale != "" && b.View.Scale != s.Scale {
		label = append(label, "SCALE "+b.View.Scale)
	}
	for i, l := range label {
		pg.text(Text{At: geom.V2((b.Lo.X+b.Hi.X)/2, b.Lo.Y-7-float64(i)*5), Size: 3.5, HAlign: 1, VAlign: 2, S: l, Bold: i == 0 && b.View.Kind == "section"})
	}
}

func bodyIndex(bs []Body, key int) int {
	for i, b := range bs {
		if b.Key == key {
			return i
		}
	}
	return 0
}

// centreLines marks holes and round bosses: a cross on every circle seen
// end on, a chain line down the axis of every cylinder or cone seen side
// on. Only round things that go (nearly) all the way round get one, so a
// rounded corner doesn't.
func (s *Sheet) centreLines(pg *Page, b *Built) {
	const over = 3.0 // mm past the outline
	// circles seen end on: by centre, the biggest
	type circ struct {
		c  geom.Vec2
		r  float64
		as []float64
	}
	var circs []*circ
	for _, c := range b.Proj.Curves {
		if c.Arc == nil || c.Thread {
			continue
		}
		var got *circ
		for _, k := range circs {
			if k.c.Dist(c.Arc.C) < 1e-6*math.Max(1, c.Arc.R) && math.Abs(k.r-c.Arc.R) < 1e-6*math.Max(1, c.Arc.R) {
				got = k
			}
		}
		if got == nil {
			got = &circ{c: c.Arc.C, r: c.Arc.R}
			circs = append(circs, got)
		}
		for _, p := range c.Pts {
			got.as = append(got.as, p.Sub(c.Arc.C).Angle())
		}
	}
	type mark struct {
		c geom.Vec2
		r float64
	}
	var marks []mark
	for _, k := range circs {
		if coverage(k.as) < 1.5*math.Pi {
			continue
		}
		merged := false
		for i := range marks {
			if marks[i].c.Dist(k.c) < 1e-6*math.Max(1, k.r) {
				marks[i].r = math.Max(marks[i].r, k.r)
				merged = true
			}
		}
		if !merged {
			marks = append(marks, mark{k.c, k.r})
		}
	}
	for _, m := range marks {
		c := b.Sheet(m.c)
		r := m.r*b.Scale + over
		if m.r*b.Scale < 2 {
			r = m.r*b.Scale + 1.5
		}
		pg.line(PenCentre, c.Add(geom.V2(-r, 0)), c.Add(geom.V2(r, 0)))
		pg.line(PenCentre, c.Add(geom.V2(0, -r)), c.Add(geom.V2(0, r)))
	}
	// axes of round faces seen side on
	type axis struct {
		a, d   geom.Vec2 // a point on it and its direction (view coords)
		t0, t1 float64
	}
	var axes []*axis
	type round struct {
		f      geom.Frame
		r      float64
		as     []float64
		t0, t1 float64
	}
	var rounds []*round
	for _, bd := range b.Proj.Bodies {
		for _, f := range bd.Body.Faces() {
			var fr geom.Frame
			var rad float64
			switch s := f.Surf.(type) {
			case geom.Cylinder:
				fr, rad = s.F, s.R
			case geom.Cone:
				fr, rad = s.F, s.R
			default:
				continue
			}
			if math.Abs(fr.Z.Dot(b.Frame.D)) > 1e-6 {
				continue // not side on
			}
			var got *round
			for _, k := range rounds {
				if k.f.Z.Cross(fr.Z).Len() < 1e-9 && k.f.O.Sub(fr.O).Cross(fr.Z).Len() < 1e-6 && math.Abs(k.r-rad) < 1e-6 {
					got = k
				}
			}
			if got == nil {
				got = &round{f: fr, r: rad, t0: math.Inf(1), t1: math.Inf(-1)}
				rounds = append(rounds, got)
			}
			for _, l := range f.Loops {
				for _, h := range l.HalfEdges() {
					e := h.Edge
					ts := geom.SampleParams(e.Curve, e.T0, e.T1, 0.05, 1e9)
					for _, t := range ts {
						p := e.Curve.Eval(t)
						q := got.f.ToLocal(p)
						got.as = append(got.as, math.Atan2(q.Y, q.X))
						z := p.Sub(got.f.O).Dot(got.f.Z)
						got.t0, got.t1 = math.Min(got.t0, z), math.Max(got.t1, z)
					}
				}
			}
		}
	}
	for _, k := range rounds {
		if coverage(k.as) < 1.5*math.Pi || k.t1 <= k.t0 {
			continue
		}
		a := b.Frame.P(k.f.O)
		d := b.Frame.P(k.f.O.Add(k.f.Z)).Sub(a).Norm()
		merged := false
		for _, x := range axes {
			if math.Abs(x.d.Cross(d)) < 1e-9 && math.Abs(a.Sub(x.a).Cross(x.d)) < 1e-6 {
				// the same line: t along x.d from x.a
				s0 := a.Add(d.Scale(k.t0)).Sub(x.a).Dot(x.d)
				s1 := a.Add(d.Scale(k.t1)).Sub(x.a).Dot(x.d)
				x.t0 = math.Min(x.t0, math.Min(s0, s1))
				x.t1 = math.Max(x.t1, math.Max(s0, s1))
				merged = true
				break
			}
		}
		if !merged {
			axes = append(axes, &axis{a: a, d: d, t0: k.t0, t1: k.t1})
		}
	}
	for _, x := range axes {
		p0 := b.Sheet(x.a.Add(x.d.Scale(x.t0))).Sub(x.d.Scale(over))
		p1 := b.Sheet(x.a.Add(x.d.Scale(x.t1))).Add(x.d.Scale(over))
		pg.line(PenCentre, p0, p1)
	}
}

// coverage is how much of the way round a set of angles reaches.
func coverage(as []float64) float64 {
	if len(as) < 2 {
		return 0
	}
	s := append([]float64{}, as...)
	sort.Float64s(s)
	gap := s[0] + 2*math.Pi - s[len(s)-1]
	for i := 0; i+1 < len(s); i++ {
		gap = math.Max(gap, s[i+1]-s[i])
	}
	return 2*math.Pi - gap
}

// drawCutLine draws a section's cutting plane across the view it is cut
// from: a chain line with wide ends, arrows the way the section looks and
// its letter at each end.
func (s *Sheet) drawCutLine(pg *Page, sec, par *Built) {
	if sec == nil || sec.Err != "" {
		return
	}
	f := par.Frame
	d := sec.Frame.D
	// the plane meets the parent's sheet along the line where p·d = At: in
	// the parent's view coordinates, n·v = At with n = (d·R, d·U)
	n := geom.V2(d.Dot(f.R), d.Dot(f.U))
	if n.Len() < 0.5 {
		return
	}
	n = n.Norm()
	// a point on it: the bodies' middle moved along n
	var box geom.Box
	for _, b := range par.Proj.Bodies {
		box.Union(b.Body.Bounds())
	}
	mid := f.P(box.Center())
	off := sec.View.At - box.Center().Dot(d)
	o := mid.Add(n.Scale(off))
	along := n.Perp()
	lo, hi := par.Proj.Box[0], par.Proj.Box[1]
	half := (math.Abs(along.X)*(hi.X-lo.X) + math.Abs(along.Y)*(hi.Y-lo.Y)) / 2
	c := o.Sub(along.Scale(o.Sub(lo.Add(hi).Scale(0.5)).Dot(along)))
	a := par.Sheet(c.Sub(along.Scale(half))).Sub(along.Scale(5))
	b := par.Sheet(c.Add(along.Scale(half))).Add(along.Scale(5))
	pg.line(PenCutLine, a, b)
	for _, end := range []struct{ p, in geom.Vec2 }{{a, along}, {b, along.Scale(-1)}} {
		pg.line(PenCut, end.p, end.p.Add(end.in.Scale(6)))
		// the arrow, pointing the way the section looks
		tip := end.p.Add(end.in.Scale(1.5)).Add(n.Scale(9))
		base := end.p.Add(end.in.Scale(1.5))
		pg.line(PenThin, base, tip)
		arrow(pg, tip, n, 3.5)
		pg.text(Text{At: tip.Add(n.Scale(2.5)), Size: 5, HAlign: 1, VAlign: 1, S: sec.View.Label, Bold: true})
	}
}

// arrow is a filled arrowhead with its tip at tip, pointing along dir.
func arrow(pg *Page, tip, dir geom.Vec2, l float64) {
	dir = dir.Norm()
	w := dir.Perp().Scale(l / 6)
	back := tip.Sub(dir.Scale(l))
	pg.Fills = append(pg.Fills, Fill{Pts: []geom.Vec2{tip, back.Add(w), back.Sub(w)}})
}

// --- frame and title block ---

func (s *Sheet) frame(pg *Page) {
	x0, y0, x1, y1 := s.FrameRect()
	pg.rect(PenBorder, x0, y0, x1, y1)
	// zones every 50 mm or so: numbers along the top and bottom, letters
	// down the sides (ISO 5457)
	nx := max(2, int(math.Round((x1-x0)/50)/2)*2)
	ny := max(2, int(math.Round((y1-y0)/50)/2)*2)
	w, h := pg.W, pg.H
	for i := 0; i <= nx; i++ {
		x := x0 + (x1-x0)*float64(i)/float64(nx)
		if i > 0 && i < nx {
			pg.line(PenFrame, geom.V2(x, y1), geom.V2(x, h-5))
			pg.line(PenFrame, geom.V2(x, y0), geom.V2(x, 5))
		}
		if i < nx {
			xm := x + (x1-x0)/float64(nx)/2
			lab := strconv.Itoa(i + 1)
			pg.text(Text{At: geom.V2(xm, (y1+h)/2), Size: 3.5, HAlign: 1, VAlign: 1, S: lab})
			pg.text(Text{At: geom.V2(xm, y0/2), Size: 3.5, HAlign: 1, VAlign: 1, S: lab})
		}
	}
	for j := 0; j <= ny; j++ {
		y := y1 - (y1-y0)*float64(j)/float64(ny)
		if j > 0 && j < ny {
			pg.line(PenFrame, geom.V2(x0, y), geom.V2(x0-5, y))
			pg.line(PenFrame, geom.V2(x1, y), geom.V2(w-5, y))
		}
		if j < ny {
			ym := y - (y1-y0)/float64(ny)/2
			lab := string(rune('A' + j))
			pg.text(Text{At: geom.V2(x0-5, ym), Size: 3.5, HAlign: 1, VAlign: 1, S: lab})
			pg.text(Text{At: geom.V2((x1+w)/2, ym), Size: 3.5, HAlign: 1, VAlign: 1, S: lab})
		}
	}
	// centring marks at the middle of each side
	pg.line(PenBorder, geom.V2((x0+x1)/2, y1), geom.V2((x0+x1)/2, h-3))
	pg.line(PenBorder, geom.V2((x0+x1)/2, y0), geom.V2((x0+x1)/2, 3))
	pg.line(PenBorder, geom.V2(x0, (y0+y1)/2), geom.V2(x0-10, (y0+y1)/2))
	pg.line(PenBorder, geom.V2(x1, (y0+y1)/2), geom.V2(w-3, (y0+y1)/2))
}

// TitleBlockRect is where the title block stands.
func (s *Sheet) TitleBlockRect() (x0, y0, x1, y1 float64) {
	_, fy0, fx1, _ := s.FrameRect()
	return fx1 - tbW, fy0, fx1, fy0 + tbH
}

func (s *Sheet) titleBlock(pg *Page) {
	x0, y0, x1, y1 := s.TitleBlockRect()
	pg.rect(PenBorder, x0, y0, x1, y1)
	t := s.Title
	cell := func(cx0, cy0, cx1, cy1 float64, label, value string, size float64, bold bool) {
		pg.rect(PenFrame, cx0, cy0, cx1, cy1)
		pg.text(Text{At: geom.V2(cx0+1.2, cy1-1.2), Size: 1.8, VAlign: 2, S: label})
		if value != "" {
			// shrink what doesn't fit
			for size > 1.8 && TextWidth(value, size, bold) > cx1-cx0-2.4 {
				size *= 0.9
			}
			pg.text(Text{At: geom.V2(cx0+1.2, cy0+1.6), Size: size, S: value, Bold: bold})
		}
	}
	// rows from the top: title / number 14 mm; material, finish, scale,
	// size 9; drawn, checked, date, revision 9; company, sheet, projection 8
	r1, r2, r3 := y1-14, y1-23, y1-32
	cell(x0, r1, x0+120, y1, "TITLE", t.Title, 5, true)
	cell(x0+120, r1, x1, y1, "DRAWING NO.", t.Number, 3.5, false)
	cell(x0, r2, x0+60, r1, "MATERIAL", t.Material, 3, false)
	cell(x0+60, r2, x0+110, r1, "FINISH", t.Finish, 3, false)
	cell(x0+110, r2, x0+145, r1, "SCALE", s.Scale, 3, false)
	sz := s.Size
	if s.Portrait {
		sz += " portrait"
	}
	cell(x0+145, r2, x1, r1, "SIZE", sz, 3, false)
	cell(x0, r3, x0+45, r2, "DRAWN", t.Author, 3, false)
	cell(x0+45, r3, x0+90, r2, "CHECKED", t.Checked, 3, false)
	cell(x0+90, r3, x0+130, r2, "DATE", t.Date, 3, false)
	cell(x0+130, r3, x1, r2, "REVISION", t.Revision, 3, false)
	cell(x0, y0, x0+90, r3, "COMPANY", t.Company, 3, false)
	cell(x0+90, y0, x0+130, r3, "UNITS", "mm", 3, false)
	cell(x0+130, y0, x1, r3, "", "", 3, false)
	// the projection symbol: a truncated cone side on and end on
	s.projectionSymbol(pg, x0+130, y0, x1, r3)
}

func (s *Sheet) projectionSymbol(pg *Page, x0, y0, x1, y1 float64) {
	cy := (y0 + y1) / 2
	h := (y1 - y0) * 0.36 // the cone's big end, half height
	hs := h * 0.55        // its small end
	l := h * 2.2          // its length
	// a cone with its small end to the left, and the view of it from the
	// left: third angle puts that view on the left (the cone's small end
	// facing it), first angle on the right
	cx := (x0 + x1) / 2
	const gap = 3.0
	span := 2*h + gap + l
	circ, cone := cx-span/2+h, cx-span/2+2*h+gap
	if s.FirstAngle {
		cone, circ = cx-span/2, cx+span/2-h
	}
	c := geom.V2(circ, cy)
	pg.Paths = append(pg.Paths, Path{Pen: PenThin, Pts: arcPoints(c, h, 0, 2*math.Pi), Closed: true, Arc: &Arc{C: c, R: h}, CCW: true})
	pg.Paths = append(pg.Paths, Path{Pen: PenThin, Pts: arcPoints(c, hs, 0, 2*math.Pi), Closed: true, Arc: &Arc{C: c, R: hs}, CCW: true})
	pg.Paths = append(pg.Paths, Path{Pen: PenThin, Closed: true, Pts: []geom.Vec2{
		geom.V2(cone, cy-hs), geom.V2(cone+l, cy-h), geom.V2(cone+l, cy+h), geom.V2(cone, cy+hs)}})
	left := math.Min(circ-h, cone) - 1.5
	right := math.Max(circ+h, cone+l) + 1.5
	pg.line(PenCentre, geom.V2(left, cy), geom.V2(right, cy))
	pg.line(PenCentre, geom.V2(circ, cy-h-1.5), geom.V2(circ, cy+h+1.5))
}

// PlaceBeside sets where a projected view or a section goes: on its side
// of the view it is seen from, gap mm clear of it, worked out from the
// bodies' extent in both views (before either is drawn).
func (s *Sheet) PlaceBeside(v *View, bodies []Body, gap float64) {
	p := s.View(v.Parent)
	if p == nil {
		return
	}
	var box geom.Box
	for _, b := range s.shown(v, bodies) {
		box.Union(b.Body.Bounds())
	}
	pf, err1 := s.FrameOf(p)
	vf, err2 := s.FrameOf(v)
	if err1 != nil || err2 != nil || box.Empty() {
		return
	}
	plo, phi := extentIn(box, pf)
	vlo, vhi := extentIn(box, vf)
	ps, vs := s.ScaleOf(p), s.ScaleOf(v)
	v.X, v.Y = p.X, p.Y
	switch v.Side {
	case "right":
		v.X += (phi.X-plo.X)*ps/2 + gap + (vhi.X-vlo.X)*vs/2
	case "left":
		v.X -= (phi.X-plo.X)*ps/2 + gap + (vhi.X-vlo.X)*vs/2
	case "above":
		v.Y += (phi.Y-plo.Y)*ps/2 + gap + (vhi.Y-vlo.Y)*vs/2
	case "below":
		v.Y -= (phi.Y-plo.Y)*ps/2 + gap + (vhi.Y-vlo.Y)*vs/2
	}
}
