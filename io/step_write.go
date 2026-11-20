package io

// STEP AP214 (ISO 10303-21 files, the automotive_design schema) export of
// B-rep bodies as advanced B-rep solids: planes, cylinders, cones, spheres and
// tori bounded by lines, circles and (for intersection curves that are neither)
// degree-1 B-splines through the polyline's points.

import (
	"fmt"
	stdio "io"
	"math"
	"strconv"
	"strings"
	"time"

	"forma/brep"
	"forma/geom"
)

// StepOptions name the file's product.
type StepOptions struct {
	Name   string // product name (the design's)
	Author string
	Time   time.Time // zero: now
}

// WriteSTEP writes bodies as one part (a product whose shape is the bodies'
// solids), in millimetres.
func WriteSTEP(w stdio.Writer, bodies []*brep.Body, opt StepOptions) error {
	if len(bodies) == 0 {
		return fmt.Errorf("nothing to export")
	}
	if opt.Name == "" {
		opt.Name = "Forma part"
	}
	if opt.Time.IsZero() {
		opt.Time = time.Now()
	}
	s := newStepWriter()
	ctx := s.context()
	items := []int{s.axis(geom.Vec3{}, geom.ZAxis, geom.XAxis)}
	for _, b := range bodies {
		ids, err := s.solids(b)
		if err != nil {
			return fmt.Errorf("body %q: %w", b.Name, err)
		}
		items = append(items, ids...)
	}
	rep := s.add("ADVANCED_BREP_SHAPE_REPRESENTATION(%s,%s,#%d)", str(opt.Name), refs(items), ctx)
	s.product(opt.Name, rep)

	return s.write(w, opt)
}

type stepWriter struct {
	lines []string
	pts   map[geom.Vec3]int
	dirs  map[geom.Vec3]int
	verts map[*brep.Vertex]int
	edges map[*brep.Edge]int
	surfs map[geom.Surface]int
	app   int // application context
}

func newStepWriter() *stepWriter {
	return &stepWriter{pts: map[geom.Vec3]int{}, dirs: map[geom.Vec3]int{},
		verts: map[*brep.Vertex]int{}, edges: map[*brep.Edge]int{}, surfs: map[geom.Surface]int{}}
}

// add appends an entity instance and returns its number.
func (s *stepWriter) add(f string, a ...any) int {
	s.lines = append(s.lines, fmt.Sprintf(f, a...))
	return len(s.lines)
}

// context is the geometric context: 3D, mm / radian / steradian, 1e-6 mm.
func (s *stepWriter) context() int {
	mm := s.add("( LENGTH_UNIT() NAMED_UNIT(*) SI_UNIT(.MILLI.,.METRE.) )")
	rad := s.add("( NAMED_UNIT(*) PLANE_ANGLE_UNIT() SI_UNIT($,.RADIAN.) )")
	sr := s.add("( NAMED_UNIT(*) SI_UNIT($,.STERADIAN.) SOLID_ANGLE_UNIT() )")
	unc := s.add("UNCERTAINTY_MEASURE_WITH_UNIT(LENGTH_MEASURE(1.E-06),#%d,'distance_accuracy_value','confusion accuracy')", mm)
	return s.add("( GEOMETRIC_REPRESENTATION_CONTEXT(3) GLOBAL_UNCERTAINTY_ASSIGNED_CONTEXT((#%d)) "+
		"GLOBAL_UNIT_ASSIGNED_CONTEXT((#%d,#%d,#%d)) REPRESENTATION_CONTEXT('Context #1','3D Context with UNIT and UNCERTAINTY') )",
		unc, mm, rad, sr)
}

