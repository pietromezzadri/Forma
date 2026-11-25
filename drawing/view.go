// Package drawing makes 2D engineering drawings of a design's bodies:
// orthographic and isometric views with their hidden lines, silhouettes of
// curved faces, sections with hatching, centre marks and lines,
// dimensions, notes and a title block on a standard sheet, written out as
// SVG, PDF or DXF. It works on B-rep bodies only (no window, no GL), so
// everything here is tested headlessly.
//
// Sheet coordinates are millimetres from the sheet's bottom-left corner,
// y up. A view's own coordinates are model millimetres along its right
// and up directions.
package drawing

import (
	"math"
	"runtime"
	"sort"
	"sync"

	"forma/brep"
	"forma/geom"
)

// Frame is how a view looks at the model: R and U are the sheet's right
// and up, D the way it looks (into the sheet). R = D × U.
type Frame struct{ R, U, D geom.Vec3 }

// FrameLooking is the frame looking along d with up as near up as it can be.
func FrameLooking(d, up geom.Vec3) Frame {
	d = d.Norm()
	u := up.Sub(d.Scale(up.Dot(d)))
	if u.Len() < 1e-9 {
		u = d.AnyPerp()
	}
	u = u.Norm()
	return Frame{R: d.Cross(u), U: u, D: d}
}

// P is a model point in the view's coordinates.
func (f Frame) P(p geom.Vec3) geom.Vec2 { return geom.V2(p.Dot(f.R), p.Dot(f.U)) }

// Depth is how far into the view p lies (larger is further away).
func (f Frame) Depth(p geom.Vec3) float64 { return p.Dot(f.D) }

// Body is one of the bodies a view shows.
type Body struct {
	Key  int
	Name string
	Body *brep.Body
	Gen  int // changes whenever the body does (for caching)
	// Threads are its screw threads (drawn as ISO 6410 has them)
	Threads []Thread
	// Callout is a hole's callout, by the name of a face of the hole (nil, or
	// no lines: no callout). Lines may hold {depth}, {cbore} and {csink},
	// drawn as the symbols.
	Callout func(face string) []string
}

// Thread is a screw thread, drawn as a thin line where its roots would be
// seen (ISO 6410): an internal thread's at its major diameter, an external
// one's at its minor; side on as two lines along it, end on as three
// quarters of a circle at an end.
type Thread struct {
	A, B     geom.Vec3 // its axis, from one end to the other (external: B the free end)
	R        float64   // the radius of its thin line
	External bool
	Face     string  // an external thread's face (the cylinder it lies in, which can't hide it)
	Limit    float64 // > 0: a wide line across at A, this far either side of the axis (where the thread stops)
}

// Arc is a circle (or a piece of one) in 2D.
type Arc struct {
	C geom.Vec2
	R float64
}

// Curve is a piece of a view's linework: all of it visible or all hidden.
type Curve struct {
	Pts     []geom.Vec2 // in the view's coordinates (model mm)
	P3      []geom.Vec3 // the model points they came from
	Hidden  bool
	Tangent bool // an edge between faces meeting smoothly
	Body    int  // the body's key
	Edge    *brep.Edge
	Face    *brep.Face // a silhouette's face (Edge nil)
	Arc     *Arc       // the piece lies on this circle (an edge seen end on)
	Thread  bool       // a thread's thin line (no edge or face of the model)
}

// Region is a face cut by a section's plane, to be hatched: its loops in
// the view's coordinates.
type Region struct {
	Body  int
	Loops [][]geom.Vec2
}

// Projection is what a view shows of its bodies.
type Projection struct {
	Frame   Frame
	Curves  []Curve
	Regions []Region
	Bodies  []Body // as drawn (a section's: the halves kept)
	Box     [2]geom.Vec2
	Warn    string
}

// Options say what a projection takes in.
type Options struct {
	Hidden   bool // work out hidden lines (else hidden pieces are dropped)
	Tangent  bool // keep tangent edges
	NoThread bool // leave the threads out
}

