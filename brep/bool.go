package brep

import (
	"fmt"
	"math"
	"sort"

	"forma/geom"
)

// BoolOp is a boolean operation.
type BoolOp int

const (
	Union BoolOp = iota
	Subtract
	Intersect
)

func (o BoolOp) String() string { return [...]string{"union", "subtract", "intersect"}[o] }

// Boolean combines two solids (a − b for Subtract).
//
// Faces keep their analytic surfaces. Where two faces' surfaces cross, the
// intersection curve is found by contouring one surface's signed distance
// over the other's parameter grid, its points pulled onto both surfaces, and
// kept as a polyline edge; where an edge of one body lies on a face of the
// other (a boss standing on a plate, a pocket starting at a face) that edge is
// imprinted on the face. Every face is split along what crosses it in its
// own parameter space, each piece is classed as inside, outside or on the
// other body by a winding number just either side of it, and the pieces the
// operation keeps are sewn into a new body. Edges are shared by where they
// came from (a piece of an original edge, a piece of an intersection curve),
// not by matching geometry.
func Boolean(a, b *Body, op BoolOp) (*Body, error) {
	c := newBoolCtx(a, b)
	if err := c.intersect(); err != nil {
		return nil, err
	}
	c.crossCuts()
	regions, err := c.splitFaces()
	if err != nil {
		return nil, err
	}
	c.aliasPieces()
	c.classify(regions)
	return c.sew(regions, op, a.Name)
}

type bctx struct {
	a, b                    *Body
	scale, ctol, tolOn, eps float64
	weldExact, weldLoose    float64
	faces                   []*bface
	samples                 map[*Edge]*edgeSamples
	meshes                  [2]*Mesh
	verts                   []wvert
	vgrid                   map[[3]int64][]int
	splits                  map[*Edge][]float64
	cuts                    []*bcut
	edgeOwner               map[*Edge]int // which body an edge is from
	pcache                  map[any][]*gpiece
	enum                    map[*Edge]int // edges numbered across both bodies
	overlap                 map[*Edge][]*Edge
	alias                   map[pkey]pieceAlias
}

type wvert struct {
	p    geom.Vec3
	prio int
}

// bface is a face being split: its boundary in a metric parameter space.
type bface struct {
	f        *Face
	body     int
	s        geom.Surface
	su, sv   float64
	pu, pv   float64 // periods (metric), 0 if none
	loops    [][]heUV
	segs     []bseg
	lo, hi   geom.Vec2
	box      geom.Box
	mpts     []geom.Vec3
	h        float64
	cuts     []*bcut
	imprints map[*Edge]bool
}

type bseg struct {
	a, b   geom.Vec2
	e      *Edge
	ta, tb float64
}

// bcut is a piece of an intersection curve lying in two faces.
type bcut struct {
	id     int
	pts    []geom.Vec3
	closed bool
	cum    []float64
	splits []float64
	faces  [2]*bface
}

type bevent struct {
	e *Edge
	t float64
	p geom.Vec3
}

func newBoolCtx(a, b *Body) *bctx {
	var box geom.Box
	box.Union(a.Bounds())
	box.Union(b.Bounds())
	c := &bctx{a: a, b: b, scale: math.Max(box.Diagonal(), 1e-3)}
	c.ctol = math.Max(c.scale*5e-5, 1e-5)
	c.tolOn = 1e-7 * math.Max(1, c.scale)
	c.weldExact = 1e-6 * math.Max(1, c.scale)
	c.weldLoose = c.ctol * 3
	c.eps = math.Max(c.ctol*6, 1e-5)
	c.vgrid = map[[3]int64][]int{}
	c.splits = map[*Edge][]float64{}
	c.edgeOwner = map[*Edge]int{}
	c.enum = map[*Edge]int{}
	opt := MeshOptions{ChordTol: c.ctol, MaxEdge: c.scale / 8}
	c.samples = map[*Edge]*edgeSamples{}
	for bi, body := range []*Body{a, b} {
		for _, e := range body.Edges() {
			c.samples[e] = sampleEdge(e, opt)
			c.edgeOwner[e] = bi
			c.enum[e] = len(c.enum) + 1
		}
		// a coarser mesh is enough for face boxes and the winding numbers
		c.meshes[bi] = Tessellate(body, MeshOptions{ChordTol: c.ctol * 2, MaxEdge: c.scale / 8})
	}
	for bi, body := range []*Body{a, b} {
		m := c.meshes[bi]
		fbox := map[int]*geom.Box{}
		fpts := map[int][]geom.Vec3{}
		for ti, t := range m.Tris {
			id := m.TriFace[ti]
			bx := fbox[id]
			if bx == nil {
				bx = &geom.Box{}
				fbox[id] = bx
			}
			for _, k := range t {
				bx.Add(m.Pos[k])
			}
			if len(fpts[id]) < 48 || ti%7 == 0 && len(fpts[id]) < 160 {
				fpts[id] = append(fpts[id], m.Pos[t[0]])
			}
		}
		for _, f := range body.Faces() {
			bf := c.newFace(f, bi, opt)
			if bx := fbox[f.ID]; bx != nil {
				bf.box = *bx
			}
			bf.mpts = fpts[f.ID]
			c.faces = append(c.faces, bf)
		}
	}
	return c
}

