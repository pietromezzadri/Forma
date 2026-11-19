package io

// STEP import: the solids (MANIFOLD_SOLID_BREP, BREP_WITH_VOIDS) of an
// AP203 / AP214 / AP242 file whose faces lie on planes, cylinders, cones,
// spheres and tori. Lines and circles stay exact; ellipses and B-spline
// edges become polylines pulled onto both faces they bound.

import (
	"fmt"
	stdio "io"
	"math"
	"sort"

	"forma/brep"
	"forma/geom"
)

// StepSolid is one solid read from a file.
type StepSolid struct {
	Body *brep.Body
	Err  error // why it couldn't be read (Body nil)
	Name string
}

// ReadSTEP reads every solid in a STEP file. Solids that use geometry Forma
// can't represent come back with Err set; the error is for the file itself.
func ReadSTEP(r stdio.Reader) ([]StepSolid, error) {
	src, err := stdio.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return ReadSTEPString(string(src))
}

func ReadSTEPString(src string) ([]StepSolid, error) {
	f, err := parseP21(src)
	if err != nil {
		return nil, err
	}
	rd := &stepReader{f: f, scale: 1, angle: 1}
	rd.units()
	places := rd.placements()
	products := rd.productNames()
	var out []StepSolid
	for _, id := range f.Order {
		e := f.Ents[id]
		if !e.Is("MANIFOLD_SOLID_BREP") && !e.Is("BREP_WITH_VOIDS") {
			continue
		}
		name := products[rd.solidRep[id]]
		if a := e.Parts[0].Args; len(a) > 0 && a[0].Kind == p21Str && a[0].Str != "" && (name == "" || len(products) == 0) {
			name = a[0].Str
		}
		if name == "" {
			name = fmt.Sprintf("Solid %d", len(out)+1)
		}
		b, err := rd.solid(e, name)
		xs := places[id]
		if len(xs) == 0 {
			xs = []geom.Xform{geom.Identity}
		}
		for k, x := range xs {
			n := name
			if len(xs) > 1 {
				n = fmt.Sprintf("%s (%d)", name, k+1)
			}
			if err != nil {
				out = append(out, StepSolid{Err: err, Name: n})
				continue
			}
			pb := b
			if x != geom.Identity {
				if pb, err = brep.Transform(b, x, nil); err != nil {
					out = append(out, StepSolid{Err: err, Name: n})
					continue
				}
			}
			pb.Name = n
			out = append(out, StepSolid{Body: pb, Name: n})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("the file has no solids")
	}
	return out, nil
}

type stepReader struct {
	f            *p21File
	scale, angle float64     // to mm, to radians
	solidRep     map[int]int // the representation each solid is in

	// per solid
	b     *brep.Builder
	verts map[int]*brep.Vertex
	edges map[int]*stepEdge
	surfs map[int]geom.Surface
}

type stepEdge struct {
	e        *brep.Edge
	sampled  bool // a polyline made here, to pull onto its faces
	surfaces []geom.Surface
}

func (r *stepReader) ent(v p21Val) (*p21Ent, error) {
	if v.Kind != p21Ref {
		return nil, fmt.Errorf("expected a reference")
	}
	e := r.f.Ents[v.Ref]
	if e == nil {
		return nil, fmt.Errorf("#%d is missing", v.Ref)
	}
	return e, nil
}

// args are the parameters of an instance's part of type t (or its first).
func args(e *p21Ent, t string) []p21Val {
	if p := e.Part(t); p != nil {
		return p.Args
	}
	return e.Parts[0].Args
}

func arg(a []p21Val, i int) p21Val {
	if i < len(a) {
		return a[i]
	}
	return p21Val{Kind: p21Null}
}

func num(v p21Val) (float64, error) {
	switch v.Kind {
	case p21Num:
		return v.Num, nil
	case p21Typed:
		if len(v.List) == 1 && v.List[0].Kind == p21Num {
			return v.List[0].Num, nil
		}
	}
	return 0, fmt.Errorf("expected a number")
}

// units finds the file's length and angle units.
func (r *stepReader) units() {
	for _, id := range r.f.Order {
		e := r.f.Ents[id]
		p := e.Part("GLOBAL_UNIT_ASSIGNED_CONTEXT")
		if p == nil || len(p.Args) == 0 {
			continue
		}
		for _, u := range p.Args[0].List {
			ue, err := r.ent(u)
			if err != nil {
				continue
			}
			switch {
			case ue.Is("LENGTH_UNIT"):
				r.scale = r.unitFactor(ue, 0) * 1000 // metres to mm
			case ue.Is("PLANE_ANGLE_UNIT"):
				r.angle = r.unitFactor(ue, 0)
			}
		}
		return
	}
}

var siPrefix = map[string]float64{"EXA": 1e18, "PETA": 1e15, "TERA": 1e12, "GIGA": 1e9, "MEGA": 1e6,
	"KILO": 1e3, "HECTO": 1e2, "DECA": 1e1, "DECI": 1e-1, "CENTI": 1e-2, "MILLI": 1e-3,
	"MICRO": 1e-6, "NANO": 1e-9, "PICO": 1e-12, "FEMTO": 1e-15, "ATTO": 1e-18}

// unitFactor is a unit's size in SI base units (metres, radians).
func (r *stepReader) unitFactor(e *p21Ent, depth int) float64 {
	if depth > 8 {
		return 1
	}
	if p := e.Part("SI_UNIT"); p != nil {
		f := 1.0
		if pre := arg(p.Args, 0); pre.Kind == p21Enum {
			if k, ok := siPrefix[pre.Str]; ok {
				f = k
			}
		}
		return f
	}
	if p := e.Part("CONVERSION_BASED_UNIT"); p != nil {
		m, err := r.ent(arg(p.Args, 1))
		if err != nil {
			return 1
		}
		ma := m.Parts[0].Args
		for _, part := range m.Parts {
			if len(part.Args) >= 2 {
				ma = part.Args
			}
		}
		v, err := num(arg(ma, 0))
		if err != nil {
			return 1
		}
		u, err := r.ent(arg(ma, 1))
		if err != nil {
			return v
		}
		return v * r.unitFactor(u, depth+1)
	}
	return 1
}

func (r *stepReader) point(v p21Val) (geom.Vec3, error) {
	e, err := r.ent(v)
	if err != nil {
		return geom.Vec3{}, err
	}
	if e.Type() != "CARTESIAN_POINT" {
		return geom.Vec3{}, fmt.Errorf("#%d is a %s, not a point", e.ID, e.Type())
	}
	c := arg(e.Parts[0].Args, 1).List
	var x [3]float64
	for i := 0; i < 3 && i < len(c); i++ {
		x[i] = c[i].Num * r.scale
	}
	return geom.Vec3{X: x[0], Y: x[1], Z: x[2]}, nil
}

func (r *stepReader) direction(v p21Val) (geom.Vec3, error) {
	e, err := r.ent(v)
	if err != nil {
		return geom.Vec3{}, err
	}
	if e.Type() == "VECTOR" {
		return r.direction(arg(e.Parts[0].Args, 1))
	}
	c := arg(e.Parts[0].Args, 1).List
	var x [3]float64
	for i := 0; i < 3 && i < len(c); i++ {
		x[i] = c[i].Num
	}
	d := geom.Vec3{X: x[0], Y: x[1], Z: x[2]}
	if d.Len() == 0 {
		return d, fmt.Errorf("#%d: zero direction", e.ID)
	}
	return d.Norm(), nil
}

// frame reads an AXIS2_PLACEMENT_3D.
func (r *stepReader) frame(v p21Val) (geom.Frame, error) {
	e, err := r.ent(v)
	if err != nil {
		return geom.Frame{}, err
	}
	a := e.Parts[0].Args
	o, err := r.point(arg(a, 1))
	if err != nil {
		return geom.Frame{}, err
	}
	z, x := geom.ZAxis, geom.XAxis
	if arg(a, 2).Kind == p21Ref {
		if z, err = r.direction(a[2]); err != nil {
			return geom.Frame{}, err
		}
	}
	if arg(a, 3).Kind == p21Ref {
		if x, err = r.direction(a[3]); err != nil {
			return geom.Frame{}, err
		}
	} else if math.Abs(z.Dot(x)) > 0.9 {
		x = geom.YAxis
	}
	f := geom.FrameZ(o, z, x)
	return f, nil
}

func (r *stepReader) solid(e *p21Ent, name string) (*brep.Body, error) {
	r.b = brep.NewBuilder(name)
	r.verts, r.edges, r.surfs = map[int]*brep.Vertex{}, map[int]*stepEdge{}, map[int]geom.Surface{}
	a := e.Parts[0].Args
	shells := []p21Val{arg(a, 1)}
	if e.Is("BREP_WITH_VOIDS") {
		shells = append(shells, arg(args(e, "BREP_WITH_VOIDS"), 2).List...)
	}
	n := 0
	for _, sv := range shells {
		if err := r.shell(sv, false, &n); err != nil {
			return nil, err
		}
	}
	r.fixEdges()
	return r.b.Finish()
}

func (r *stepReader) shell(v p21Val, flip bool, n *int) error {
	e, err := r.ent(v)
	if err != nil {
		return err
	}
	switch e.Type() {
	case "ORIENTED_CLOSED_SHELL":
		a := e.Parts[0].Args
		return r.shell(arg(a, 2), flip != (arg(a, 3).Str == "F"), n)
	case "CLOSED_SHELL", "OPEN_SHELL":
		for _, fv := range arg(e.Parts[0].Args, 1).List {
			fe, err := r.ent(fv)
			if err != nil {
				return err
			}
			*n++
			if err := r.face(fe, flip, fmt.Sprintf("imp:%d", *n)); err != nil {
				return fmt.Errorf("face #%d: %w", fe.ID, err)
			}
		}
		return nil
	}
	return fmt.Errorf("#%d: a %s isn't a shell", e.ID, e.Type())
}

func (r *stepReader) face(e *p21Ent, flip bool, name string) error {
	t := e.Type()
	if t == "ORIENTED_FACE" {
		a := e.Parts[0].Args
		fe, err := r.ent(arg(a, 2))
		if err != nil {
			return err
		}
		return r.face(fe, flip != (arg(a, 3).Str == "F"), name)
	}
	if t != "ADVANCED_FACE" && t != "FACE_SURFACE" {
		return fmt.Errorf("a %s isn't a face Forma reads", t)
	}
	a := e.Parts[0].Args
	surf, err := r.surface(arg(a, 2))
	if err != nil {
		return err
	}
	reversed := (arg(a, 3).Str == "F") != flip
	type bound struct {
		uses  []brep.Use
		outer bool
	}
	var bounds []bound
	for _, bv := range arg(a, 1).List {
		be, err := r.ent(bv)
		if err != nil {
			return err
		}
		ba := be.Parts[0].Args
		le, err := r.ent(arg(ba, 1))
		if err != nil {
			return err
		}
		if le.Type() == "VERTEX_LOOP" { // a pole: its vertex is on the loop's edges
			continue
		}
		if le.Type() != "EDGE_LOOP" {
			return fmt.Errorf("a %s bound isn't read yet", le.Type())
		}
		var uses []brep.Use
		for _, ov := range arg(le.Parts[0].Args, 1).List {
			oe, err := r.ent(ov)
			if err != nil {
				return err
			}
			oa := oe.Parts[0].Args
			ec, err := r.ent(arg(oa, 3))
			if err != nil {
				return err
			}
			se, err := r.edge(ec)
			if err != nil {
				return fmt.Errorf("edge #%d: %w", ec.ID, err)
			}
			se.surfaces = append(se.surfaces, surf)
			uses = append(uses, brep.Use{E: se.e, Fwd: arg(oa, 4).Str != "F"})
		}
		if (arg(ba, 2).Str == "F") != flip {
			for i, j := 0, len(uses)-1; i < j; i, j = i+1, j-1 {
				uses[i], uses[j] = uses[j], uses[i]
			}
			for i := range uses {
				uses[i].Fwd = !uses[i].Fwd
			}
		}
		bounds = append(bounds, bound{uses, be.Type() == "FACE_OUTER_BOUND"})
	}
	if len(bounds) == 0 {
		return fmt.Errorf("no edge loops")
	}
	// the outer loop first: the file's, else (on a plane) the largest
	outer := -1
	for i, b := range bounds {
		if b.outer {
			outer = i
		}
	}
	if outer < 0 {
		if p, ok := surf.(geom.Plane); ok && len(bounds) > 1 {
			best := -1.0
			for i, b := range bounds {
				if ar := math.Abs(loopArea(b.uses, p)); ar > best {
					best, outer = ar, i
				}
			}
		} else {
			outer = 0
		}
	}
	loops := [][]brep.Use{bounds[outer].uses}
	for i, b := range bounds {
		if i != outer {
			loops = append(loops, b.uses)
		}
	}
	r.b.Face(surf, reversed, name, loops...)
	return nil
}

// loopArea is a loop's area in a plane's coordinates (its points sampled).
func loopArea(uses []brep.Use, p geom.Plane) float64 {
	var pts []geom.Vec2
	for _, u := range uses {
		e := u.E
		for i := 0; i < 8; i++ {
			s := float64(i) / 8
			if !u.Fwd {
				s = 1 - s
			}
			q := p.F.ToLocal(e.Curve.Eval(e.T0 + (e.T1-e.T0)*s))
			pts = append(pts, geom.Vec2{X: q.X, Y: q.Y})
		}
	}
	a := 0.0
	for i := range pts {
		j := (i + 1) % len(pts)
		a += pts[i].X*pts[j].Y - pts[j].X*pts[i].Y
	}
	return a / 2
}

func (r *stepReader) surface(v p21Val) (geom.Surface, error) {
	if s, ok := r.surfs[v.Ref]; ok {
		return s, nil
	}
	e, err := r.ent(v)
	if err != nil {
		return nil, err
	}
	a := e.Parts[0].Args
	var s geom.Surface
	switch t := e.Type(); t {
	case "PLANE":
		f, err := r.frame(arg(a, 1))
		if err != nil {
			return nil, err
		}
		s = geom.Plane{F: f}
	case "CYLINDRICAL_SURFACE", "CONICAL_SURFACE", "SPHERICAL_SURFACE", "TOROIDAL_SURFACE", "DEGENERATE_TOROIDAL_SURFACE":
		f, err := r.frame(arg(a, 1))
		if err != nil {
			return nil, err
		}
		x1, err := num(arg(a, 2))
		if err != nil {
			return nil, fmt.Errorf("%s #%d: %w", t, e.ID, err)
		}
		x1 *= r.scale
		switch t {
		case "CYLINDRICAL_SURFACE":
			s = geom.Cylinder{F: f, R: x1}
		case "SPHERICAL_SURFACE":
			s = geom.Sphere{F: f, R: x1}
		case "CONICAL_SURFACE":
			ang, err := num(arg(a, 3))
			if err != nil {
				return nil, err
			}
			s = geom.Cone{F: f, R: x1, A: ang * r.angle}
		default:
			minor, err := num(arg(a, 3))
			if err != nil {
				return nil, err
			}
			s = geom.Torus{F: f, R: x1, Rm: minor * r.scale}
		}
	default:
		return nil, fmt.Errorf("its surface is a %s, which Forma can't represent yet", stepTypeName(t))
	}
	r.surfs[v.Ref] = s
	return s, nil
}

func stepTypeName(t string) string {
	switch t {
	case "B_SPLINE_SURFACE_WITH_KNOTS", "B_SPLINE_SURFACE", "BOUNDED_SURFACE":
		return "B-spline surface"
	case "SURFACE_OF_REVOLUTION":
		return "surface of revolution"
	case "SURFACE_OF_LINEAR_EXTRUSION":
		return "extruded surface"
	}
	return t
}

func (r *stepReader) vertex(v p21Val) (*brep.Vertex, error) {
	if x, ok := r.verts[v.Ref]; ok {
		return x, nil
	}
	e, err := r.ent(v)
	if err != nil {
		return nil, err
	}
	p, err := r.point(arg(e.Parts[0].Args, 1))
	if err != nil {
		return nil, err
	}
	x := r.b.Vertex(p)
	r.verts[v.Ref] = x
	return x, nil
}

func (r *stepReader) edge(e *p21Ent) (*stepEdge, error) {
	if se, ok := r.edges[e.ID]; ok {
		return se, nil
	}
	if e.Type() != "EDGE_CURVE" {
		return nil, fmt.Errorf("a %s isn't an edge Forma reads", e.Type())
	}
	a := e.Parts[0].Args
	v0, err := r.vertex(arg(a, 1))
	if err != nil {
		return nil, err
	}
	v1, err := r.vertex(arg(a, 2))
	if err != nil {
		return nil, err
	}
	sense := arg(a, 4).Str != "F"
	ce, err := r.ent(arg(a, 3))
	if err != nil {
		return nil, err
	}
	se, err := r.curve(ce, v0, v1, sense)
	if err != nil {
		return nil, err
	}
	r.edges[e.ID] = se
	return se, nil
}

// curve makes the edge from v0 to v1 along a curve entity (sense: the way
// the curve runs).
func (r *stepReader) curve(c *p21Ent, v0, v1 *brep.Vertex, sense bool) (*stepEdge, error) {
	a := c.Parts[0].Args
	closed := v0 == v1
	switch t := c.Type(); t {
	case "SURFACE_CURVE", "SEAM_CURVE", "INTERSECTION_CURVE":
		c3, err := r.ent(arg(a, 1))
		if err != nil {
			return nil, err
		}
		return r.curve(c3, v0, v1, sense)
	case "TRIMMED_CURVE":
		basis, err := r.ent(arg(a, 1))
		if err != nil {
			return nil, err
		}
		return r.curve(basis, v0, v1, sense == (arg(a, 4).Str != "F"))
	case "LINE":
		p, err := r.point(arg(a, 1))
		if err != nil {
			return nil, err
		}
		d, err := r.direction(arg(a, 2))
		if err != nil {
			return nil, err
		}
		l := geom.Line{P: p, D: d}
		return &stepEdge{e: r.b.Edge(v0, v1, l, l.Project(v0.P), l.Project(v1.P))}, nil
	case "CIRCLE":
		f, err := r.frame(arg(a, 1))
		if err != nil {
			return nil, err
		}
		rad, err := num(arg(a, 2))
		if err != nil {
			return nil, err
		}
		c := geom.Circle{F: f, R: rad * r.scale}
		t0, t1 := c.Project(v0.P), c.Project(v1.P)
		if sense {
			if t1 <= t0+1e-12 && (closed || t1 < t0-1e-12) {
				t1 += 2 * math.Pi
			}
		} else if t1 >= t0-1e-12 && (closed || t1 > t0+1e-12) {
			t1 -= 2 * math.Pi
		}
		return &stepEdge{e: r.b.Edge(v0, v1, c, t0, t1)}, nil
	case "ELLIPSE":
		f, err := r.frame(arg(a, 1))
		if err != nil {
			return nil, err
		}
		ra, err1 := num(arg(a, 2))
		rb, err2 := num(arg(a, 3))
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("ellipse #%d: bad radii", c.ID)
		}
		ra, rb = ra*r.scale, rb*r.scale
		ev := func(t float64) geom.Vec3 {
			return f.O.Add(f.X.Scale(ra * math.Cos(t))).Add(f.Y.Scale(rb * math.Sin(t)))
		}
		par := func(p geom.Vec3) float64 {
			l := f.ToLocal(p)
			return geom.WrapAngle(math.Atan2(l.Y/rb, l.X/ra))
		}
		return r.sampled(ev, 0, 2*math.Pi, true, par(v0.P), par(v1.P), v0, v1, sense)
	case "POLYLINE":
		var pts []geom.Vec3
		for _, pv := range arg(a, 1).List {
			p, err := r.point(pv)
			if err != nil {
				return nil, err
			}
			pts = append(pts, p)
		}
		return r.polyEdge(pts, v0, v1, sense)
	}
	if bs := c.Part("B_SPLINE_CURVE_WITH_KNOTS"); bs != nil {
		sp, err := r.bspline(c)
		if err != nil {
			return nil, err
		}
		lo, hi := sp.knots[sp.deg], sp.knots[len(sp.knots)-sp.deg-1]
		per := sp.eval(lo).Dist(sp.eval(hi)) < 1e-9*(1+sp.eval(lo).Len())
		return r.sampled(sp.eval, lo, hi, per, sp.project(v0.P), sp.project(v1.P), v0, v1, sense)
	}
	return nil, fmt.Errorf("its curve is a %s, which Forma can't read yet", c.Type())
}

