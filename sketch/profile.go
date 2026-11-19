package sketch

import (
	"math"
	"sort"

	"forma/geom"
)

// Seg is a piece of a region's boundary: a line from A to B, or an arc from
// A to B round C (counter-clockwise when CCW).
type Seg struct {
	Arc  bool
	A, B geom.Vec2
	C    geom.Vec2
	R    float64
	CCW  bool
	Src  int // the entity it came from
}

// Region is a closed area the sketch's curves bound: an outer loop
// (counter-clockwise) and holes (clockwise).
type Region struct {
	Outer []Seg
	Holes [][]Seg
	Area  float64
	// Poly / HolePolys are the loops sampled, for display and tests.
	Poly      []geom.Vec2
	HolePolys [][]geom.Vec2
}

// SegPoints samples a segment (both ends included).
func SegPoints(sg Seg, chordTol float64) []geom.Vec2 {
	if !sg.Arc {
		return []geom.Vec2{sg.A, sg.B}
	}
	a0 := sg.A.Sub(sg.C).Angle()
	a1 := sg.B.Sub(sg.C).Angle()
	var sw float64
	if sg.CCW {
		sw = geom.WrapAngle(a1 - a0)
	} else {
		sw = -geom.WrapAngle(a0 - a1)
	}
	if math.Abs(sw) < 1e-12 {
		sw = 2 * math.Pi
		if !sg.CCW {
			sw = -sw
		}
	}
	n := int(math.Ceil(math.Abs(sw) / geom.ArcStep(math.Max(sg.R, 1e-6), chordTol)))
	if n < 2 {
		n = 2
	}
	out := make([]geom.Vec2, n+1)
	for i := 0; i <= n; i++ {
		a := a0 + sw*float64(i)/float64(n)
		out[i] = sg.C.Add(geom.Vec2{X: math.Cos(a), Y: math.Sin(a)}.Scale(sg.R))
	}
	out[0], out[n] = sg.A, sg.B
	return out
}

func loopPoly(l []Seg, tol float64) []geom.Vec2 {
	var out []geom.Vec2
	for _, s := range l {
		pts := SegPoints(s, tol)
		out = append(out, pts[:len(pts)-1]...)
	}
	return out
}

func polyArea(p []geom.Vec2) float64 {
	a := 0.0
	for i := range p {
		a += p[i].Cross(p[(i+1)%len(p)])
	}
	return a / 2
}

func pointInPoly(q geom.Vec2, p []geom.Vec2) bool {
	in := false
	for i := range p {
		a, b := p[i], p[(i+1)%len(p)]
		if (a.Y > q.Y) != (b.Y > q.Y) {
			if x := a.X + (q.Y-a.Y)*(b.X-a.X)/(b.Y-a.Y); x > q.X {
				in = !in
			}
		}
	}
	return in
}

// a curve of the planar graph
type gcurve struct {
	arc  bool
	a, b geom.Vec2 // line ends
	c    geom.Vec2
	r    float64
	a0   float64 // arc start angle
	sw   float64 // arc sweep (positive)
	src  int
	ts   []float64 // split parameters in [0, 1]
}

func (g *gcurve) at(t float64) geom.Vec2 {
	if !g.arc {
		return g.a.Lerp(g.b, t)
	}
	a := g.a0 + g.sw*t
	return g.c.Add(geom.Vec2{X: math.Cos(a), Y: math.Sin(a)}.Scale(g.r))
}

// param of an arc point (or -1 outside the arc)
func (g *gcurve) arcParam(p geom.Vec2, tol float64) float64 {
	a := geom.WrapAngle(p.Sub(g.c).Angle() - g.a0)
	if a <= g.sw+tol/math.Max(g.r, 1e-9) {
		return math.Min(a/g.sw, 1)
	}
	if 2*math.Pi-a < tol/math.Max(g.r, 1e-9) {
		return 0
	}
	return -1
}