// Project draws bodies as seen through fr: every edge and every silhouette
// of a curved face, split where they go behind something.
func Project(bodies []Body, fr Frame, opt Options) *Projection {
	pr := &Projection{Frame: fr, Bodies: bodies}
	var all geom.Box
	for _, b := range bodies {
		all.Union(b.Body.Bounds())
	}
	if all.Empty() {
		return pr
	}
	diag := math.Max(all.Diagonal(), 1e-3)
	mo := brep.MeshOptions{ChordTol: math.Max(diag*6e-5, 1e-4), MaxEdge: diag / 10}
	occ := &occluder{fr: fr, eps: 6*mo.ChordTol + diag*2e-6}
	type cand struct {
		pts      []geom.Vec3
		body     int
		edge     *brep.Edge
		face     *brep.Face
		tangent  bool
		arc      *Arc
		own      int32   // a silhouette's face (in the occluder's count)
		far      float64 // how far in front its own face must be to hide it (< 0: it can't)
		thread   bool
		external bool // a screw's thread: none of it drawn hidden
	}
	var cands []cand
	nOwn := int32(0)
	for _, b := range bodies {
		m := brep.Tessellate(b.Body, mo)
		faces := map[int]*brep.Face{}
		own := map[int]int32{}
		ownByName := map[string]int32{}
		for _, f := range b.Body.Faces() {
			faces[f.ID] = f
			nOwn++
			own[f.ID] = nOwn
			ownByName[f.Name] = nOwn
		}
		occ.add(m, own)
		for _, e := range b.Body.Edges() {
			if e.HE[0] == nil || e.HE[1] == nil {
				continue
			}
			fa, fb := e.HE[0].Loop.Face, e.HE[1].Loop.Face
			tan := smooth(e, fa, fb)
			if fa == fb && tan {
				continue // a seam
			}
			if tan && !opt.Tangent {
				continue
			}
			ts := geom.SampleParams(e.Curve, e.T0, e.T1, mo.ChordTol/4, diag/60)
			pts := make([]geom.Vec3, len(ts))
			for i, t := range ts {
				pts[i] = e.Curve.Eval(t)
			}
			c := cand{pts: pts, body: b.Key, edge: e, tangent: tan}
			if ci, ok := e.Curve.(geom.Circle); ok && math.Abs(ci.F.Z.Dot(fr.D)) > 1-1e-9 {
				c.arc = &Arc{C: fr.P(ci.F.O), R: ci.R}
			}
			cands = append(cands, c)
		}
		for _, s := range silhouettes(m, faces, fr, diag) {
			// the silhouette is found on the facets, a hair inside the true
			// outline, where the face's facets nearer the viewer stand far
			// in front of it (the face is seen edge on): a cylinder, cone or
			// sphere can't hide its own outline (a line of sight touching
			// it doesn't meet it again), anything else only from well in
			// front
			far := diag * 0.02
			switch s.face.Surf.(type) {
			case geom.Cylinder, geom.Cone, geom.Sphere:
				far = -1
			}
			cands = append(cands, cand{pts: s.pts, body: b.Key, face: s.face, own: own[s.face.ID], far: far})
		}
		if !opt.NoThread {
			for _, t := range b.Threads {
				for _, tc := range threadLines(t, fr, diag) {
					c := cand{pts: tc.pts, body: b.Key, arc: tc.arc, thread: !tc.limit}
					if t.External {
						c.own, c.far = ownByName[t.Face], -1
					}
					c.external = t.External
					cands = append(cands, c)
				}
			}
		}
	}
	occ.index()

	// each candidate split into visible and hidden runs, on all the cores
	step := diag / 500
	pieces := make([][]Curve, len(cands))
	var wg sync.WaitGroup
	jobs := make(chan int)
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				c := cands[i]
				for _, run := range occ.split(c.pts, step, c.own, c.far) {
					if run.hidden && !opt.Hidden {
						continue
					}
					if run.hidden && c.external {
						continue
					}
					cv := Curve{P3: run.pts, Hidden: run.hidden, Tangent: c.tangent, Body: c.body, Edge: c.edge, Face: c.face, Arc: c.arc, Thread: c.thread}
					cv.Pts = make([]geom.Vec2, len(run.pts))
					for k, p := range run.pts {
						cv.Pts[k] = fr.P(p)
					}
					pieces[i] = append(pieces[i], cv)
				}
			}
		}()
	}
	for i := range cands {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	var out []Curve
	for _, p := range pieces {
		out = append(out, p...)
	}
	pr.Curves = dedupe(out, fr, diag*4e-4)
	pr.Box = boxOf(pr.Curves)
	return pr
}

// smooth: the faces either side of e meet without a crease (at its middle).
func smooth(e *brep.Edge, fa, fb *brep.Face) bool {
	p := e.Curve.Eval((e.T0 + e.T1) / 2)
	na := faceNormal(fa, p)
	nb := faceNormal(fb, p)
	return na.Dot(nb) > 1-2e-4
}