// polyEdge is an edge along a polyline of points, from v0 to v1.
func (r *stepReader) polyEdge(pts []geom.Vec3, v0, v1 *brep.Vertex, sense bool) (*stepEdge, error) {
	if !sense {
		for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
			pts[i], pts[j] = pts[j], pts[i]
		}
	}
	if len(pts) < 2 {
		return nil, fmt.Errorf("a polyline of %d points", len(pts))
	}
	pts[0], pts[len(pts)-1] = v0.P, v1.P
	pl := geom.NewPolyline(pts, 1e-6)
	return &stepEdge{e: r.b.Edge(v0, v1, pl, 0, pl.Length()), sampled: true}, nil
}

// sampled makes a polyline edge along ev from parameter ts (at v0) to te
// (at v1), the curve's way when sense; periodic curves wrap round [lo, hi].
func (r *stepReader) sampled(ev func(float64) geom.Vec3, lo, hi float64, periodic bool, ts, te float64,
	v0, v1 *brep.Vertex, sense bool) (*stepEdge, error) {
	span := hi - lo
	if periodic {
		if sense && te <= ts+1e-12 {
			te += span
		}
		if !sense && te >= ts-1e-12 {
			te -= span
		}
	}
	at := func(t float64) geom.Vec3 {
		if periodic {
			t = lo + math.Mod(math.Mod(t-lo, span)+span, span)
		}
		return ev(t)
	}
	// chord-adaptive: split until each midpoint is within tol of its chord
	const tol = 2e-4
	var params []float64
	var split func(a, b float64, depth int)
	split = func(a, b float64, depth int) {
		m := (a + b) / 2
		pa, pb, pm := at(a), at(b), at(m)
		q1, q3 := at((a+m)/2), at((m+b)/2)
		if depth < 4 || depth < 14 && (segDist(pm, pa, pb) > tol || segDist(q1, pa, pm) > tol || segDist(q3, pm, pb) > tol) {
			split(a, m, depth+1)
			split(m, b, depth+1)
			return
		}
		params = append(params, b)
	}
	params = append(params, ts)
	split(ts, te, 0)
	pts := make([]geom.Vec3, len(params))
	for i, t := range params {
		pts[i] = at(t)
	}
	return r.polyEdge(pts, v0, v1, true)
}