// Regions finds the closed regions bounded by the sketch's non-construction
// curves, splitting them where they cross.
func (s *Sketch) Regions() []Region {
	var cs []*gcurve
	for _, e := range s.Ents {
		if e.Construction {
			continue
		}
		switch e.Kind {
		case Line:
			a, b := s.lineEnds(e.ID)
			if a.Dist(b) > 1e-9 {
				cs = append(cs, &gcurve{a: a, b: b, src: e.ID})
			}
		case Circle:
			r := s.Radius(e)
			if r > 1e-9 {
				cs = append(cs, &gcurve{arc: true, c: s.center(e.ID), r: r, a0: 0, sw: 2 * math.Pi, src: e.ID})
			}
		case Arc:
			a0, sw := s.ArcAngles(e)
			r := s.Radius(e)
			if r > 1e-9 {
				cs = append(cs, &gcurve{arc: true, c: s.center(e.ID), r: r, a0: a0, sw: sw, src: e.ID})
			}
		}
	}
	if len(cs) == 0 {
		return nil
	}
	var box geom.Box
	for _, g := range cs {
		for i := 0; i <= 8; i++ {
			p := g.at(float64(i) / 8)
			box.Add(geom.Vec3{X: p.X, Y: p.Y})
		}
	}
	tol := 1e-7 * (box.Diagonal() + 1)

	for _, g := range cs {
		g.ts = []float64{0, 1}
	}
	for i := 0; i < len(cs); i++ {
		for j := i + 1; j < len(cs); j++ {
			for _, p := range intersect(cs[i], cs[j], tol) {
				cs[i].ts = append(cs[i].ts, p[0])
				cs[j].ts = append(cs[j].ts, p[1])
			}
		}
	}
	// vertices
	var verts []geom.Vec2
	vertex := func(p geom.Vec2) int {
		for i, v := range verts {
			if v.Dist(p) < tol*10 {
				return i
			}
		}
		verts = append(verts, p)
		return len(verts) - 1
	}
	type half struct {
		from, to int
		seg      Seg
		angle    float64 // leaving direction
		curv     float64 // signed curvature leaving (for ties)
		twin     int
		used     bool
	}
	var hs []half
	for _, g := range cs {
		sort.Float64s(g.ts)
		ts := []float64{g.ts[0]}
		for _, t := range g.ts[1:] {
			if g.at(t).Dist(g.at(ts[len(ts)-1])) > tol*10 {
				ts = append(ts, t)
			}
		}
		if ts[len(ts)-1] < 1 && g.at(1).Dist(g.at(ts[len(ts)-1])) <= tol*10 {
			ts[len(ts)-1] = 1
		} else if ts[len(ts)-1] < 1 {
			ts = append(ts, 1)
		}
		// a full circle split fewer than twice gets a second split opposite
		if g.arc && g.sw > 2*math.Pi-1e-9 && len(ts) <= 2 {
			ts = []float64{0, 0.5, 1}
		}
		for k := 0; k+1 < len(ts); k++ {
			t0, t1 := ts[k], ts[k+1]
			pa, pb := g.at(t0), g.at(t1)
			va, vb := vertex(pa), vertex(pb)
			if va == vb && !g.arc {
				continue
			}
			sg := Seg{Arc: g.arc, A: verts[va], B: verts[vb], C: g.c, R: g.r, CCW: true, Src: g.src}
			var dirA, dirB geom.Vec2
			var curv float64
			if g.arc {
				aa := g.a0 + g.sw*t0
				ab := g.a0 + g.sw*t1
				dirA = geom.Vec2{X: -math.Sin(aa), Y: math.Cos(aa)}
				dirB = geom.Vec2{X: math.Sin(ab), Y: -math.Cos(ab)}
				curv = 1 / g.r
			} else {
				dirA = pb.Sub(pa).Norm()
				dirB = pa.Sub(pb).Norm()
			}
			i := len(hs)
			hs = append(hs, half{from: va, to: vb, seg: sg, angle: dirA.Angle(), curv: curv, twin: i + 1})
			rev := sg
			rev.A, rev.B, rev.CCW = sg.B, sg.A, false
			hs = append(hs, half{from: vb, to: va, seg: rev, angle: dirB.Angle(), curv: -curv, twin: i})
		}
	}
	// outgoing half-edges of each vertex, counter-clockwise
	out := make([][]int, len(verts))
	for i, h := range hs {
		out[h.from] = append(out[h.from], i)
	}
	for _, o := range out {
		sort.Slice(o, func(a, b int) bool {
			ha, hb := hs[o[a]], hs[o[b]]
			if math.Abs(ha.angle-hb.angle) > 1e-9 {
				return ha.angle < hb.angle
			}
			return ha.curv < hb.curv
		})
	}
	pos := make([]int, len(hs))
	for _, o := range out {
		for k, i := range o {
			pos[i] = k
		}
	}
	// trace faces: from h, at its end take the edge just clockwise of h's twin
	type cycle struct {
		segs []Seg
		poly []geom.Vec2
		area float64
	}
	var cycles []cycle
	for i := range hs {
		if hs[i].used {
			continue
		}
		var segs []int
		h := i
		for n := 0; n < len(hs)+1; n++ {
			if hs[h].used {
				break
			}
			hs[h].used = true
			segs = append(segs, h)
			t := hs[h].twin
			o := out[hs[h].to]
			k := pos[t]
			h = o[(k-1+len(o))%len(o)]
		}
		if h != i {
			continue // didn't close (shouldn't happen)
		}
		// drop spurs (an edge then straight back along it)
		segs = dropSpurs(segs, func(a int) int { return hs[a].twin })
		if len(segs) == 0 {
			continue
		}
		var c cycle
		for _, k := range segs {
			c.segs = append(c.segs, hs[k].seg)
		}
		c.poly = loopPoly(c.segs, tol*1e4)
		c.area = LoopArea(c.segs)
		cycles = append(cycles, c)
	}
	var regions []Region
	idx := map[int]int{}
	for ci, c := range cycles {
		if c.area > tol*tol*100 {
			idx[ci] = len(regions)
			regions = append(regions, Region{Outer: c.segs, Area: c.area, Poly: c.poly})
		}
	}
	// a negative cycle is a component's outline: a hole of the smallest
	// region containing it
	for _, c := range cycles {
		if c.area >= -tol*tol*100 {
			continue
		}
		best, bestArea := -1, math.Inf(1)
		probe := c.poly[0].Lerp(c.poly[len(c.poly)/2], 1e-3)
		for ri, r := range regions {
			if r.Area > -c.area+tol && r.Area < bestArea && pointInPoly(c.poly[0], r.Poly) && pointInPoly(probe, r.Poly) && !samePoly(c.poly, r.Poly) {
				best, bestArea = ri, r.Area
			}
		}
		if best >= 0 {
			r := &regions[best]
			r.Holes = append(r.Holes, c.segs)
			r.HolePolys = append(r.HolePolys, c.poly)
			r.Area += c.area
		}
	}
	sort.SliceStable(regions, func(a, b int) bool { return regions[a].Area > regions[b].Area })
	return regions
}