func sampleEdge(e *Edge, opt MeshOptions) *edgeSamples {
	ts := geom.SampleParams(e.Curve, e.T0, e.T1, opt.ChordTol, opt.MaxEdge)
	s := &edgeSamples{ts: ts}
	for i, t := range ts {
		p := e.Curve.Eval(t)
		if i == 0 {
			p = e.Verts[0].P
		} else if i == len(ts)-1 {
			p = e.Verts[1].P
		}
		s.pts = append(s.pts, p)
	}
	return s
}

func (c *bctx) newFace(f *Face, body int, opt MeshOptions) *bface {
	s := f.Surf
	su, sv := s.Metric()
	bf := &bface{f: f, body: body, s: s, su: su, sv: sv, pu: s.PeriodU() * su, pv: s.PeriodV() * sv, imprints: map[*Edge]bool{}}
	bf.lo = geom.Vec2{X: math.Inf(1), Y: math.Inf(1)}
	bf.hi = geom.Vec2{X: math.Inf(-1), Y: math.Inf(-1)}
	for _, l := range f.Loops {
		for _, run := range loopRuns(l, s.PeriodU() == 0 && s.PeriodV() == 0) {
			hs := unwrapLoop(run, s, c.samples, s.PeriodU(), s.PeriodV(), f.Reversed)
			for i := range hs {
				for k := range hs[i].uv {
					hs[i].uv[k] = geom.Vec2{X: hs[i].uv[k].X * su, Y: hs[i].uv[k].Y * sv}
					q := hs[i].uv[k]
					bf.lo = geom.Vec2{X: math.Min(bf.lo.X, q.X), Y: math.Min(bf.lo.Y, q.Y)}
					bf.hi = geom.Vec2{X: math.Max(bf.hi.X, q.X), Y: math.Max(bf.hi.Y, q.Y)}
				}
				for k := 0; k+1 < len(hs[i].uv); k++ {
					bf.segs = append(bf.segs, bseg{a: hs[i].uv[k], b: hs[i].uv[k+1], e: hs[i].h.Edge, ta: hs[i].ts[k], tb: hs[i].ts[k+1]})
				}
			}
			bf.loops = append(bf.loops, hs)
		}
	}
	// pole gaps close the polygon without an edge
	for _, l := range bf.loops {
		for i := range l {
			a := l[i].uv[len(l[i].uv)-1]
			b := l[(i+1)%len(l)].uv[0]
			if a.Dist(b) > 1e-9 {
				bf.segs = append(bf.segs, bseg{a: a, b: b})
			}
		}
	}
	bf.h = math.Max((bf.hi.X-bf.lo.X)+(bf.hi.Y-bf.lo.Y), 1e-3) / 24
	if r := minRadius(s); r > 0 {
		bf.h = math.Min(bf.h, r*geom.ArcStep(r, c.ctol))
	}
	return bf
}

func (f *bface) uvRaw(p geom.Vec3) geom.Vec2 {
	u, v := f.s.Project(p)
	return geom.Vec2{X: u * f.su, Y: v * f.sv}
}

func (f *bface) at(q geom.Vec2) geom.Vec3 { return f.s.Eval(q.X/f.su, q.Y/f.sv) }

// inside: even-odd over the face's boundary.
func (f *bface) inside(q geom.Vec2) bool {
	in := false
	for _, s := range f.segs {
		a, b := s.a, s.b
		if (a.Y > q.Y) != (b.Y > q.Y) {
			if x := a.X + (q.Y-a.Y)*(b.X-a.X)/(b.Y-a.Y); x > q.X {
				in = !in
			}
		}
	}
	return in
}

