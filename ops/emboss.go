package ops

// Embossing: letters (or any sketch regions) projected along the sketch's
// normal onto a face and raised from it, cut into it, or inlaid: a pocket
// for each letter grown by a clearance, and the letters as bodies of their
// own sitting flush in it.
//
// A letter is its region's prism cut down to a layer that follows the
// face: the face's own surface and that surface moved along its normal,
// over the patch the letter lands on. Every surface here is a plane, a
// surface of revolution (cylinders, cones, spheres, tori, revolved
// profiles) or an extrusion, so the layer is a slab or a revolved strip of
// the face's profile and its offset: exact faces, and booleans the kernel
// already does.

import (
	"fmt"
	"math"

	"forma/brep"
	"forma/geom"
	"forma/sketch"
)

// Emboss modes.
const (
	EmbossRaise = iota // letters stand out from the face
	EmbossCut          // letters cut into it
	EmbossInlay        // pockets cut, letters made to fit them
)

// EmbossSpec says how deep, which way and with what clearance.
type EmbossSpec struct {
	Mode      int
	Depth     float64 // how far letters stand out or go in, along the face's normal
	Clearance float64 // an inlay's gap round its sides and under it
	// Flip lands the letters on the face behind the sketch (against its
	// normal) where the line through a point meets the face both ways
	Flip bool
}

// EmbossFace embosses regions of a sketch (on plane) onto face fc of body
// b. grown are the same letters grown by the clearance (for an inlay's
// pockets; ignored otherwise). It returns the body changed and, for an
// inlay, the letters.
func EmbossFace(b *brep.Body, fc *brep.Face, regs, grown []sketch.Region, plane geom.Frame, sp EmbossSpec) (*brep.Body, []*brep.Body, error) {
	if sp.Depth <= geom.Tol {
		return nil, nil, fmt.Errorf("the depth must be more than 0")
	}
	if len(regs) == 0 {
		return nil, nil, fmt.Errorf("nothing to emboss")
	}
	reach := 2 * math.Max(b.Bounds().Diagonal(), 1)
	// the tool's span along the face's normal, reaching past the surface by
	// over so no face of it lies on the surface
	span := func(over float64) (float64, float64) {
		if sp.Mode == EmbossRaise {
			return -over, sp.Depth
		}
		return -sp.Depth, over
	}
	op := brep.Subtract
	src := regs
	switch sp.Mode {
	case EmbossRaise:
		op = brep.Union
	case EmbossCut:
	case EmbossInlay:
		if sp.Clearance < 0 || sp.Clearance >= sp.Depth {
			return nil, nil, fmt.Errorf("the clearance must be between 0 and the depth")
		}
		if len(grown) > 0 {
			src = grown
		}
	default:
		return nil, nil, fmt.Errorf("unknown emboss mode %d", sp.Mode)
	}
	dir := 1.0
	if sp.Flip {
		dir = -1
	}
	out := b
	// letters far enough apart go in together, one boolean a group; a group
	// that fails goes again letter by letter
	gap := 0.5 + sp.Depth
	var done []bool
	for _, g := range letterGroups(src, gap) {
		if len(g) < 2 {
			continue
		}
		d0, d1 := span(0.3*sp.Depth + 0.2)
		var tools []*brep.Body
		for _, i := range g {
			t, err := letterOn(fc, src[i], plane, d0, d1, reach, dir, fmt.Sprintf("letter%d", i+1))
			if err != nil {
				return nil, nil, fmt.Errorf("letter %d: %v", i+1, err)
			}
			tools = append(tools, t)
		}
		all, err := brep.Gather("letters", tools...)
		if err != nil {
			continue
		}
		res, err := brep.Boolean(out, all, op)
		if err != nil || brep.Check(res) != nil {
			continue
		}
		out = res
		if done == nil {
			done = make([]bool, len(src))
		}
		for _, i := range g {
			done[i] = true
		}
	}
	for i, r := range src {
		if done != nil && done[i] {
			continue
		}
		// a boolean can still trip where a cut happens to fall on the
		// points its curves are traced at; a tool reaching a little further
		// past the surface moves those points
		var err error
		for _, over := range []float64{1, 1.17, 1.41, 1.73} {
			d0, d1 := span(over * (0.3*sp.Depth + 0.2))
			var t, res *brep.Body
			t, err = letterOn(fc, r, plane, d0, d1, reach, dir, fmt.Sprintf("letter%d", i+1))
			if err != nil {
				return nil, nil, fmt.Errorf("letter %d: %v", i+1, err)
			}
			res, err = brep.Boolean(out, t, op)
			if err == nil {
				out = res
				break
			}
		}
		if err != nil {
			return nil, nil, fmt.Errorf("letter %d: %v", i+1, err)
		}
	}
	out.Name = b.Name
	if sp.Mode != EmbossInlay {
		return out, nil, nil
	}
	var inlays []*brep.Body
	for i, r := range regs {
		l, err := letterOn(fc, r, plane, -(sp.Depth - sp.Clearance), 0, reach, dir, fmt.Sprintf("inlay%d", i+1))
		if err != nil {
			return nil, nil, fmt.Errorf("inlay %d: %v", i+1, err)
		}
		inlays = append(inlays, l)
	}
	return out, inlays, nil
}