func faceNormal(f *brep.Face, p geom.Vec3) geom.Vec3 {
	u, v := f.Surf.Project(p)
	return f.OutwardNormal(u, v)
}

func boxOf(cs []Curve) [2]geom.Vec2 {
	lo := geom.V2(math.Inf(1), math.Inf(1))
	hi := geom.V2(math.Inf(-1), math.Inf(-1))
	for _, c := range cs {
		for _, p := range c.Pts {
			lo = geom.V2(math.Min(lo.X, p.X), math.Min(lo.Y, p.Y))
			hi = geom.V2(math.Max(hi.X, p.X), math.Max(hi.Y, p.Y))
		}
	}
	if lo.X > hi.X {
		return [2]geom.Vec2{}
	}
	return [2]geom.Vec2{lo, hi}
}

// --- silhouettes ---

type silh struct {
	pts  []geom.Vec3
	face *brep.Face
}

// silhouettes are the lines on curved faces where they turn away from the
// view: the zero line of n·D over each face's triangles (the mesh's normals
// are the surface's own), chained into polylines.
func silhouettes(m *brep.Mesh, faces map[int]*brep.Face, fr Frame, diag float64) []silh {
	byFace := map[int][]int{}
	for t, f := range m.TriFace {
		byFace[f] = append(byFace[f], t)
	}
	var out []silh
	q := diag * 1e-7
	key := func(p geom.Vec3) [3]int64 {
		return [3]int64{int64(math.Round(p.X / q)), int64(math.Round(p.Y / q)), int64(math.Round(p.Z / q))}
	}
	ids := make([]int, 0, len(byFace))
	for id := range byFace {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, fid := range ids {
		f := faces[fid]
		if f == nil {
			continue
		}
		if _, flat := f.Surf.(geom.Plane); flat {
			continue
		}
		// zero crossings on each triangle's edges, keyed by the mesh edge
		type seg struct{ a, b geom.Vec3 }
		var segs []seg
		for _, t := range byFace[fid] {
			tri := m.Tris[t]
			var g [3]float64
			var p [3]geom.Vec3
			flat := true
			for k := 0; k < 3; k++ {
				p[k] = m.Pos[tri[k]]
				g[k] = m.Nrm[tri[k]].Dot(fr.D)
				if math.Abs(g[k]) > 1e-7 {
					flat = false
				}
				if math.Abs(g[k]) < 1e-10 {
					// on the line: one side for all, or a silhouette lying
					// on a seam (0 one side, −2e-16 the other) is lost
					g[k] = 1e-12
				}
			}
			if flat {
				continue // seen edge on all over (a cylinder end on): its edges outline it
			}
			var xs []geom.Vec3
			for k := 0; k < 3; k++ {
				a, b := k, (k+1)%3
				if (g[a] > 0) != (g[b] > 0) {
					s := g[a] / (g[a] - g[b])
					xs = append(xs, p[a].Lerp(p[b], s))
				}
			}
			if len(xs) == 2 && xs[0].Dist(xs[1]) > q {
				segs = append(segs, seg{xs[0], xs[1]})
			}
		}
		if len(segs) == 0 {
			continue
		}
		// chain them
		at := map[[3]int64][]int{}
		for i, s := range segs {
			at[key(s.a)] = append(at[key(s.a)], i)
			at[key(s.b)] = append(at[key(s.b)], i)
		}
		used := make([]bool, len(segs))
		next := func(p geom.Vec3) (int, bool) {
			for _, i := range at[key(p)] {
				if !used[i] {
					return i, true
				}
			}
			return 0, false
		}
		for i := range segs {
			if used[i] {
				continue
			}
			used[i] = true
			line := []geom.Vec3{segs[i].a, segs[i].b}
			for {
				j, ok := next(line[len(line)-1])
				if !ok {
					break
				}
				used[j] = true
				if key(segs[j].a) == key(line[len(line)-1]) {
					line = append(line, segs[j].b)
				} else {
					line = append(line, segs[j].a)
				}
			}
			for {
				j, ok := next(line[0])
				if !ok {
					break
				}
				used[j] = true
				o := segs[j].a
				if key(segs[j].a) == key(line[0]) {
					o = segs[j].b
				}
				line = append([]geom.Vec3{o}, line...)
			}
			out = append(out, silh{pts: line, face: f})
		}
	}
	return out
}

// --- visibility ---

type otri struct {
	a, b, c    geom.Vec2
	za, zb, zc float64
	own        int32 // the face it is part of
}

// occluder answers whether a point is behind the bodies' triangles.
type occluder struct {
	fr   Frame
	eps  float64
	tris []otri
	lo   geom.Vec2
	cell float64
	nx   int
	ny   int
	grid [][]int32
}

func (o *occluder) add(m *brep.Mesh, own map[int]int32) {
	for i, t := range m.Tris {
		a, b, c := m.Pos[t[0]], m.Pos[t[1]], m.Pos[t[2]]
		o.tris = append(o.tris, otri{o.fr.P(a), o.fr.P(b), o.fr.P(c), o.fr.Depth(a), o.fr.Depth(b), o.fr.Depth(c), own[m.TriFace[i]]})
	}
}

func (o *occluder) index() {
	if len(o.tris) == 0 {
		return
	}
	lo := geom.V2(math.Inf(1), math.Inf(1))
	hi := geom.V2(math.Inf(-1), math.Inf(-1))
	for _, t := range o.tris {
		for _, p := range []geom.Vec2{t.a, t.b, t.c} {
			lo = geom.V2(math.Min(lo.X, p.X), math.Min(lo.Y, p.Y))
			hi = geom.V2(math.Max(hi.X, p.X), math.Max(hi.Y, p.Y))
		}
	}
	w, h := math.Max(hi.X-lo.X, 1e-6), math.Max(hi.Y-lo.Y, 1e-6)
	n := math.Sqrt(float64(len(o.tris)) / 2)
	o.cell = math.Max(w, h) / math.Max(n, 1)
	o.nx = int(w/o.cell) + 1
	o.ny = int(h/o.cell) + 1
	for o.nx*o.ny > 4_000_000 {
		o.cell *= 1.5
		o.nx = int(w/o.cell) + 1
		o.ny = int(h/o.cell) + 1
	}
	o.lo = lo
	o.grid = make([][]int32, o.nx*o.ny)
	for i, t := range o.tris {
		x0, y0 := o.cellOf(geom.V2(math.Min(t.a.X, math.Min(t.b.X, t.c.X)), math.Min(t.a.Y, math.Min(t.b.Y, t.c.Y))))
		x1, y1 := o.cellOf(geom.V2(math.Max(t.a.X, math.Max(t.b.X, t.c.X)), math.Max(t.a.Y, math.Max(t.b.Y, t.c.Y))))
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				o.grid[y*o.nx+x] = append(o.grid[y*o.nx+x], int32(i))
			}
		}
	}
}