// shifts tries a point's periodic copies; the first inside, else the nearest
// to the face's middle.
func (f *bface) place(q geom.Vec2) (geom.Vec2, bool) {
	ks := []float64{0, 1, -1, 2, -2}
	for _, ku := range ks {
		if ku != 0 && f.pu == 0 {
			continue
		}
		for _, kv := range ks {
			if kv != 0 && f.pv == 0 {
				continue
			}
			c := geom.Vec2{X: q.X + ku*f.pu, Y: q.Y + kv*f.pv}
			if f.inside(c) {
				return c, true
			}
		}
	}
	mid := f.lo.Lerp(f.hi, 0.5)
	if f.pu > 0 {
		q.X += math.Round((mid.X-q.X)/f.pu) * f.pu
	}
	if f.pv > 0 {
		q.Y += math.Round((mid.Y-q.Y)/f.pv) * f.pv
	}
	return q, false
}

func (f *bface) wrapNear(q, ref geom.Vec2) geom.Vec2 {
	if f.pu > 0 {
		q.X += math.Round((ref.X-q.X)/f.pu) * f.pu
	}
	if f.pv > 0 {
		q.Y += math.Round((ref.Y-q.Y)/f.pv) * f.pv
	}
	return q
}

// weld finds or adds a vertex at p (tol: how far another may be).
func (c *bctx) weld(p geom.Vec3, prio int, tol float64) int {
	cell := c.weldLoose
	key := func(q geom.Vec3) [3]int64 {
		return [3]int64{int64(math.Floor(q.X / cell)), int64(math.Floor(q.Y / cell)), int64(math.Floor(q.Z / cell))}
	}
	k := key(p)
	best, bd := -1, math.Inf(1)
	for dx := int64(-1); dx <= 1; dx++ {
		for dy := int64(-1); dy <= 1; dy++ {
			for dz := int64(-1); dz <= 1; dz++ {
				for _, i := range c.vgrid[[3]int64{k[0] + dx, k[1] + dy, k[2] + dz}] {
					t := tol
					if c.verts[i].prio < 2 {
						t = math.Max(tol, c.weldLoose)
					}
					if d := c.verts[i].p.Dist(p); d <= t && d < bd {
						best, bd = i, d
					}
				}
			}
		}
	}
	if best >= 0 {
		if prio > c.verts[best].prio {
			c.verts[best] = wvert{p, prio}
		}
		return best
	}
	c.verts = append(c.verts, wvert{p, prio})
	c.vgrid[k] = append(c.vgrid[k], len(c.verts)-1)
	return len(c.verts) - 1
}

func boxesOverlap(a, b geom.Box, m float64) bool {
	if a.Empty() || b.Empty() {
		return false
	}
	return a.Min.X <= b.Max.X+m && b.Min.X <= a.Max.X+m &&
		a.Min.Y <= b.Max.Y+m && b.Min.Y <= a.Max.Y+m &&
		a.Min.Z <= b.Max.Z+m && b.Min.Z <= a.Max.Z+m
}

// intersect finds every cut and imprint between the two bodies' faces.
func (c *bctx) intersect() error {
	var fa, fb []*bface
	for _, f := range c.faces {
		if f.body == 0 {
			fa = append(fa, f)
		} else {
			fb = append(fb, f)
		}
	}
	c.findOverlaps()
	imprinted := map[[2]any]bool{}
	imprint := func(e *Edge, f *bface) {
		k := [2]any{e, f}
		if imprinted[k] || c.overlapsFace(e, f.f) {
			return
		}
		imprinted[k] = true
		c.imprintEdge(e, f)
	}
	for _, x := range fa {
		for _, y := range fb {
			if !boxesOverlap(x.box, y.box, c.weldLoose) {
				continue
			}
			coinc := geom.Coincident(x.s, y.s, x.mpts, c.tolOn*10) && geom.Coincident(y.s, x.s, y.mpts, c.tolOn*10)
			if !coinc {
				if err := c.ssi(x, y); err != nil {
					return err
				}
			}
			for _, e := range faceEdges(y.f) {
				if c.edgeOn(e, x.s) {
					imprint(e, x)
				}
			}
			for _, e := range faceEdges(x.f) {
				if c.edgeOn(e, y.s) {
					imprint(e, y)
				}
			}
		}
	}
	return nil
}

func faceEdges(f *Face) []*Edge {
	seen := map[*Edge]bool{}
	var out []*Edge
	for _, l := range f.Loops {
		for _, h := range l.HalfEdges() {
			if !seen[h.Edge] {
				seen[h.Edge] = true
				out = append(out, h.Edge)
			}
		}
	}
	return out
}

// edgeOn: every sample of e lies on surface s.
func (c *bctx) edgeOn(e *Edge, s geom.Surface) bool {
	for _, p := range c.samples[e].pts {
		if math.Abs(geom.SignedDist(s, p)) > c.tolOn*100 {
			return false
		}
	}
	return true
}