// letterOn is region r's prism (along plane's normal) within the layer of
// face fc from d0 to d1 along its outward normal.
func letterOn(fc *brep.Face, r sketch.Region, plane geom.Frame, d0, d1, reach, dir float64, name string) (*brep.Body, error) {
	pts := footprint(r)
	hits, err := landOn(fc, pts, plane, reach, dir)
	if err != nil {
		return nil, err
	}
	margin := 3*(d1-d0) + 0.5
	if err := onFace(fc, hits); err != nil {
		return nil, err
	}
	// built directly between the face's surface moved d0 and d1 out
	if s0, err := capSurface(fc, d0, hits, margin); err == nil {
		if s1, err := capSurface(fc, d1, hits, margin); err == nil {
			guess := hits[0].Sub(plane.At2(pts[0])).Dot(plane.Z)
			if t, ok := crossAlong(fc.Surf, plane.At2(r.Outer[0].A), plane.Z, guess, reach); ok {
				l, err := ExtrudeBetween(name, tidyRegion(r, 0.15), plane, s0, s1, t)
				if err == nil && brep.Check(l) == nil {
					nameFaces(l, name)
					return l, nil
				}
			}
		}
	}
	// else as the prism cut down to the layer
	layer, err := faceLayer(fc, hits, d0, d1, margin)
	if err != nil {
		return nil, err
	}
	// the prism reaches through the layer and no further
	bx := layer.Bounds()
	lo, hi := math.Inf(1), math.Inf(-1)
	for i := 0; i < 8; i++ {
		c := geom.Vec3{X: pick(i&1, bx.Min.X, bx.Max.X), Y: pick(i&2, bx.Min.Y, bx.Max.Y), Z: pick(i&4, bx.Min.Z, bx.Max.Z)}
		t := c.Sub(plane.O).Dot(plane.Z)
		lo, hi = math.Min(lo, t), math.Max(hi, t)
	}
	prism, err := Extrude(name, tidyRegion(r, 0.15), plane, lo-1, hi+1)
	if err != nil {
		return nil, err
	}
	l, err := brep.Boolean(prism, layer, brep.Intersect)
	if err == brep.ErrEmpty {
		return nil, fmt.Errorf("it misses the face")
	}
	if err != nil {
		return nil, err
	}
	l.Name = name
	nameFaces(l, name)
	return l, nil
}

// nameFaces puts the letter's name before its faces' (letter2/side:3:1).
func nameFaces(b *brep.Body, name string) {
	for _, f := range b.Faces() {
		f.Name = name + "/" + f.Name
	}
}

// footprint is a few dozen points round a region's outline.
// letterGroups sorts the regions into groups whose boxes stay gap apart
// (first fit, in order), so each group's tools never touch each other.
func letterGroups(rs []sketch.Region, gap float64) [][]int {
	boxes := make([]geom.Box, len(rs))
	for i, r := range rs {
		poly := r.Poly
		if len(poly) == 0 {
			poly = sketch.SegPoints2(r.Outer, 1e-2)
		}
		for _, p := range poly {
			boxes[i].Add(geom.V3(p.X, p.Y, 0))
		}
	}
	var groups [][]int
next:
	for i := range rs {
		for gi, g := range groups {
			clear := true
			for _, j := range g {
				if boxesNear(boxes[i], boxes[j], gap) {
					clear = false
					break
				}
			}
			if clear {
				groups[gi] = append(g, i)
				continue next
			}
		}
		groups = append(groups, []int{i})
	}
	return groups
}