// samePoly reports a component's outline tracing the same points as a region
// (a simple loop's two sides).
func samePoly(a, b []geom.Vec2) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[[2]int64]bool{}
	for _, p := range b {
		set[[2]int64{int64(math.Round(p.X * 1e6)), int64(math.Round(p.Y * 1e6))}] = true
	}
	for _, p := range a {
		if !set[[2]int64{int64(math.Round(p.X * 1e6)), int64(math.Round(p.Y * 1e6))}] {
			return false
		}
	}
	return true
}

func dropSpurs(l []int, twin func(int) int) []int {
	changed := true
	for changed && len(l) > 1 {
		changed = false
		for i := 0; i < len(l); i++ {
			j := (i + 1) % len(l)
			if twin(l[i]) == l[j] {
				if j > i {
					l = append(l[:i], l[j+1:]...)
				} else {
					l = l[1 : len(l)-1]
				}
				changed = true
				break
			}
		}
	}
	if len(l) == 2 && twin(l[0]) == l[1] {
		return nil
	}
	return l
}

// intersect returns parameter pairs (on a, on b) where two curves meet.
func intersect(a, b *gcurve, tol float64) [][2]float64 {
	var out [][2]float64
	add := func(p geom.Vec2) {
		ta, tb := a.param(p, tol), b.param(p, tol)
		if ta >= 0 && tb >= 0 {
			out = append(out, [2]float64{ta, tb})
		}
	}
	switch {
	case !a.arc && !b.arc:
		d1, d2 := a.b.Sub(a.a), b.b.Sub(b.a)
		den := d1.Cross(d2)
		if math.Abs(den) < 1e-14*d1.Len()*d2.Len() {
			// parallel: overlapping ends still meet
			for _, p := range []geom.Vec2{b.a, b.b} {
				add(p)
			}
			for _, p := range []geom.Vec2{a.a, a.b} {
				add(p)
			}
			return out
		}
		t := b.a.Sub(a.a).Cross(d2) / den
		add(a.a.Add(d1.Scale(t)))
	case a.arc && b.arc:
		for _, p := range circleCircle(a.c, a.r, b.c, b.r, tol) {
			add(p)
		}
	default:
		l, c := a, b
		if a.arc {
			l, c = b, a
		}
		for _, p := range lineCircle(l.a, l.b, c.c, c.r, tol) {
			add(p)
		}
	}
	return out
}