// ssi finds the intersection curves of two faces.
func (c *bctx) ssi(x, y *bface) error {
	// contour on a plane when there is one (its grid is the most even)
	D, O := x, y
	if _, ok := y.s.(geom.Plane); ok {
		if _, ok2 := x.s.(geom.Plane); !ok2 {
			D, O = y, x
		}
	}
	h := math.Min(D.h, O.h)
	lo, hi := D.lo, D.hi
	if _, ok := D.s.(geom.Plane); ok {
		// only where the other face is
		var bx geom.Box
		for _, p := range []geom.Vec3{O.box.Min, O.box.Max,
			{X: O.box.Min.X, Y: O.box.Min.Y, Z: O.box.Max.Z}, {X: O.box.Min.X, Y: O.box.Max.Y, Z: O.box.Min.Z},
			{X: O.box.Max.X, Y: O.box.Min.Y, Z: O.box.Min.Z}, {X: O.box.Max.X, Y: O.box.Max.Y, Z: O.box.Min.Z},
			{X: O.box.Max.X, Y: O.box.Min.Y, Z: O.box.Max.Z}, {X: O.box.Min.X, Y: O.box.Max.Y, Z: O.box.Max.Z}} {
			q := D.uvRaw(p)
			bx.Add(geom.Vec3{X: q.X, Y: q.Y})
		}
		lo.X, lo.Y = math.Max(lo.X, bx.Min.X), math.Max(lo.Y, bx.Min.Y)
		hi.X, hi.Y = math.Min(hi.X, bx.Max.X), math.Min(hi.Y, bx.Max.Y)
		if lo.X > hi.X+h || lo.Y > hi.Y+h {
			return nil
		}
	}
	lo = lo.Sub(geom.Vec2{X: 2 * h, Y: 2 * h})
	hi = hi.Add(geom.Vec2{X: 2 * h, Y: 2 * h})
	// at most 600 cells a side: widen the cells on big faces
	if w := math.Max(hi.X-lo.X, hi.Y-lo.Y); w/h > 600 {
		h = w / 600
	}
	nx := int(math.Ceil((hi.X - lo.X) / h))
	ny := int(math.Ceil((hi.Y - lo.Y) / h))
	if nx < 2 {
		nx = 2
	}
	if ny < 2 {
		ny = 2
	}
	var lines [][]geom.Vec2
	if planes := splitPlanes(D.s, O.s); planes != nil {
		// two cylinders of one radius whose axes cross meet in two plane
		// curves that cross each other: contour each plane on its own
		for _, pl := range planes {
			pl := pl
			g := func(u, v float64) float64 { return geom.SignedDist(pl, D.at(geom.Vec2{X: u, Y: v})) }
			lines = append(lines, contour(g, lo.X, lo.Y, h, nx, ny)...)
		}
	} else {
		g := func(u, v float64) float64 { return geom.SignedDist(O.s, D.at(geom.Vec2{X: u, Y: v})) }
		lines = contour(g, lo.X, lo.Y, h, nx, ny)
	}
	both := isPlane(D.s) && isPlane(O.s)
	for _, l := range lines {
		closed := len(l) > 2 && l[0].Dist(l[len(l)-1]) < 1e-12
		pts := make([]geom.Vec3, 0, len(l))
		for _, q := range l {
			p, _ := geom.OntoBoth(D.s, O.s, D.at(q), c.tolOn)
			pts = append(pts, p)
		}
		if closed {
			pts[len(pts)-1] = pts[0]
		}
		pts = simplify(pts, closed, c.tolOn, both, c.ctol*0.05)
		if len(pts) < 2 {
			continue
		}
		for _, p1 := range c.clip(D, pts, closed, O.s, nil) {
			for _, p2 := range c.clip(O, p1.pts, p1.closed, D.s, nil) {
				if p2.start == nil && !p2.closed && p2.pts[0].Dist(p1.pts[0]) < 1e-12 {
					p2.start = p1.start
				}
				if p2.end == nil && !p2.closed && p2.pts[len(p2.pts)-1].Dist(p1.pts[len(p1.pts)-1]) < 1e-12 {
					p2.end = p1.end
				}
				if !p2.closed && (p2.start == nil || p2.end == nil) {
					continue // ran off the grid: no real curve
				}
				if len(p2.pts) < 2 {
					continue
				}
				if c.alongEdge(p2.pts, D.f, O.f) {
					continue // an existing edge (imprinted instead)
				}
				if polyLen(p2.pts) < c.weldLoose*2 && !p2.closed {
					continue
				}
				c.addEvent(p2.start)
				c.addEvent(p2.end)
				if !both {
					p2.pts = c.densify(D.s, O.s, p2.pts)
				}
				cut := &bcut{id: len(c.cuts) + 1, pts: p2.pts, closed: p2.closed, faces: [2]*bface{D, O}}
				cut.cum = cumLen(cut.pts)
				c.cuts = append(c.cuts, cut)
				D.cuts = append(D.cuts, cut)
				O.cuts = append(O.cuts, cut)
			}
		}
	}
	return nil
}