// product is the part's product structure round its shape representation.
func (s *stepWriter) product(name string, rep int) int {
	if s.app == 0 {
		s.app = s.add("APPLICATION_CONTEXT('core data for automotive mechanical design processes')")
		s.add("APPLICATION_PROTOCOL_DEFINITION('international standard','automotive_design',2000,#%d)", s.app)
	}
	pc := s.add("PRODUCT_CONTEXT('',#%d,'mechanical')", s.app)
	prod := s.add("PRODUCT(%s,%s,'',(#%d))", str(name), str(name), pc)
	s.add("PRODUCT_RELATED_PRODUCT_CATEGORY('part',$,(#%d))", prod)
	form := s.add("PRODUCT_DEFINITION_FORMATION('','',#%d)", prod)
	dc := s.add("PRODUCT_DEFINITION_CONTEXT('part definition',#%d,'design')", s.app)
	def := s.add("PRODUCT_DEFINITION('design','',#%d,#%d)", form, dc)
	pds := s.add("PRODUCT_DEFINITION_SHAPE('','',#%d)", def)
	s.add("SHAPE_DEFINITION_REPRESENTATION(#%d,#%d)", pds, rep)
	return def
}

// solids writes a body's shells: each shell enclosing material is a solid,
// each enclosing a cavity (inside out) a void of the solid round it.
func (s *stepWriter) solids(b *brep.Body) ([]int, error) {
	type shellInfo struct {
		sh   *brep.Shell
		vol  float64
		box  geom.Box
		void []*brep.Shell
	}
	var outer, voids []*shellInfo
	for _, sh := range b.Shells {
		one := &brep.Body{Name: b.Name, Shells: []*brep.Shell{sh}}
		m := brep.Tessellate(one, brep.DefaultMeshOptions(one))
		si := &shellInfo{sh: sh, vol: m.Volume(), box: one.Bounds()}
		if si.vol < 0 {
			voids = append(voids, si)
		} else {
			outer = append(outer, si)
		}
	}
	if len(outer) == 0 {
		return nil, fmt.Errorf("no shell encloses material")
	}
	for _, v := range voids {
		// the smallest solid whose box holds the void's
		var best *shellInfo
		for _, o := range outer {
			if o.box.Contains(v.box.Min) && o.box.Contains(v.box.Max) && (best == nil || o.vol < best.vol) {
				best = o
			}
		}
		if best == nil {
			best = outer[0]
		}
		best.void = append(best.void, v.sh)
	}
	var ids []int
	for i, o := range outer {
		name := b.Name
		if len(outer) > 1 {
			name = fmt.Sprintf("%s.%d", b.Name, i+1)
		}
		shell, err := s.shell(o.sh, false)
		if err != nil {
			return nil, err
		}
		if len(o.void) == 0 {
			ids = append(ids, s.add("MANIFOLD_SOLID_BREP(%s,#%d)", str(name), shell))
			continue
		}
		var vs []int
		for _, v := range o.void {
			vsh, err := s.shell(v, true)
			if err != nil {
				return nil, err
			}
			vs = append(vs, s.add("ORIENTED_CLOSED_SHELL('',*,#%d,.F.)", vsh))
		}
		ids = append(ids, s.add("BREP_WITH_VOIDS(%s,#%d,%s)", str(name), shell, refs(vs)))
	}
	return ids, nil
}

// shell writes a closed shell; flip writes its faces turned round (a void's
// shell, which its oriented shell turns back).
func (s *stepWriter) shell(sh *brep.Shell, flip bool) (int, error) {
	var faces []int
	for _, f := range sh.Faces {
		id, err := s.face(f, flip)
		if err != nil {
			return 0, fmt.Errorf("face %q: %w", f.Name, err)
		}
		faces = append(faces, id)
	}
	return s.add("CLOSED_SHELL('',%s)", refs(faces)), nil
}