func (g *gcurve) param(p geom.Vec2, tol float64) float64 {
	if g.arc {
		if math.Abs(p.Dist(g.c)-g.r) > tol*100 {
			return -1
		}
		return g.arcParam(p, tol*10)
	}
	d := g.b.Sub(g.a)
	l2 := d.Dot(d)
	t := p.Sub(g.a).Dot(d) / l2
	if t < -tol/math.Sqrt(l2) || t > 1+tol/math.Sqrt(l2) {
		return -1
	}
	if g.at(t).Dist(p) > tol*100 {
		return -1
	}
	return math.Max(0, math.Min(1, t))
}

func lineCircle(a, b, c geom.Vec2, r, tol float64) []geom.Vec2 {
	d := b.Sub(a)
	f := a.Sub(c)
	A := d.Dot(d)
	B := 2 * f.Dot(d)
	C := f.Dot(f) - r*r
	disc := B*B - 4*A*C
	if disc < -tol*A {
		return nil
	}
	if disc < 0 {
		disc = 0
	}
	sq := math.Sqrt(disc)
	t1 := (-B - sq) / (2 * A)
	t2 := (-B + sq) / (2 * A)
	if sq < 1e-12 {
		return []geom.Vec2{a.Add(d.Scale(t1))}
	}
	return []geom.Vec2{a.Add(d.Scale(t1)), a.Add(d.Scale(t2))}
}

func circleCircle(c1 geom.Vec2, r1 float64, c2 geom.Vec2, r2 float64, tol float64) []geom.Vec2 {
	d := c1.Dist(c2)
	if d < 1e-12 || d > r1+r2+tol || d < math.Abs(r1-r2)-tol {
		return nil
	}
	a := (r1*r1 - r2*r2 + d*d) / (2 * d)
	h2 := r1*r1 - a*a
	if h2 < 0 {
		h2 = 0
	}
	h := math.Sqrt(h2)
	u := c2.Sub(c1).Scale(1 / d)
	m := c1.Add(u.Scale(a))
	if h < 1e-12 {
		return []geom.Vec2{m}
	}
	p := u.Perp().Scale(h)
	return []geom.Vec2{m.Add(p), m.Sub(p)}
}

// LoopArea is a loop's exact signed area (positive counter-clockwise):
// the polygon of its ends plus each arc's circular segment.
func LoopArea(l []Seg) float64 {
	a := 0.0
	for _, s := range l {
		a += s.A.Cross(s.B) / 2
		if s.Arc {
			a0 := s.A.Sub(s.C).Angle()
			a1 := s.B.Sub(s.C).Angle()
			var sw float64
			if s.CCW {
				sw = geom.WrapAngle(a1 - a0)
				if sw < 1e-12 {
					sw = 2 * math.Pi
				}
			} else {
				sw = -geom.WrapAngle(a0 - a1)
				if sw > -1e-12 {
					sw = -2 * math.Pi
				}
			}
			a += s.R * s.R * (sw - math.Sin(sw)) / 2
		}
	}
	return a
}