func isPlane(s geom.Surface) bool { _, ok := s.(geom.Plane); return ok }

func polyLen(p []geom.Vec3) float64 {
	l := 0.0
	for i := 1; i < len(p); i++ {
		l += p[i].Dist(p[i-1])
	}
	return l
}

func cumLen(p []geom.Vec3) []float64 {
	c := make([]float64, len(p))
	for i := 1; i < len(p); i++ {
		c[i] = c[i-1] + p[i].Dist(p[i-1])
	}
	return c
}

// simplify drops points: on straight curves (two planes) all but the
// corners, else points closer than minStep.
func simplify(pts []geom.Vec3, closed bool, tol float64, straight bool, minStep float64) []geom.Vec3 {
	if straight && !closed {
		out := []geom.Vec3{pts[0]}
		for i := 1; i+1 < len(pts); i++ {
			a, b, p := out[len(out)-1], pts[i+1], pts[i]
			ab := b.Sub(a)
			if l := ab.Len(); l > 0 && ab.Cross(p.Sub(a)).Len()/l > tol*10 {
				out = append(out, p)
			}
		}
		return append(out, pts[len(pts)-1])
	}
	out := []geom.Vec3{pts[0]}
	for i := 1; i < len(pts)-1; i++ {
		if pts[i].Dist(out[len(out)-1]) >= minStep {
			out = append(out, pts[i])
		}
	}
	return append(out, pts[len(pts)-1])
}

// alongEdge: the polyline runs along an edge of either face.
func (c *bctx) alongEdge(pts []geom.Vec3, fs ...*Face) bool {
	probe := []geom.Vec3{pts[len(pts)/2]}
	if len(pts) == 2 {
		probe = []geom.Vec3{pts[0].Lerp(pts[1], 0.5)}
	}
	probe = append(probe, pts[0].Lerp(pts[1], 0.5))
	for _, f := range fs {
		for _, e := range faceEdges(f) {
			ok := true
			for _, q := range probe {
				if e.Curve.Eval(e.Curve.Project(q)).Dist(q) > c.weldLoose {
					ok = false
					break
				}
				t := e.Curve.Project(q)
				lo, hi := math.Min(e.T0, e.T1), math.Max(e.T0, e.T1)
				if _, closed := e.Curve.(geom.Circle); !closed && (t < lo-1e-9 || t > hi+1e-9) {
					ok = false
					break
				}
			}
			if ok {
				return true
			}
		}
	}
	return false
}

func (c *bctx) addEvent(ev *bevent) {
	if ev == nil || ev.e == nil {
		return
	}
	c.weld(ev.p, 2, c.weldExact)
	c.splits[ev.e] = append(c.splits[ev.e], ev.t)
}

type bpiece struct {
	pts        []geom.Vec3
	start, end *bevent
	closed     bool
}