func (s *stepWriter) face(f *brep.Face, flip bool) (int, error) {
	surf, err := s.surface(f.Surf)
	if err != nil {
		return 0, err
	}
	var bounds []int
	for i, l := range f.Loops {
		var loop int
		if l.First == nil {
			if l.Lone == nil {
				continue
			}
			loop = s.add("VERTEX_LOOP('',#%d)", s.vertex(l.Lone))
		} else {
			var oes []int
			for _, h := range l.HalfEdges() {
				e, err := s.edge(h.Edge)
				if err != nil {
					return 0, err
				}
				oes = append(oes, s.add("ORIENTED_EDGE('',*,*,#%d,%s)", e, logical(h.Fwd)))
			}
			loop = s.add("EDGE_LOOP('',%s)", refs(oes))
		}
		kind := "FACE_BOUND"
		if i == 0 {
			kind = "FACE_OUTER_BOUND"
		}
		bounds = append(bounds, s.add("%s('',#%d,%s)", kind, loop, logical(!flip)))
	}
	return s.add("ADVANCED_FACE('',%s,#%d,%s)", refs(bounds), surf, logical(f.Reversed == flip)), nil
}

func (s *stepWriter) surface(g geom.Surface) (int, error) {
	if id, ok := s.surfs[g]; ok {
		return id, nil
	}
	var id int
	switch k := g.(type) {
	case geom.Plane:
		id = s.add("PLANE('',#%d)", s.axis(k.F.O, k.F.Z, k.F.X))
	case geom.Cylinder:
		id = s.add("CYLINDRICAL_SURFACE('',#%d,%s)", s.frameAxis(k.F), real(k.R))
	case geom.Cone:
		f, r, a := k.F, k.R, k.A
		if math.Abs(a) < 1e-12 {
			id = s.add("CYLINDRICAL_SURFACE('',#%d,%s)", s.frameAxis(f), real(r))
			break
		}
		if a < 0 { // STEP wants a positive half angle: the axis the other way
			f = geom.Frame{O: f.O, X: f.X, Y: f.Y.Neg(), Z: f.Z.Neg()}
			a = -a
		}
		if r < 0 { // and a radius ≥ 0 at its origin: start at the apex
			f.O = f.O.Add(f.Z.Scale(-r / math.Tan(a)))
			r = 0
		}
		id = s.add("CONICAL_SURFACE('',#%d,%s,%s)", s.frameAxis(f), real(r), real(a))
	case geom.Sphere:
		id = s.add("SPHERICAL_SURFACE('',#%d,%s)", s.frameAxis(k.F), real(k.R))
	case geom.Torus:
		if k.R > k.Rm {
			id = s.add("TOROIDAL_SURFACE('',#%d,%s,%s)", s.frameAxis(k.F), real(k.R), real(k.Rm))
		} else {
			id = s.add("DEGENERATE_TOROIDAL_SURFACE('',#%d,%s,%s,.T.)", s.frameAxis(k.F), real(k.R), real(k.Rm))
		}
	case geom.Revolved:
		// the profile at u = 0, in the world; the axis along F.Z (u turns
		// right-handed about it, as ours does)
		prof := k.C.Map(func(p geom.Vec3) geom.Vec3 { return k.F.O.Add(k.F.X.Scale(p.X)).Add(k.F.Z.Scale(p.Z)) })
		ax := s.add("AXIS1_PLACEMENT('',#%d,#%d)", s.point(k.F.O), s.direction(k.F.Z))
		id = s.add("SURFACE_OF_REVOLUTION('',#%d,#%d)", s.bspline(prof), ax)
	case geom.Extruded:
		vec := s.add("VECTOR('',#%d,1.)", s.direction(k.D))
		id = s.add("SURFACE_OF_LINEAR_EXTRUSION('',#%d,#%d)", s.bspline(k.C), vec)
	default:
		return 0, fmt.Errorf("no STEP form for a %s surface", g.Kind())
	}
	s.surfs[g] = id
	return id, nil
}

// frameAxis is a frame's placement, its axis X × Y so the surface's u runs
// the way STEP's does.
func (s *stepWriter) frameAxis(f geom.Frame) int {
	return s.axis(f.O, f.X.Cross(f.Y), f.X)
}