func (o *occluder) cellOf(p geom.Vec2) (int, int) {
	x := int((p.X - o.lo.X) / o.cell)
	y := int((p.Y - o.lo.Y) / o.cell)
	return max(0, min(o.nx-1, x)), max(0, min(o.ny-1, y))
}

// hidden: something lies in front of p. Face own's triangles only count
// from far in front (far < 0: not at all).
func (o *occluder) hidden(p geom.Vec3, own int32, far float64) bool {
	if o.grid == nil {
		return false
	}
	q := o.fr.P(p)
	z := o.fr.Depth(p) - o.eps
	x, y := o.cellOf(q)
	for _, i := range o.grid[y*o.nx+x] {
		t := &o.tris[i]
		if t.za >= z && t.zb >= z && t.zc >= z {
			continue
		}
		v0, v1, v2 := t.b.Sub(t.a), t.c.Sub(t.a), q.Sub(t.a)
		den := v0.Cross(v1)
		if math.Abs(den) < 1e-18 {
			continue
		}
		s := v2.Cross(v1) / den
		r := v0.Cross(v2) / den
		const in = -1e-9
		if s < in || r < in || s+r > 1-in {
			continue
		}
		zt := t.za + s*(t.zb-t.za) + r*(t.zc-t.za)
		if own != 0 && t.own == own {
			if far < 0 || zt >= z-far {
				continue
			}
		}
		if zt < z {
			return true
		}
	}
	return false
}

type run struct {
	pts    []geom.Vec3
	hidden bool
}