// clip keeps the parts of a polyline inside face F, ending them where they
// cross F's boundary at exact points on its edges. A crossing is refined onto
// other (the surface the polyline also lies on) or, for an imprinted edge
// (along), onto that edge.
func (c *bctx) clip(F *bface, pts []geom.Vec3, closed bool, other geom.Surface, along *Edge) []bpiece {
	n := len(pts)
	uv := make([]geom.Vec2, n)
	uv[0], _ = F.place(F.uvRaw(pts[0]))
	in := F.inside(uv[0])
	var out []bpiece
	var cur *bpiece
	onB := false // started on the boundary: crossings right there are that start
	var touch *geom.Vec3
	// starting on the boundary (an edge from a vertex on it): in if it
	// heads inside, with that point as its start
	if n >= 2 && c.onBoundary(F, pts[0]) {
		// which way it heads: judged at the first point clear of the
		// boundary (the first step can be too short to tell)
		q := F.wrapNear(F.uvRaw(pts[0].Lerp(pts[1], 0.5)), uv[0])
		prev := uv[0]
		for k := 1; k < n; k++ {
			qk := F.wrapNear(F.uvRaw(pts[k]), prev)
			prev = qk
			if !F.nearBoundary(qk, c) {
				q = F.wrapNear(F.uvRaw(pts[k-1].Lerp(pts[k], 0.5)), qk)
				if F.nearBoundary(q, c) {
					q = qk
				}
				break
			}
		}
		in = F.inside(q)
		if in {
			cur = &bpiece{pts: []geom.Vec3{pts[0]}, start: c.boundaryEvent(F, uv[0], pts[0])}
		}
		onB = true
	} else if in {
		cur = &bpiece{pts: []geom.Vec3{pts[0]}}
	}
	for i := 1; i < n; i++ {
		a := uv[i-1]
		b := F.wrapNear(F.uvRaw(pts[i]), a)
		uv[i] = b
		xs := F.crossings(a, b)
		for _, x := range xs {
			if onB && pts[i-1].Lerp(pts[i], x.s).Dist(pts[0]) < c.weldLoose {
				continue
			}
			if touch != nil && pts[i-1].Lerp(pts[i], x.s).Dist(*touch) < c.weldLoose {
				continue
			}
			// at the segment's end, on the boundary: a touch if the curve
			// stays on the same side either way
			if xp := pts[i-1].Lerp(pts[i], x.s); i+1 < n && xp.Dist(pts[i]) < c.weldLoose {
				side := func(p, q geom.Vec3, ref geom.Vec2) bool {
					m := F.wrapNear(F.uvRaw(p.Lerp(q, 0.5)), ref)
					return F.inside(m)
				}
				if side(pts[i-1], pts[i], a) == side(pts[i], pts[i+1], b) {
					p := pts[i]
					touch = &p
					continue
				}
			}
			ev := c.refineCrossing(F, x, pts[i-1].Lerp(pts[i], x.s), other, along)
			if in {
				cur.pts = append(cur.pts, ev.p)
				cur.end = ev
				out = append(out, *cur)
				cur = nil
				in = false
			} else {
				cur = &bpiece{pts: []geom.Vec3{ev.p}, start: ev}
				in = true
			}
		}
		if !in {
			// crossed a seam onto the face's other copy?
			if q, ok := F.place(b); ok && len(xs) > 0 && q.Dist(b) > 1e-9 {
				last := xs[len(xs)-1]
				ev := c.refineCrossing(F, last, pts[i-1].Lerp(pts[i], last.s), other, along)
				cur = &bpiece{pts: []geom.Vec3{ev.p}, start: ev}
				in = true
				uv[i] = q
			}
		}
		if in {
			// no point right on top of the last (a crossing just before it)
			if last := cur.pts[len(cur.pts)-1]; last.Dist(pts[i]) > c.ctol*0.05 {
				cur.pts = append(cur.pts, pts[i])
			}
		}
	}
	if cur != nil {
		// ending on the boundary without a crossing there (rounding)
		if last := cur.pts[len(cur.pts)-1]; cur.end == nil && n >= 2 && c.onBoundary(F, last) {
			q := F.wrapNear(F.uvRaw(last), uv[n-1])
			cur.end = c.boundaryEvent(F, q, last)
		}
		out = append(out, *cur)
	}
	// a curve touching the boundary leaves and comes back at one point:
	// one piece
	for k := 0; k+1 < len(out); k++ {
		a, b := out[k], out[k+1]
		if a.end != nil && b.start != nil && !isSeam(a.end.e) && !isSeam(b.start.e) && a.end.p.Dist(b.start.p) < c.weldLoose {
			m := bpiece{pts: append(append([]geom.Vec3{}, a.pts...), b.pts[1:]...), start: a.start, end: b.end}
			out = append(append(out[:k:k], m), out[k+2:]...)
			k--
		}
	}
	// nor right before a closing crossing
	for k := range out {
		p := out[k].pts
		if n := len(p); n > 2 && p[n-2].Dist(p[n-1]) < c.ctol*0.05 {
			out[k].pts = append(p[:n-2:n-2], p[n-1])
		}
	}
	if closed && len(out) > 0 {
		f, l := out[0], out[len(out)-1]
		switch {
		case len(out) == 1 && f.start == nil && f.end == nil:
			out[0].closed = true
		case f.start == nil && l.end == nil && len(out) > 1:
			// the run through the closing point: join its two halves
			merged := bpiece{pts: append(append([]geom.Vec3{}, l.pts...), f.pts[1:]...), start: l.start, end: f.end}
			out = append([]bpiece{merged}, out[1:len(out)-1]...)
		}
	}
	return out
}

type crossing struct {
	s   float64 // along the polyline segment
	seg bseg
	r   float64 // along the boundary segment
}