func boxesNear(a, b geom.Box, gap float64) bool {
	return a.Min.X-gap < b.Max.X && b.Min.X-gap < a.Max.X && a.Min.Y-gap < b.Max.Y && b.Min.Y-gap < a.Max.Y
}

func footprint(r sketch.Region) []geom.Vec2 {
	poly := r.Poly
	if len(poly) == 0 {
		poly = sketch.SegPoints2(r.Outer, 1e-2)
	}
	step := len(poly) / 64
	if step < 1 {
		step = 1
	}
	var out []geom.Vec2
	for i := 0; i < len(poly); i += step {
		out = append(out, poly[i])
	}
	return out
}

// landOn projects sketch points along the sketch's normal onto face fc's
// surface: the crossing nearest the sketch plane.
func landOn(fc *brep.Face, pts []geom.Vec2, plane geom.Frame, reach, dir float64) ([]geom.Vec3, error) {
	s := fc.Surf
	f := func(p geom.Vec3, t float64) float64 { return geom.SignedDist(s, p.Add(plane.Z.Scale(t))) }
	root := func(p geom.Vec3, a, b float64) float64 {
		fa := f(p, a)
		for i := 0; i < 80; i++ {
			m := (a + b) / 2
			fm := f(p, m)
			if (fm < 0) == (fa < 0) {
				a, fa = m, fm
			} else {
				b = m
			}
		}
		return (a + b) / 2
	}
	// out from t0 both ways in steps of h, up to reach
	search := func(p geom.Vec3, t0, h, reach float64) (float64, bool) {
		f0 := f(p, t0)
		up, dn := t0, t0
		fu, fd := f0, f0
		for k := 1; float64(k)*h <= reach; k++ {
			nu, nd := t0+float64(k)*h, t0-float64(k)*h
			gu, gd := f(p, nu), f(p, nd)
			if (gu < 0) != (fu < 0) {
				return root(p, up, nu), true
			}
			if (gd < 0) != (fd < 0) {
				return root(p, nd, dn), true
			}
			up, dn, fu, fd = nu, nd, gu, gd
		}
		return 0, false
	}
	// one way only, for the first point: which side of the sketch
	ahead := func(p geom.Vec3, d float64) (float64, bool) {
		h := reach / 4000
		prev, fp := 0.0, f(p, 0)
		for k := 1; float64(k)*h <= reach; k++ {
			t := d * float64(k) * h
			if g := f(p, t); (g < 0) != (fp < 0) {
				return root(p, prev, t), true
			} else {
				prev, fp = t, g
			}
		}
		return 0, false
	}
	var out []geom.Vec3
	have := false
	last := 0.0
	for _, q := range pts {
		p := plane.At2(q)
		t, ok := 0.0, false
		if have {
			t, ok = search(p, last, reach/2000, reach/20)
		}
		if !ok {
			if t, ok = ahead(p, dir); !ok {
				t, ok = ahead(p, -dir)
			}
		}
		if !ok {
			return nil, fmt.Errorf("it doesn't land on the face (seen along the sketch's normal)")
		}
		last, have = t, true
		out = append(out, p.Add(plane.Z.Scale(t)))
	}
	return out, nil
}