func (s *stepWriter) vertex(v *brep.Vertex) int {
	if id, ok := s.verts[v]; ok {
		return id
	}
	id := s.add("VERTEX_POINT('',#%d)", s.point(v.P))
	s.verts[v] = id
	return id
}

func (s *stepWriter) edge(e *brep.Edge) (int, error) {
	if id, ok := s.edges[e]; ok {
		return id, nil
	}
	v0, v1 := s.vertex(e.Verts[0]), s.vertex(e.Verts[1])
	var curve int
	sense := true
	switch k := e.Curve.(type) {
	case geom.Line:
		curve = s.line(k.P, k.D)
		sense = e.T1 >= e.T0
	case geom.Circle:
		curve = s.add("CIRCLE('',#%d,%s)", s.axis(k.F.O, k.F.X.Cross(k.F.Y), k.F.X), real(k.R))
		sense = e.T1 >= e.T0
	case geom.Ellipse:
		curve = s.add("ELLIPSE('',#%d,%s,%s)", s.axis(k.F.O, k.F.X.Cross(k.F.Y), k.F.X), real(k.A), real(k.B))
		sense = e.T1 >= e.T0
	case *geom.BSpline:
		curve = s.bspline(k)
		sense = e.T1 >= e.T0
	default:
		curve = s.fitted(e)
	}
	id := s.add("EDGE_CURVE('',#%d,#%d,#%d,%s)", v0, v1, curve, logical(sense))
	s.edges[e] = id
	return id, nil
}

// bspline writes a (non-rational) B-spline curve: its distinct knots with
// their multiplicities.
func (s *stepWriter) bspline(b *geom.BSpline) int {
	var cps []int
	for _, p := range b.Ctrl {
		cps = append(cps, s.point(p))
	}
	var ks, ms []string
	for i := 0; i < len(b.Knots); {
		j := i
		for j < len(b.Knots) && b.Knots[j] == b.Knots[i] {
			j++
		}
		ks = append(ks, real(b.Knots[i]))
		ms = append(ms, fmt.Sprint(j-i))
		i = j
	}
	return s.add("B_SPLINE_CURVE_WITH_KNOTS('',%d,%s,.UNSPECIFIED.,.F.,.F.,(%s),(%s),.UNSPECIFIED.)",
		b.Deg, refs(cps), strings.Join(ms, ","), strings.Join(ks, ","))
}

func (s *stepWriter) line(p, d geom.Vec3) int {
	vec := s.add("VECTOR('',#%d,1.)", s.direction(d))
	return s.add("LINE('',#%d,#%d)", s.point(p), vec)
}

// fitted writes an edge on a polyline (or any other curve) in its own
// direction: a line or circle when its points lie on one, else a degree-1
// B-spline through them.
func (s *stepWriter) fitted(e *brep.Edge) int {
	pts := edgePoints(e)
	a, b := pts[0], pts[len(pts)-1]
	if a.Dist(b) > geom.Tol {
		if d := a.Dist(b); maxOff(pts, func(p geom.Vec3) float64 {
			return segDist(p, a, b)
		}) < fitTol(d) {
			return s.line(a, b.Sub(a))
		}
	}
	if c, ok := fitCircle(pts); ok {
		return s.add("CIRCLE('',#%d,%s)", s.axis(c.F.O, c.F.Z, c.F.X), real(c.R))
	}
	n := len(pts)
	var cps []int
	for _, p := range pts {
		cps = append(cps, s.point(p))
	}
	mult := make([]string, n)
	knots := make([]string, n)
	t := 0.0
	for i := range pts {
		if i > 0 {
			t += pts[i].Dist(pts[i-1])
		}
		mult[i] = "1"
		knots[i] = real(t)
	}
	mult[0], mult[n-1] = "2", "2"
	return s.add("B_SPLINE_CURVE_WITH_KNOTS('',1,%s,.POLYLINE_FORM.,.U.,.F.,(%s),(%s),.UNSPECIFIED.)",
		refs(cps), strings.Join(mult, ","), strings.Join(knots, ","))
}