// fixEdges pulls every sampled edge's points onto the surfaces of both its
// faces and sets its tolerance from how far they had to go (and how far the
// chords between them stray).
func (r *stepReader) fixEdges() {
	ids := make([]int, 0, len(r.edges))
	for id := range r.edges {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		se := r.edges[id]
		pl, ok := se.e.Curve.(*geom.Polyline)
		if !ok || len(se.surfaces) != 2 {
			continue
		}
		a, b := se.surfaces[0], se.surfaces[1]
		pts := append([]geom.Vec3(nil), pl.Pts...)
		for i := 1; i+1 < len(pts); i++ {
			if p, ok := geom.OntoBoth(a, b, pts[i], 1e-9); ok {
				pts[i] = p
			}
		}
		tol := 1e-6
		for i := 0; i+1 < len(pts); i++ {
			m := pts[i].Lerp(pts[i+1], 0.5)
			for _, s := range []geom.Surface{a, b} {
				tol = math.Max(tol, geom.SurfaceDist(s, m))
				tol = math.Max(tol, geom.SurfaceDist(s, pts[i]))
			}
		}
		npl := geom.NewPolyline(pts, tol)
		se.e.Curve, se.e.T0, se.e.T1 = npl, 0, npl.Length()
	}
}

// stepSpline is a (rational) B-spline curve.
type stepSpline struct {
	deg   int
	cps   []geom.Vec3
	w     []float64 // nil: not rational
	knots []float64 // expanded by multiplicity
}