// faceLayer is the solid between face fc's surface moved d0 and d1 along
// its outward normal, over the patch round the points (on it) and margin
// more.
func faceLayer(fc *brep.Face, hits []geom.Vec3, d0, d1, margin float64) (*brep.Body, error) {
	sign := 1.0
	if fc.Reversed {
		sign = -1
	}
	if pl, ok := fc.Surf.(geom.Plane); ok {
		fr := pl.F
		if sign < 0 {
			fr = fr.Flip()
		}
		var bx geom.Box
		for _, h := range hits {
			l := fr.ToLocal(h)
			bx.Add(geom.Vec3{X: l.X, Y: l.Y})
		}
		s := sketch.New("layer", fr)
		a, b := geom.V2(bx.Min.X-margin, bx.Min.Y-margin), geom.V2(bx.Max.X+margin, bx.Max.Y+margin)
		p := []int{s.AddPoint(a).ID, s.AddPoint(geom.V2(b.X, a.Y)).ID, s.AddPoint(b).ID, s.AddPoint(geom.V2(a.X, b.Y)).ID}
		for i := range p {
			s.AddLine(p[i], p[(i+1)%4])
		}
		rs := s.Regions()
		if len(rs) != 1 {
			return nil, fmt.Errorf("no layer on the face")
		}
		return Extrude("layer", rs[0], fr, d0, d1)
	}
	ax, ok := revAxisOf(fc.Surf)
	if !ok {
		return nil, fmt.Errorf("can't emboss on a %s face yet", fc.Surf.Kind())
	}
	// the patch in the surface's (u, v): angles unwrapped round the first
	us := make([]float64, len(hits))
	vs := make([]float64, len(hits))
	rmin := math.Inf(1)
	for i, h := range hits {
		u, v := fc.Surf.Project(h)
		if i > 0 {
			u = us[0] + math.Remainder(u-us[0], 2*math.Pi)
			if p := fc.Surf.PeriodV(); p > 0 {
				v = vs[0] + math.Remainder(v-vs[0], p)
			}
		}
		us[i], vs[i] = u, v
		l := ax.ToLocal(h)
		rmin = math.Min(rmin, math.Hypot(l.X, l.Y))
	}
	u0, u1 := minMax(us)
	v0, v1 := minMax(vs)
	_, sv := fc.Surf.Metric()
	du := margin / math.Max(rmin, margin)
	dv := margin / math.Max(sv, 1e-9)
	u0, u1 = u0-du, u1+du
	if u1-u0 > 1.7*math.Pi {
		return nil, fmt.Errorf("the letters go too far round the face")
	}
	// the patch stays on the face
	fv0, fv1 := faceVRange(fc, vs[0])
	if v0 < fv0-1e-6*(1+math.Abs(fv0)) || v1 > fv1+1e-6*(1+math.Abs(fv1)) {
		return nil, fmt.Errorf("the letters run off the face")
	}
	v0, v1 = math.Max(v0-dv, fv0), math.Min(v1+dv, fv1)
	base, err := profileIn(fc.Surf, v0, v1)
	if err != nil {
		return nil, err
	}
	// which side of the profile (left of its travel) is outward
	vm := (v0 + v1) / 2
	n3 := fc.Surf.Normal((u0+u1)/2, vm).Scale(sign)
	radial := ax.Dir(geom.Vec3{X: math.Cos((u0 + u1) / 2), Y: math.Sin((u0 + u1) / 2)})
	n2 := geom.Vec2{X: n3.Dot(radial), Y: n3.Dot(ax.Z)}
	pa, pb := rhAt(fc.Surf, ax, vm-1e-4*(v1-v0)), rhAt(fc.Surf, ax, vm+1e-4*(v1-v0))
	tg := pb.Sub(pa)
	side := 1.0
	if (geom.Vec2{X: -tg.Y, Y: tg.X}).Dot(n2) < 0 {
		side = -1
	}
	at := func(d float64) []sketch.Seg {
		if d == 0 {
			return base
		}
		var out []sketch.Seg
		for _, s := range base {
			out = append(out, sketch.OffsetSeg(s, side*d)...)
		}
		return out
	}
	c0, c1 := at(d0), at(d1)
	if len(c0) == 0 || len(c1) == 0 {
		return nil, fmt.Errorf("the face curves too tightly for that depth")
	}
	mer := geom.Frame{O: ax.O, X: radial, Y: ax.Z}
	cu, su := math.Cos(u0), math.Sin(u0)
	mer.X = ax.Dir(geom.Vec3{X: cu, Y: su})
	mer.Z = mer.X.Cross(mer.Y)
	s := sketch.New("layer", mer)
	var pts []*sketch.Entity
	pt := func(p geom.Vec2) int {
		for _, e := range pts {
			if s.P(e.ID).Dist(p) < 1e-9 {
				return e.ID
			}
		}
		e := s.AddPoint(p)
		pts = append(pts, e)
		return e.ID
	}
	draw := func(c []sketch.Seg) {
		for _, g := range c {
			switch {
			case g.Ctrl != nil:
				ids := make([]int, len(g.Ctrl))
				for i, q := range g.Ctrl {
					if i == 0 || i == len(g.Ctrl)-1 {
						ids[i] = pt(q)
					} else {
						ids[i] = s.AddPoint(q).ID
					}
				}
				s.AddSpline(ids...)
			case g.Arc:
				a, b := g.A, g.B
				if !g.CCW {
					a, b = b, a
				}
				s.AddArc(s.AddPoint(g.C).ID, pt(a), pt(b))
			default:
				s.AddLine(pt(g.A), pt(g.B))
			}
		}
	}
	draw(c0)
	draw(c1)
	s.AddLine(pt(c0[0].A), pt(c1[0].A))
	s.AddLine(pt(c0[len(c0)-1].B), pt(c1[len(c1)-1].B))
	var reg *sketch.Region
	rs := s.Regions()
	for i := range rs {
		if reg == nil || math.Abs(rs[i].Area) > math.Abs(reg.Area) {
			reg = &rs[i]
		}
	}
	if reg == nil {
		return nil, fmt.Errorf("no layer on the face")
	}
	return Revolve("layer", *reg, mer, geom.Vec2{}, geom.Vec2{Y: 1}, u1-u0)
}