// edgePoints are an edge's points from its start vertex to its end vertex.
func edgePoints(e *brep.Edge) []geom.Vec3 {
	var ts []float64
	if _, ok := e.Curve.(*geom.Polyline); ok {
		ts = geom.SampleParams(e.Curve, e.T0, e.T1, 0, 0)
	} else {
		ts = geom.SampleParams(e.Curve, e.T0, e.T1, 1e-4, math.Inf(1))
		if len(ts) < 64 {
			ts = make([]float64, 65)
			for i := range ts {
				ts[i] = e.T0 + (e.T1-e.T0)*float64(i)/64
			}
		}
	}
	var pts []geom.Vec3
	for i, t := range ts {
		p := e.Curve.Eval(t)
		switch i {
		case 0:
			p = e.Verts[0].P
		case len(ts) - 1:
			p = e.Verts[1].P
		}
		if len(pts) > 0 && p.Dist(pts[len(pts)-1]) < 1e-9 && i != len(ts)-1 {
			continue
		}
		pts = append(pts, p)
	}
	return pts
}

// fitTol is how near a curve the points must lie to be written as it: the
// boolean pulls its points onto both surfaces to a few 1e-6 mm.
func fitTol(size float64) float64 { return math.Max(1e-5, size*1e-7) }

func maxOff(pts []geom.Vec3, d func(geom.Vec3) float64) float64 {
	m := 0.0
	for _, p := range pts {
		m = math.Max(m, d(p))
	}
	return m
}

func segDist(p, a, b geom.Vec3) float64 {
	ab := b.Sub(a)
	t := math.Max(0, math.Min(1, p.Sub(a).Dot(ab)/ab.Dot(ab)))
	return a.Add(ab.Scale(t)).Dist(p)
}

// fitCircle finds the circle all the points lie on, framed so they run
// counter-clockwise from its X axis at the first point.
func fitCircle(pts []geom.Vec3) (geom.Circle, bool) {
	n := len(pts)
	if n < 4 {
		return geom.Circle{}, false
	}
	a, b, c := pts[0], pts[n/3], pts[2*n/3]
	ab, ac := b.Sub(a), c.Sub(a)
	nrm := ab.Cross(ac)
	if nrm.Len() < 1e-12*ab.Len()*ac.Len() || nrm.Len() == 0 {
		return geom.Circle{}, false
	}
	z := nrm.Norm()
	// least squares in the plane (Kåsa): x² + y² + Dx + Ey + F = 0
	var c0 geom.Vec3
	for _, p := range pts {
		c0 = c0.Add(p)
	}
	c0 = c0.Scale(1 / float64(n))
	u := a.Sub(c0)
	u = u.Sub(z.Scale(u.Dot(z)))
	if u.Len() == 0 {
		return geom.Circle{}, false
	}
	u = u.Norm()
	v := z.Cross(u)
	var m [3][4]float64
	for _, p := range pts {
		q := p.Sub(c0)
		x, y := q.Dot(u), q.Dot(v)
		row := [4]float64{x, y, 1, -(x*x + y*y)}
		for i := 0; i < 3; i++ {
			for j := 0; j < 4; j++ {
				m[i][j] += row[i] * row[j]
			}
		}
	}
	sol, ok := solve3(m)
	if !ok {
		return geom.Circle{}, false
	}
	cx, cy := -sol[0]/2, -sol[1]/2
	r2 := cx*cx + cy*cy - sol[2]
	if r2 <= 0 {
		return geom.Circle{}, false
	}
	o := c0.Add(u.Scale(cx)).Add(v.Scale(cy))
	r := math.Sqrt(r2)
	tol := fitTol(r)
	for _, p := range pts {
		if math.Abs(p.Dist(o)-r) > tol || math.Abs(p.Sub(o).Dot(z)) > tol {
			return geom.Circle{}, false
		}
	}
	// the way the points go round: the first step's turn about z
	if pts[0].Sub(o).Cross(pts[1].Sub(o)).Dot(z) < 0 {
		z = z.Neg()
	}
	x := a.Sub(o).Norm()
	return geom.Circle{F: geom.Frame{O: o, X: x, Y: z.Cross(x), Z: z}, R: r}, true
}