func (r *stepReader) bspline(c *p21Ent) (*stepSpline, error) {
	// A simple B_SPLINE_CURVE_WITH_KNOTS has (name, degree, points, form,
	// closed, self-intersect, multiplicities, knots, spec); in a complex
	// instance B_SPLINE_CURVE has (degree, points, …) and the knots part only
	// (multiplicities, knots, spec).
	var degv p21Val
	var cpl, mults, knots []p21Val
	if bc := c.Part("B_SPLINE_CURVE"); bc != nil && len(c.Parts) > 1 {
		ka := args(c, "B_SPLINE_CURVE_WITH_KNOTS")
		degv, cpl, mults, knots = arg(bc.Args, 0), arg(bc.Args, 1).List, arg(ka, 0).List, arg(ka, 1).List
	} else {
		ka := args(c, "B_SPLINE_CURVE_WITH_KNOTS")
		degv, cpl, mults, knots = arg(ka, 1), arg(ka, 2).List, arg(ka, 6).List, arg(ka, 7).List
	}
	degf, err := num(degv)
	if err != nil {
		return nil, fmt.Errorf("B-spline #%d: no degree", c.ID)
	}
	sp := &stepSpline{deg: int(degf)}
	for _, pv := range cpl {
		p, err := r.point(pv)
		if err != nil {
			return nil, err
		}
		sp.cps = append(sp.cps, p)
	}
	if len(mults) != len(knots) {
		return nil, fmt.Errorf("B-spline #%d: %d multiplicities for %d knots", c.ID, len(mults), len(knots))
	}
	for i := range knots {
		for k := 0; k < int(mults[i].Num); k++ {
			sp.knots = append(sp.knots, knots[i].Num)
		}
	}
	if rp := c.Part("RATIONAL_B_SPLINE_CURVE"); rp != nil {
		for _, w := range arg(rp.Args, 0).List {
			sp.w = append(sp.w, w.Num)
		}
		if len(sp.w) != len(sp.cps) {
			sp.w = nil
		}
	}
	if len(sp.knots) != len(sp.cps)+sp.deg+1 || sp.deg < 1 {
		return nil, fmt.Errorf("B-spline #%d: %d knots for %d points of degree %d", c.ID, len(sp.knots), len(sp.cps), sp.deg)
	}
	return sp, nil
}