// split resamples a polyline every step and cuts it where it goes behind
// something or comes out (found to a hundredth of a step).
func (o *occluder) split(pts []geom.Vec3, step float64, own int32, far float64) []run {
	if len(pts) < 2 {
		return nil
	}
	// dense points, each with its state; a piece's state is its middle's
	type pt struct {
		p geom.Vec3
		h bool
	}
	var ps []pt
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		n := int(math.Ceil(a.Dist(b) / step))
		if n < 1 {
			n = 1
		}
		for k := 0; k < n; k++ {
			p := a.Lerp(b, float64(k)/float64(n))
			ps = append(ps, pt{p: p})
		}
	}
	ps = append(ps, pt{p: pts[len(pts)-1]})
	// classify the middle of every piece
	mid := make([]bool, len(ps)-1)
	for i := range mid {
		mid[i] = o.hidden(ps[i].p.Lerp(ps[i+1].p, 0.5), own, far)
	}
	// lone flips over a single short piece are noise where two faces meet
	for i := 1; i+1 < len(mid); i++ {
		if mid[i-1] == mid[i+1] && mid[i] != mid[i-1] {
			mid[i] = mid[i-1]
		}
	}
	var out []run
	cur := run{pts: []geom.Vec3{ps[0].p}, hidden: mid[0]}
	for i := 0; i < len(mid); i++ {
		if mid[i] != cur.hidden {
			// the change lies between the middles of pieces i-1 and i
			a := ps[i-1].p.Lerp(ps[i].p, 0.5)
			b := ps[i].p.Lerp(ps[i+1].p, 0.5)
			ha := cur.hidden
			for k := 0; k < 8; k++ {
				m := a.Lerp(b, 0.5)
				if o.hidden(m, own, far) == ha {
					a = m
				} else {
					b = m
				}
			}
			x := a.Lerp(b, 0.5)
			cur.pts = append(cur.pts, x)
			out = append(out, cur)
			cur = run{pts: []geom.Vec3{x}, hidden: mid[i]}
		}
		cur.pts = append(cur.pts, ps[i+1].p)
	}
	out = append(out, cur)
	// drop slivers
	var kept []run
	for _, r := range out {
		if polyLen3(r.pts) > step*0.5 {
			kept = append(kept, r)
		}
	}
	return kept
}

func polyLen3(p []geom.Vec3) float64 {
	l := 0.0
	for i := 0; i+1 < len(p); i++ {
		l += p[i].Dist(p[i+1])
	}
	return l
}

// --- overlaps ---

// dedupe drops linework lying on linework that matters more: visible edges,
// then visible silhouettes, tangent edges, hidden edges, hidden silhouettes.
// (A hole's far rim under its near one, a cylinder's outline on an edge.)
func dedupe(cs []Curve, fr Frame, tol float64) []Curve {
	rank := func(c Curve) int {
		r := 0
		if c.Edge == nil {
			r = 1
		}
		if c.Tangent {
			r = 2
		}
		if c.Hidden {
			r += 3
		}
		return r
	}
	// nearest first among equals: a hole's near rim takes the line, its far
	// one (on the outline of what covers it, so in and out in bits) goes
	depth := make([]float64, len(cs))
	for i, c := range cs {
		for _, p := range c.P3 {
			depth[i] += p.Dot(fr.D)
		}
		depth[i] /= float64(max(1, len(c.P3)))
	}
	ord := make([]int, len(cs))
	for i := range ord {
		ord[i] = i
	}
	sort.SliceStable(ord, func(i, j int) bool {
		ri, rj := rank(cs[ord[i]]), rank(cs[ord[j]])
		if ri != rj {
			return ri < rj
		}
		return depth[ord[i]] < depth[ord[j]]-tol
	})
	sorted := make([]Curve, len(cs))
	for i, k := range ord {
		sorted[i] = cs[k]
	}
	cs = sorted
	idx := &segGrid{cell: math.Max(tol*40, 1e-6), m: map[[2]int][]seg2{}}
	var out []Curve
	for _, c := range cs {
		// split c where its points lie on what is already in
		var cur Curve
		flush := func() {
			if len(cur.Pts) >= 2 && polyLen2(cur.Pts) > tol*2 {
				out = append(out, cur)
			}
			cur = Curve{}
		}
		for i := range c.Pts {
			on := false
			if i+1 < len(c.Pts) {
				on = idx.near(c.Pts[i].Lerp(c.Pts[i+1], 0.5), tol)
			} else if i > 0 {
				on = idx.near(c.Pts[i-1].Lerp(c.Pts[i], 0.5), tol)
			}
			if on {
				if len(cur.Pts) > 0 {
					cur.Pts = append(cur.Pts, c.Pts[i])
					cur.P3 = append(cur.P3, c.P3[i])
				}
				flush()
				continue
			}
			if len(cur.Pts) == 0 {
				cur = c
				cur.Pts = nil
				cur.P3 = nil
			}
			cur.Pts = append(cur.Pts, c.Pts[i])
			cur.P3 = append(cur.P3, c.P3[i])
		}
		flush()
		for i := 0; i+1 < len(c.Pts); i++ {
			idx.add(c.Pts[i], c.Pts[i+1])
		}
	}
	// put the pieces back into a stable order: by body, edge, visibility
	return out
}