func (s *stepWriter) point(p geom.Vec3) int {
	if id, ok := s.pts[p]; ok {
		return id
	}
	id := s.add("CARTESIAN_POINT('',(%s,%s,%s))", real(p.X), real(p.Y), real(p.Z))
	s.pts[p] = id
	return id
}

func (s *stepWriter) direction(d geom.Vec3) int {
	d = d.Norm()
	if id, ok := s.dirs[d]; ok {
		return id
	}
	id := s.add("DIRECTION('',(%s,%s,%s))", real(d.X), real(d.Y), real(d.Z))
	s.dirs[d] = id
	return id
}

// axis is an AXIS2_PLACEMENT_3D at o with axis z and reference direction x
// (made square to z).
func (s *stepWriter) axis(o, z, x geom.Vec3) int {
	z = z.Norm()
	x = x.Sub(z.Scale(x.Dot(z)))
	if x.Len() < 1e-9 {
		x = geom.FrameZ(o, z, geom.XAxis).X
	}
	return s.add("AXIS2_PLACEMENT_3D('',#%d,#%d,#%d)", s.point(o), s.direction(z), s.direction(x))
}

// real formats a number as a STEP REAL (always with a decimal point).
func real(x float64) string {
	if math.Abs(x) < 1e-15 {
		return "0."
	}
	t := strconv.FormatFloat(x, 'G', 15, 64)
	if i := strings.IndexByte(t, 'E'); i >= 0 {
		if !strings.Contains(t[:i], ".") {
			t = t[:i] + "." + t[i:]
		}
		return t
	}
	if !strings.Contains(t, ".") {
		t += "."
	}
	return t
}

// str quotes a STEP string: apostrophes doubled, anything outside printable
// ASCII as \X2\ hex.
func str(v string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range v {
		switch {
		case r == '\'':
			b.WriteString("''")
		case r == '\\':
			b.WriteString("\\\\")
		case r >= 32 && r < 127:
			b.WriteRune(r)
		case r < 0x10000:
			fmt.Fprintf(&b, "\\X2\\%04X\\X0\\", r)
		default:
			fmt.Fprintf(&b, "\\X4\\%08X\\X0\\", r)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

func logical(v bool) string {
	if v {
		return ".T."
	}
	return ".F."
}

func refs(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = "#" + strconv.Itoa(id)
	}
	return "(" + strings.Join(parts, ",") + ")"
}

// solve3 solves a 3 × 3 system given as an augmented matrix.
func solve3(m [3][4]float64) ([3]float64, bool) {
	for c := 0; c < 3; c++ {
		p := c
		for r := c + 1; r < 3; r++ {
			if math.Abs(m[r][c]) > math.Abs(m[p][c]) {
				p = r
			}
		}
		if math.Abs(m[p][c]) < 1e-300 {
			return [3]float64{}, false
		}
		m[c], m[p] = m[p], m[c]
		for r := 0; r < 3; r++ {
			if r == c {
				continue
			}
			k := m[r][c] / m[c][c]
			for j := c; j < 4; j++ {
				m[r][j] -= k * m[c][j]
			}
		}
	}
	return [3]float64{m[0][3] / m[0][0], m[1][3] / m[1][1], m[2][3] / m[2][2]}, true
}

// EdgeCircle is the circle an edge lies on: its own, or one fitted to a
// polyline's points (a boolean's hole rim).
func EdgeCircle(e *brep.Edge) (geom.Circle, bool) {
	if c, ok := e.Curve.(geom.Circle); ok {
		return c, true
	}
	if _, ok := e.Curve.(*geom.Polyline); ok {
		return fitCircle(edgePoints(e))
	}
	return geom.Circle{}, false
}