// crossings of segment a→b with the face boundary, in order along it.
func (f *bface) crossings(a, b geom.Vec2) []crossing {
	var xs []crossing
	d := b.Sub(a)
	for _, s := range f.segs {
		e := s.b.Sub(s.a)
		den := d.Cross(e)
		if math.Abs(den) < 1e-18 {
			continue
		}
		w := s.a.Sub(a)
		t := w.Cross(e) / den
		r := w.Cross(d) / den
		if t > 0 && t <= 1 && r >= 0 && r < 1 {
			xs = append(xs, crossing{s: t, seg: s, r: r})
		}
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i].s < xs[j].s })
	return xs
}

// refineCrossing puts a boundary crossing exactly on the face's edge.
func (c *bctx) refineCrossing(F *bface, x crossing, guess geom.Vec3, other geom.Surface, along *Edge) *bevent {
	e := x.seg.e
	if e == nil { // a pole gap: no edge
		return &bevent{p: guess}
	}
	ta, tb := x.seg.ta, x.seg.tb
	t := ta + (tb-ta)*x.r
	if along != nil {
		// two curves: alternate projections
		p := e.Curve.Eval(t)
		for it := 0; it < 20; it++ {
			q := along.Curve.Eval(along.Curve.Project(p))
			t = clampT(e, e.Curve.Project(q))
			p = e.Curve.Eval(t)
		}
		return &bevent{e: e, t: t, p: p}
	}
	g := func(t float64) float64 { return geom.SignedDist(other, e.Curve.Eval(t)) }
	// scan round the estimate for the sign change nearest it (near a tangency
	// both roots can lie in one sample interval)
	w := math.Abs(tb - ta)
	const n = 48
	lo0, hi0 := clampT(e, t-w), clampT(e, t+w)
	lo, hi := lo0, hi0
	best := math.Inf(1)
	prevT, prevG := lo0, g(lo0)
	for k := 1; k <= n; k++ {
		tk := lo0 + (hi0-lo0)*float64(k)/n
		gk := g(tk)
		if prevG == 0 || gk == 0 || prevG*gk < 0 {
			mid := (prevT + tk) / 2
			if d := math.Abs(mid - t); d < best {
				best, lo, hi = d, prevT, tk
			}
		}
		prevT, prevG = tk, gk
	}
	ga, gb := g(lo), g(hi)
	switch {
	case ga == 0:
		t = lo
	case gb == 0:
		t = hi
	case ga*gb < 0:
		for it := 0; it < 80 && math.Abs(hi-lo) > 1e-14*(1+math.Abs(t)); it++ {
			m := (lo + hi) / 2
			gm := g(m)
			if (gm <= 0) == (ga <= 0) {
				lo, ga = m, gm
			} else {
				hi = m
			}
		}
		t = (lo + hi) / 2
	}
	return &bevent{e: e, t: t, p: e.Curve.Eval(t)}
}

func clampT(e *Edge, t float64) float64 {
	lo, hi := math.Min(e.T0, e.T1), math.Max(e.T0, e.T1)
	return math.Max(lo, math.Min(hi, t))
}

// imprintEdge marks an edge of one body lying on face F of the other, and
// splits it (and F's edges) where it crosses F's boundary.
func (c *bctx) imprintEdge(e *Edge, F *bface) {
	pts := c.samples[e].pts
	closed := e.Verts[0] == e.Verts[1]
	ps := c.clip(F, pts, closed, nil, e)
	if len(ps) == 0 {
		return
	}
	F.imprints[e] = true
	for _, p := range ps {
		for _, ev := range []*bevent{p.start, p.end} {
			if ev == nil || ev.e == nil {
				continue
			}
			c.addEvent(ev)
			// the same point on the imprinted edge
			t := clampT(e, e.Curve.Project(ev.p))
			c.splits[e] = append(c.splits[e], t)
		}
	}
}