// eval by de Boor's algorithm, in homogeneous coordinates.
func (s *stepSpline) eval(t float64) geom.Vec3 {
	p, n := s.deg, len(s.cps)
	t = math.Max(s.knots[p], math.Min(s.knots[n], t))
	k := p
	for k < n-1 && s.knots[k+1] <= t {
		k++
	}
	type h struct{ x, y, z, w float64 }
	d := make([]h, p+1)
	for j := 0; j <= p; j++ {
		c := s.cps[j+k-p]
		w := 1.0
		if s.w != nil {
			w = s.w[j+k-p]
		}
		d[j] = h{c.X * w, c.Y * w, c.Z * w, w}
	}
	for rr := 1; rr <= p; rr++ {
		for j := p; j >= rr; j-- {
			i := j + k - p
			den := s.knots[i+p-rr+1] - s.knots[i]
			al := 0.0
			if den != 0 {
				al = (t - s.knots[i]) / den
			}
			d[j] = h{(1-al)*d[j-1].x + al*d[j].x, (1-al)*d[j-1].y + al*d[j].y,
				(1-al)*d[j-1].z + al*d[j].z, (1-al)*d[j-1].w + al*d[j].w}
		}
	}
	return geom.Vec3{X: d[p].x / d[p].w, Y: d[p].y / d[p].w, Z: d[p].z / d[p].w}
}

// project finds the parameter of the curve point nearest q (sampled, then
// refined by golden-section search).
func (s *stepSpline) project(q geom.Vec3) float64 {
	lo, hi := s.knots[s.deg], s.knots[len(s.cps)]
	n := 64 * len(s.cps)
	best, bt := math.Inf(1), lo
	for i := 0; i <= n; i++ {
		t := lo + (hi-lo)*float64(i)/float64(n)
		if d := s.eval(t).Dist(q); d < best {
			best, bt = d, t
		}
	}
	h := (hi - lo) / float64(n)
	a, b := math.Max(lo, bt-h), math.Min(hi, bt+h)
	const g = 0.6180339887498949
	for it := 0; it < 60; it++ {
		c, d := b-g*(b-a), a+g*(b-a)
		if s.eval(c).Dist(q) < s.eval(d).Dist(q) {
			b = d
		} else {
			a = c
		}
	}
	return (a + b) / 2
}