func minMax(xs []float64) (float64, float64) {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, x := range xs {
		lo, hi = math.Min(lo, x), math.Max(hi, x)
	}
	return lo, hi
}

// revAxisOf is the frame a surface of revolution turns about (Z its axis,
// X where u = 0).
func revAxisOf(s geom.Surface) (geom.Frame, bool) {
	switch k := s.(type) {
	case geom.Cylinder:
		return k.F, true
	case geom.Cone:
		return k.F, true
	case geom.Sphere:
		return k.F, true
	case geom.Torus:
		return k.F, true
	case geom.Revolved:
		return k.F, true
	}
	return geom.Frame{}, false
}

// rhAt is a surface of revolution's profile point at v: (distance from the
// axis, height along it).
func rhAt(s geom.Surface, ax geom.Frame, v float64) geom.Vec2 {
	l := ax.ToLocal(s.Eval(0, v))
	return geom.Vec2{X: math.Hypot(l.X, l.Y), Y: l.Z}
}

// profileIn is a surface of revolution's profile from v0 to v1 in its
// (r, h) half-plane.
func profileIn(s geom.Surface, v0, v1 float64) ([]sketch.Seg, error) {
	switch k := s.(type) {
	case geom.Cylinder:
		return []sketch.Seg{{A: geom.V2(k.R, v0), B: geom.V2(k.R, v1)}}, nil
	case geom.Cone:
		t := math.Tan(k.A)
		return []sketch.Seg{{A: geom.V2(k.R+v0*t, v0), B: geom.V2(k.R+v1*t, v1)}}, nil
	case geom.Sphere:
		c := geom.Vec2{}
		return []sketch.Seg{{Arc: true, C: c, R: k.R, CCW: true,
			A: geom.V2(k.R*math.Cos(v0), k.R*math.Sin(v0)), B: geom.V2(k.R*math.Cos(v1), k.R*math.Sin(v1))}}, nil
	case geom.Torus:
		c := geom.V2(k.R, 0)
		return []sketch.Seg{{Arc: true, C: c, R: k.Rm, CCW: true,
			A: c.Add(geom.V2(k.Rm*math.Cos(v0), k.Rm*math.Sin(v0))), B: c.Add(geom.V2(k.Rm*math.Cos(v1), k.Rm*math.Sin(v1)))}}, nil
	case geom.Revolved:
		c := k.C
		if len(c.Knots) != 2*(c.Deg+1) {
			return nil, fmt.Errorf("can't emboss on a revolved spline of more than one span yet")
		}
		a, b := c.Range()
		sub := c.SubBezier((v0-a)/(b-a), (v1-a)/(b-a))
		ctrl := make([]geom.Vec2, len(sub.Ctrl))
		for i, q := range sub.Ctrl {
			ctrl[i] = geom.V2(q.X, q.Z)
		}
		if len(ctrl) == 2 {
			return []sketch.Seg{{A: ctrl[0], B: ctrl[1]}}, nil
		}
		return []sketch.Seg{{A: ctrl[0], B: ctrl[len(ctrl)-1], Ctrl: ctrl}}, nil
	}
	return nil, fmt.Errorf("can't emboss on a %s face yet", s.Kind())
}

// faceVRange is the span of v a face covers (v unwrapped round near, for a
// surface periodic in v).
func faceVRange(fc *brep.Face, near float64) (float64, float64) {
	lo, hi := math.Inf(1), math.Inf(-1)
	pv := fc.Surf.PeriodV()
	for _, l := range fc.Loops {
		for _, h := range l.HalfEdges() {
			e := h.Edge
			for i := 0; i <= 16; i++ {
				_, v := fc.Surf.Project(e.Curve.Eval(e.T0 + (e.T1-e.T0)*float64(i)/16))
				if pv > 0 {
					v = near + math.Remainder(v-near, pv)
				}
				lo, hi = math.Min(lo, v), math.Max(hi, v)
			}
		}
	}
	return lo, hi
}