func polyLen2(p []geom.Vec2) float64 {
	l := 0.0
	for i := 0; i+1 < len(p); i++ {
		l += p[i].Dist(p[i+1])
	}
	return l
}

type seg2 struct{ a, b geom.Vec2 }

type segGrid struct {
	cell float64
	m    map[[2]int][]seg2
}

func (g *segGrid) key(p geom.Vec2) [2]int {
	return [2]int{int(math.Floor(p.X / g.cell)), int(math.Floor(p.Y / g.cell))}
}

func (g *segGrid) add(a, b geom.Vec2) {
	k0, k1 := g.key(geom.V2(math.Min(a.X, b.X), math.Min(a.Y, b.Y))), g.key(geom.V2(math.Max(a.X, b.X), math.Max(a.Y, b.Y)))
	if (k1[0]-k0[0]+1)*(k1[1]-k0[1]+1) > 400 {
		// a long segment: cut it up
		m := a.Lerp(b, 0.5)
		g.add(a, m)
		g.add(m, b)
		return
	}
	for y := k0[1]; y <= k1[1]; y++ {
		for x := k0[0]; x <= k1[0]; x++ {
			g.m[[2]int{x, y}] = append(g.m[[2]int{x, y}], seg2{a, b})
		}
	}
}

func (g *segGrid) near(p geom.Vec2, tol float64) bool {
	k := g.key(p)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			for _, s := range g.m[[2]int{k[0] + dx, k[1] + dy}] {
				if distSeg(p, s.a, s.b) <= tol {
					return true
				}
			}
		}
	}
	return false
}

func distSeg(p, a, b geom.Vec2) float64 {
	ab := b.Sub(a)
	t := 0.0
	if l2 := ab.Dot(ab); l2 > 0 {
		t = math.Max(0, math.Min(1, p.Sub(a).Dot(ab)/l2))
	}
	return p.Dist(a.Add(ab.Scale(t)))
}

type threadLine struct {
	pts   []geom.Vec3
	arc   *Arc
	limit bool
}

// threadLines are a thread's lines as fr sees it: side on, its two thin
// lines (and the wide line across where an external one stops); end on,
// three quarters of a circle at each end (an external thread's at its
// free end only), open at the upper right.
func threadLines(t Thread, fr Frame, diag float64) []threadLine {
	ax := t.B.Sub(t.A)
	L := ax.Len()
	if L < geom.Tol || t.R <= geom.Tol {
		return nil
	}
	z := ax.Scale(1 / L)
	var out []threadLine
	if math.Abs(z.Dot(fr.D)) < 1-1e-6 {
		w := fr.D.Cross(z).Norm()
		for _, s := range []float64{-1, 1} {
			o := w.Scale(s * t.R)
			out = append(out, threadLine{pts: []geom.Vec3{t.A.Add(o), t.B.Add(o)}})
		}
		if t.Limit > 0 {
			out = append(out, threadLine{pts: []geom.Vec3{t.A.Sub(w.Scale(t.Limit)), t.A.Add(w.Scale(t.Limit))}, limit: true})
		}
		return out
	}
	ends := []geom.Vec3{t.B}
	if !t.External {
		ends = append(ends, t.A)
	}
	n := max(12, int(math.Ceil(t.R*2*math.Pi*0.75/(diag/200))))
	for _, c := range ends {
		pts := make([]geom.Vec3, n+1)
		for i := range pts {
			a := math.Pi/2 + 1.5*math.Pi*float64(i)/float64(n)
			pts[i] = c.Add(fr.R.Scale(t.R * math.Cos(a))).Add(fr.U.Scale(t.R * math.Sin(a)))
		}
		out = append(out, threadLine{pts: pts, arc: &Arc{C: fr.P(c), R: t.R}})
	}
	return out
}