// crossCuts splits cuts (and imprinted edges) where they cross on a face.
func (c *bctx) crossCuts() {
	type item struct {
		cut *bcut
		e   *Edge
		pts []geom.Vec3
		uv  []geom.Vec2
		par []float64
	}
	for _, F := range c.faces {
		var items []item
		mapUV := func(pts []geom.Vec3) []geom.Vec2 {
			uv := make([]geom.Vec2, len(pts))
			mid := len(pts) / 2
			uv[mid], _ = F.place(F.uvRaw(pts[mid]))
			for i := mid + 1; i < len(pts); i++ {
				uv[i] = F.wrapNear(F.uvRaw(pts[i]), uv[i-1])
			}
			for i := mid - 1; i >= 0; i-- {
				uv[i] = F.wrapNear(F.uvRaw(pts[i]), uv[i+1])
			}
			return uv
		}
		for _, k := range F.cuts {
			items = append(items, item{cut: k, pts: k.pts, uv: mapUV(k.pts), par: k.cum})
		}
		for e := range F.imprints {
			s := c.samples[e]
			items = append(items, item{e: e, pts: s.pts, uv: mapUV(s.pts), par: s.ts})
		}
		sort.SliceStable(items, func(i, j int) bool {
			ki, kj := 0, 0
			if items[i].cut != nil {
				ki = items[i].cut.id
			} else {
				ki = 1<<30 + items[i].e.ID
			}
			if items[j].cut != nil {
				kj = items[j].cut.id
			} else {
				kj = 1<<30 + items[j].e.ID
			}
			return ki < kj
		})
		addSplit := func(it item, i int, s float64, p geom.Vec3) {
			c.weld(p, 1, c.weldLoose)
			t := it.par[i] + (it.par[i+1]-it.par[i])*s
			if it.cut != nil {
				it.cut.splits = append(it.cut.splits, t)
			} else {
				c.splits[it.e] = append(c.splits[it.e], clampT(it.e, it.e.Curve.Project(p)))
			}
		}
		for i := 0; i < len(items); i++ {
			for j := i + 1; j < len(items); j++ {
				a, b := items[i], items[j]
				if a.e != nil && b.e != nil {
					continue // edges of one body meet at vertices only
				}
				for p := 0; p+1 < len(a.uv); p++ {
					for q := 0; q+1 < len(b.uv); q++ {
						s, r, ok := segX(a.uv[p], a.uv[p+1], b.uv[q], b.uv[q+1])
						if !ok {
							continue
						}
						pa := a.pts[p].Lerp(a.pts[p+1], s)
						pb := b.pts[q].Lerp(b.pts[q+1], r)
						pt := pa.Lerp(pb, 0.5)
						// shared end points are vertices already
						if isEnd(a.pts, pt, c.weldLoose) && isEnd(b.pts, pt, c.weldLoose) {
							continue
						}
						addSplit(a, p, s, pt)
						addSplit(b, q, r, pt)
					}
				}
			}
		}
	}
}

func isEnd(pts []geom.Vec3, p geom.Vec3, tol float64) bool {
	return pts[0].Dist(p) < tol || pts[len(pts)-1].Dist(p) < tol
}

// segX intersects segments p0p1 and q0q1 (ends included).
func segX(p0, p1, q0, q1 geom.Vec2) (float64, float64, bool) {
	d, e := p1.Sub(p0), q1.Sub(q0)
	den := d.Cross(e)
	if math.Abs(den) < 1e-18 {
		return 0, 0, false
	}
	w := q0.Sub(p0)
	s := w.Cross(e) / den
	r := w.Cross(d) / den
	const k = 1e-9
	if s < -k || s > 1+k || r < -k || r > 1+k {
		return 0, 0, false
	}
	return math.Max(0, math.Min(1, s)), math.Max(0, math.Min(1, r)), true
}

// ErrEmpty: nothing is left (an intersection of things that don't meet, a
// cut that takes all).
var ErrEmpty = fmt.Errorf("boolean: the result is empty")

// densify adds points (pulled onto both surfaces) until every segment's
// middle is within the tolerance of both.
func (c *bctx) densify(a, b geom.Surface, pts []geom.Vec3) []geom.Vec3 {
	dev := func(p, q geom.Vec3) float64 {
		m := p.Lerp(q, 0.5)
		return math.Max(math.Abs(geom.SignedDist(a, m)), math.Abs(geom.SignedDist(b, m)))
	}
	var rec func(p, q geom.Vec3, depth int, out []geom.Vec3) []geom.Vec3
	rec = func(p, q geom.Vec3, depth int, out []geom.Vec3) []geom.Vec3 {
		if depth >= 6 || dev(p, q) <= c.ctol*0.5 {
			return append(out, q)
		}
		m, ok := geom.OntoBoth(a, b, p.Lerp(q, 0.5), c.tolOn)
		if !ok {
			return append(out, q)
		}
		out = rec(p, m, depth+1, out)
		return rec(m, q, depth+1, out)
	}
	out := []geom.Vec3{pts[0]}
	for i := 1; i < len(pts); i++ {
		out = rec(pts[i-1], pts[i], 0, out)
	}
	return out
}

// curveTol is how far a cut's segments stray from its two surfaces.
func curveTol(a, b geom.Surface, pts []geom.Vec3) float64 {
	t := 0.0
	for i := 1; i < len(pts); i++ {
		m := pts[i-1].Lerp(pts[i], 0.5)
		t = math.Max(t, math.Max(math.Abs(geom.SignedDist(a, m)), math.Abs(geom.SignedDist(b, m))))
	}
	return t
}