// tidyRegion drops the outline's segments shorter than min (fonts have
// some a few hundredths of a millimetre long, whose walls are slivers),
// the next one starting where the last ended instead.
func tidyRegion(r sketch.Region, min float64) sketch.Region {
	tidy := func(l []sketch.Seg) []sketch.Seg {
		if len(l) < 4 {
			return l
		}
		var out []sketch.Seg
		for _, s := range l {
			if s.A.Dist(s.B) < min && !s.Arc {
				continue
			}
			out = append(out, s)
		}
		if len(out) < 3 {
			return l
		}
		for i := range out {
			prev := out[(i+len(out)-1)%len(out)]
			if out[i].A.Dist(prev.B) > 1e-12 {
				out[i] = startAt(out[i], prev.B)
			}
		}
		return out
	}
	r.Outer = tidy(r.Outer)
	holes := make([][]sketch.Seg, len(r.Holes))
	for i, h := range r.Holes {
		holes[i] = tidy(h)
	}
	r.Holes = holes
	return r
}

// startAt is s begun at p instead (a Bézier's first handle moved with it).
func startAt(s sketch.Seg, p geom.Vec2) sketch.Seg {
	if s.Ctrl != nil {
		c := append([]geom.Vec2(nil), s.Ctrl...)
		c[1] = c[1].Add(p.Sub(c[0]))
		c[0] = p
		s.Ctrl = c
	}
	s.A = p
	return s
}

// onFace checks the points landed on the face's own span (in v, the way
// round a surface of revolution's profile; a plane is taken as it is).
func onFace(fc *brep.Face, hits []geom.Vec3) error {
	if _, ok := revAxisOf(fc.Surf); !ok {
		return nil
	}
	_, v := fc.Surf.Project(hits[0])
	fv0, fv1 := faceVRange(fc, v)
	for _, h := range hits {
		_, w := fc.Surf.Project(h)
		if p := fc.Surf.PeriodV(); p > 0 {
			w = v + math.Remainder(w-v, p)
		}
		if w < fv0-1e-6*(1+math.Abs(fv0)) || w > fv1+1e-6*(1+math.Abs(fv1)) {
			return fmt.Errorf("the letters run off the face")
		}
	}
	return nil
}

// capSurface is face fc's surface moved d along its outward normal: exact
// for planes and the conics, for a revolved profile the offset of the
// stretch of it the points cover (and margin more) when one Bézier fits it.
func capSurface(fc *brep.Face, d float64, hits []geom.Vec3, margin float64) (geom.Surface, error) {
	if d == 0 {
		return fc.Surf, nil
	}
	rv, ok := fc.Surf.(geom.Revolved)
	if !ok {
		return offsetSurface(fc, d)
	}
	ax := rv.F
	vs := make([]float64, len(hits))
	for i, h := range hits {
		_, vs[i] = rv.Project(h)
	}
	v0, v1 := minMax(vs)
	_, sv := rv.Metric()
	dv := margin / math.Max(sv, 1e-9)
	fv0, fv1 := faceVRange(fc, vs[0])
	v0, v1 = math.Max(v0-dv, fv0), math.Min(v1+dv, fv1)
	base, err := profileIn(rv, v0, v1)
	if err != nil {
		return nil, err
	}
	// outward is left of the profile's travel when the face's normal is
	sign := 1.0
	if fc.Reversed {
		sign = -1
	}
	vm := (v0 + v1) / 2
	n3 := rv.Normal(0, vm).Scale(sign)
	n2 := geom.Vec2{X: n3.Dot(ax.X), Y: n3.Dot(ax.Z)}
	pa, pb := rhAt(rv, ax, vm-1e-4*(v1-v0)), rhAt(rv, ax, vm+1e-4*(v1-v0))
	tg := pb.Sub(pa)
	side := 1.0
	if (geom.Vec2{X: -tg.Y, Y: tg.X}).Dot(n2) < 0 {
		side = -1
	}
	off := sketch.OffsetSeg(base[0], side*d)
	if len(off) != 1 {
		return nil, fmt.Errorf("no single offset of that profile")
	}
	s := off[0]
	ctrl := s.Ctrl
	if ctrl == nil {
		ctrl = []geom.Vec2{s.A, s.B}
	}
	pts := make([]geom.Vec3, len(ctrl))
	for i, q := range ctrl {
		pts[i] = geom.Vec3{X: q.X, Z: q.Y}
	}
	return geom.Revolved{F: ax, C: geom.Bezier(pts...)}, nil
}
