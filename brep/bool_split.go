package brep

import (
	"fmt"
	"math"
	"sort"

	"forma/geom"
)

// pkey names a piece of edge in the result: kind 0 a piece of an original
// edge, 1 a piece of an intersection curve.
type pkey struct{ kind, id, idx int }

// gpiece is a piece of edge, in its own direction.
type gpiece struct {
	key    pkey
	pts    []geom.Vec3
	va, vb int
	edge   *Edge
	ta, tb float64
	closed bool
	cut    *bcut
}

type region struct {
	f      *bface
	cycles [][]puse // the outer boundary first
	p, n   geom.Vec3
	cls    int // classOut, classIn, classOnSame, classOnOpp
}

type puse struct {
	pc  *gpiece // nil: a pole gap (no edge)
	fwd bool
}

const (
	classOut = iota
	classIn
	classOnSame
	classOnOpp
)

func uniqueSorted(ts []float64, lo, hi float64, eval func(float64) geom.Vec3, tol float64) []float64 {
	sort.Float64s(ts)
	out := []float64{}
	last := lo
	pl := eval(lo)
	for _, t := range ts {
		if t <= lo || t >= hi {
			continue
		}
		p := eval(t)
		if p.Dist(pl) < tol || t-last < 1e-12 {
			continue
		}
		out = append(out, t)
		last, pl = t, p
	}
	// drop a last split too close to the end
	if n := len(out); n > 0 && eval(out[n-1]).Dist(eval(hi)) < tol {
		out = out[:n-1]
	}
	return out
}

// edgePieces splits an original edge at its splits (cached).
func (c *bctx) edgePieces(e *Edge) []*gpiece {
	if c.pcache == nil {
		c.pcache = map[any][]*gpiece{}
	}
	cache := c.pcache
	if ps, ok := cache[e]; ok {
		return ps
	}
	lo, hi := e.T0, e.T1
	sgn := 1.0
	if hi < lo {
		sgn = -1
	}
	// work in increasing order of sgn·t
	var ts []float64
	for _, t := range c.splits[e] {
		ts = append(ts, sgn*t)
	}
	eval := func(t float64) geom.Vec3 { return e.Curve.Eval(sgn * t) }
	inner := uniqueSorted(ts, sgn*lo, sgn*hi, eval, c.weldExact*10)
	params := append(append([]float64{sgn * lo}, inner...), sgn*hi)
	s := c.samples[e]
	var out []*gpiece
	for i := 0; i+1 < len(params); i++ {
		a, b := params[i], params[i+1]
		pts := []geom.Vec3{eval(a)}
		for k, t := range s.ts {
			if st := sgn * t; st > a+1e-12 && st < b-1e-12 {
				pts = append(pts, s.pts[k])
			}
		}
		pts = append(pts, eval(b))
		if i == 0 {
			pts[0] = e.Verts[0].P
		}
		if i == len(params)-2 {
			pts[len(pts)-1] = e.Verts[1].P
		}
		pc := &gpiece{key: pkey{0, c.enum[e], i}, pts: pts, edge: e, ta: sgn * a, tb: sgn * b}
		pc.va = c.weld(pts[0], 2, c.weldExact)
		pc.vb = c.weld(pts[len(pts)-1], 2, c.weldExact)
		out = append(out, pc)
	}
	cache[e] = out
	return out
}

// cutPieces splits an intersection curve at its splits (cached).
func (c *bctx) cutPieces(k *bcut) []*gpiece {
	if c.pcache == nil {
		c.pcache = map[any][]*gpiece{}
	}
	cache := c.pcache
	if ps, ok := cache[k]; ok {
		return ps
	}
	L := k.cum[len(k.cum)-1]
	eval := func(t float64) geom.Vec3 {
		i := sort.SearchFloat64s(k.cum, t)
		if i <= 0 {
			return k.pts[0]
		}
		if i >= len(k.cum) {
			return k.pts[len(k.pts)-1]
		}
		s := (t - k.cum[i-1]) / math.Max(k.cum[i]-k.cum[i-1], 1e-300)
		return k.pts[i-1].Lerp(k.pts[i], s)
	}
	inner := uniqueSorted(append([]float64(nil), k.splits...), 0, L, eval, c.weldLoose)
	var params []float64
	if k.closed && len(inner) > 0 {
		params = append(append([]float64(nil), inner...), inner[0]+L)
	} else {
		params = append(append([]float64{0}, inner...), L)
	}
	sub := func(a, b float64) []geom.Vec3 {
		// along the (possibly wrapped) curve from a to b
		var out []geom.Vec3
		out = append(out, eval(math.Mod(a, L+1e-300)))
		for t := 0; t < 2; t++ {
			off := float64(t) * L
			for i, cc := range k.cum {
				x := cc + off
				if x > a+1e-12 && x < b-1e-12 {
					out = append(out, k.pts[i])
				}
			}
		}
		bb := b
		if bb > L {
			bb -= L
		}
		return append(out, eval(bb))
	}
	var out []*gpiece
	for i := 0; i+1 < len(params); i++ {
		pts := sub(params[i], params[i+1])
		pc := &gpiece{key: pkey{1, k.id, i}, pts: dedupe3(pts), cut: k}
		pc.va = c.weld(pc.pts[0], 1, c.weldLoose)
		pc.vb = c.weld(pc.pts[len(pc.pts)-1], 1, c.weldLoose)
		out = append(out, pc)
	}
	if len(out) == 1 && k.closed && len(inner) == 0 {
		out[0].closed = true
		out[0].vb = out[0].va
	}
	cache[k] = out
	return out
}

func dedupe3(p []geom.Vec3) []geom.Vec3 {
	out := []geom.Vec3{p[0]}
	for _, q := range p[1:] {
		if q.Dist(out[len(out)-1]) > 1e-12 {
			out = append(out, q)
		}
	}
	if len(out) == 1 {
		out = append(out, p[len(p)-1])
	}
	return out
}

type gedge struct {
	pc   *gpiece
	uv   []geom.Vec2
	a, b int // nodes
}

// splitFaces cuts every face into regions along its cuts and imprints.
func (c *bctx) splitFaces() ([]*region, error) {
	var out []*region
	for _, F := range c.faces {
		rs, err := c.splitFace(F)
		if err != nil {
			return nil, err
		}
		out = append(out, rs...)
	}
	return out, nil
}

func (c *bctx) splitFace(F *bface) ([]*region, error) {
	var edges []gedge
	type node struct {
		vid int
		uv  geom.Vec2
	}
	var nodes []node
	nodeFor := func(vid int, uv geom.Vec2) int {
		for i, n := range nodes {
			if n.vid != vid {
				continue
			}
			if F.pu == 0 && F.pv == 0 {
				return i
			}
			lim := math.Inf(1)
			if F.pu > 0 {
				lim = F.pu / 4
			}
			if F.pv > 0 {
				lim = math.Min(lim, F.pv/4)
			}
			if n.uv.Dist(uv) < lim {
				return i
			}
		}
		nodes = append(nodes, node{vid, uv})
		return len(nodes) - 1
	}
	add := func(pc *gpiece, uv []geom.Vec2) {
		va, vb := pc.va, pc.vb
		edges = append(edges, gedge{pc: pc, uv: uv, a: nodeFor(va, uv[0]), b: nodeFor(vb, uv[len(uv)-1])})
	}
	mapUV := func(pts []geom.Vec3) []geom.Vec2 {
		uv := make([]geom.Vec2, len(pts))
		mid := len(pts) / 2
		if len(pts) == 2 {
			// place by the middle of the segment
			m, _ := F.place(F.uvRaw(pts[0].Lerp(pts[1], 0.5)))
			uv[0] = F.wrapNear(F.uvRaw(pts[0]), m)
			uv[1] = F.wrapNear(F.uvRaw(pts[1]), m)
			return uv
		}
		uv[mid], _ = F.place(F.uvRaw(pts[mid]))
		for i := mid + 1; i < len(pts); i++ {
			uv[i] = F.wrapNear(F.uvRaw(pts[i]), uv[i-1])
		}
		for i := mid - 1; i >= 0; i-- {
			uv[i] = F.wrapNear(F.uvRaw(pts[i]), uv[i+1])
		}
		return uv
	}
	// the boundary, split at the edges' splits
	for _, l := range F.loops {
		for i, hu := range l {
			e := hu.h.Edge
			for _, pc := range c.edgePieces(e) {
				uv := subUV(hu, pc.ta, pc.tb)
				if uv == nil {
					continue
				}
				add(pc, uv)
			}
			// a pole gap to the next half-edge
			nx := l[(i+1)%len(l)]
			a, b := hu.uv[len(hu.uv)-1], nx.uv[0]
			if a.Dist(b) > 1e-9 {
				vid := c.weld(hu.pts[len(hu.pts)-1], 2, c.weldExact)
				gp := &gpiece{key: pkey{2, F.f.ID, i}, va: vid, vb: vid}
				edges = append(edges, gedge{pc: gp, uv: []geom.Vec2{a, b}, a: nodeFor(vid, a), b: nodeFor(vid, b)})
			}
		}
	}
	for _, k := range F.cuts {
		for _, pc := range c.cutPieces(k) {
			add(pc, mapUV(pc.pts))
		}
	}
	for e := range F.imprints {
		for _, pc := range c.edgePieces(e) {
			mid := pc.pts[len(pc.pts)/2]
			if len(pc.pts) == 2 {
				mid = pc.pts[0].Lerp(pc.pts[1], 0.5)
			}
			q, ok := F.place(F.uvRaw(mid))
			if !ok || F.nearBoundary(q, c) {
				continue
			}
			add(pc, mapUV(pc.pts))
		}
	}
	sortEdges(edges)

	// half-edges: 2i forward, 2i+1 backward
	type half struct {
		from, to int
		ang      float64
	}
	hs := make([]half, 2*len(edges))
	minD := 1e-7 * math.Max(1, c.scale)
	dirAng := func(uv []geom.Vec2) float64 {
		for k := 1; k < len(uv); k++ {
			d := uv[k].Sub(uv[0])
			if d.Len() > minD || k == len(uv)-1 {
				return d.Angle()
			}
		}
		return 0
	}
	rev := func(uv []geom.Vec2) []geom.Vec2 {
		r := make([]geom.Vec2, len(uv))
		for i := range uv {
			r[len(uv)-1-i] = uv[i]
		}
		return r
	}
	for i, e := range edges {
		hs[2*i] = half{e.a, e.b, dirAng(e.uv)}
		hs[2*i+1] = half{e.b, e.a, dirAng(rev(e.uv))}
	}
	out := make([][]int, len(nodes))
	for i, h := range hs {
		out[h.from] = append(out[h.from], i)
	}
	for _, o := range out {
		sort.SliceStable(o, func(a, b int) bool { return hs[o[a]].ang < hs[o[b]].ang })
	}
	pos := make([]int, len(hs))
	for _, o := range out {
		for k, i := range o {
			pos[i] = k
		}
	}

	used := make([]bool, len(hs))
	type cycle struct {
		hs   []int
		poly []geom.Vec2
		area float64
	}
	var cycles []cycle
	for i := range hs {
		if used[i] {
			continue
		}
		var cyc []int
		h := i
		ok := false
		for n := 0; n <= len(hs); n++ {
			if used[h] {
				ok = h == i
				break
			}
			used[h] = true
			cyc = append(cyc, h)
			o := out[hs[h].to]
			tw := h ^ 1
			h = o[(pos[tw]-1+len(o))%len(o)]
		}
		if !ok {
			continue
		}
		cyc = dropSpurs(cyc)
		if len(cyc) == 0 {
			continue
		}
		var poly []geom.Vec2
		for _, h := range cyc {
			uv := edges[h/2].uv
			if h%2 == 1 {
				uv = rev(uv)
			}
			poly = append(poly, uv[:len(uv)-1]...)
		}
		cycles = append(cycles, cycle{hs: cyc, poly: poly, area: polyArea2(poly)})
	}
	toUses := func(cyc []int) []puse {
		var us []puse
		for _, h := range cyc {
			pc := edges[h/2].pc
			if pc.key.kind == 2 {
				us = append(us, puse{})
				continue
			}
			us = append(us, puse{pc: pc, fwd: h%2 == 0})
		}
		return us
	}
	var regs []*region
	var polys [][]geom.Vec2
	var holesOf [][]int
	for ci, cy := range cycles {
		if cy.area > 1e-14 {
			regs = append(regs, &region{f: F, cycles: [][]puse{toUses(cy.hs)}})
			polys = append(polys, cy.poly)
			holesOf = append(holesOf, []int{ci})
		}
	}
	for ci, cy := range cycles {
		if cy.area >= -1e-14 {
			continue
		}
		best, ba := -1, math.Inf(1)
		for ri := range regs {
			a := cycles[holesOf[ri][0]].area
			if a > -cy.area*(1+1e-9)+1e-12 && a < ba && pointInPoly2(cy.poly[0].Lerp(cy.poly[len(cy.poly)/2], 1e-3), polys[ri]) {
				best, ba = ri, a
			}
		}
		if best >= 0 {
			regs[best].cycles = append(regs[best].cycles, toUses(cy.hs))
			holesOf[best] = append(holesOf[best], ci)
		}
	}
	// keep the regions inside the face; find a point in each
	var res []*region
	for ri, r := range regs {
		var pts []geom.Vec2
		var segs [][2]int
		for _, ci := range holesOf[ri] {
			p := cycles[ci].poly
			st := len(pts)
			pts = append(pts, p...)
			for k := range p {
				segs = append(segs, [2]int{st + k, st + (k+1)%len(p)})
			}
		}
		pts, segs = dedupePts(pts, segs)
		tris := geom.Triangulate(pts, segs)
		bestA, best := 0.0, geom.Vec2{}
		for _, t := range tris {
			a := math.Abs(pts[t[1]].Sub(pts[t[0]]).Cross(pts[t[2]].Sub(pts[t[0]])))
			if a > bestA {
				bestA = a
				best = pts[t[0]].Add(pts[t[1]]).Add(pts[t[2]]).Scale(1.0 / 3)
			}
		}
		if bestA == 0 {
			continue
		}
		if !F.inside(best) {
			continue
		}
		u, v := best.X/F.su, best.Y/F.sv
		r.p = F.s.Eval(u, v)
		r.n = F.f.OutwardNormal(u, v)
		res = append(res, r)
	}
	if len(res) == 0 && len(F.f.Loops) > 0 {
		var areas []float64
		for _, cy := range cycles {
			areas = append(areas, cy.area)
		}
		return nil, fmt.Errorf("boolean: face %q lost all of itself while being split (%d edges, %d nodes, cycles %v, %d regions)", F.f.Name, len(edges), len(nodes), areas, len(regs))
	}
	return res, nil
}

func sortEdges(es []gedge) {
	sort.SliceStable(es, func(i, j int) bool {
		a, b := es[i].pc.key, es[j].pc.key
		if a.kind != b.kind {
			return a.kind < b.kind
		}
		if a.id != b.id {
			return a.id < b.id
		}
		return a.idx < b.idx
	})
}

func dropSpurs(l []int) []int {
	changed := true
	for changed && len(l) > 1 {
		changed = false
		for i := 0; i < len(l); i++ {
			j := (i + 1) % len(l)
			if l[i]^1 == l[j] {
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
	if len(l) == 2 && l[0]^1 == l[1] {
		return nil
	}
	return l
}

func polyArea2(p []geom.Vec2) float64 {
	a := 0.0
	for i := range p {
		a += p[i].Cross(p[(i+1)%len(p)])
	}
	return a / 2
}

func pointInPoly2(q geom.Vec2, p []geom.Vec2) bool {
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

func dedupePts(pts []geom.Vec2, segs [][2]int) ([]geom.Vec2, [][2]int) {
	idx := make([]int, len(pts))
	var out []geom.Vec2
	m := map[[2]int64]int{}
	for i, p := range pts {
		k := [2]int64{int64(math.Round(p.X * 1e9)), int64(math.Round(p.Y * 1e9))}
		if j, ok := m[k]; ok {
			idx[i] = j
			continue
		}
		m[k] = len(out)
		idx[i] = len(out)
		out = append(out, p)
	}
	var ss [][2]int
	for _, s := range segs {
		a, b := idx[s[0]], idx[s[1]]
		if a != b {
			ss = append(ss, [2]int{a, b})
		}
	}
	return out, ss
}

// subUV is the stretch of a half-edge's (u, v) samples between edge
// parameters ta and tb (in the edge's direction; nil if outside it).
func subUV(hu heUV, ta, tb float64) []geom.Vec2 {
	n := len(hu.ts)
	if n < 2 {
		return nil
	}
	// as a function of the edge parameter, in the edge's own direction
	ts, uv := hu.ts, hu.uv
	if !hu.h.Fwd {
		ts = make([]float64, n)
		uv = make([]geom.Vec2, n)
		for i := 0; i < n; i++ {
			ts[i], uv[i] = hu.ts[n-1-i], hu.uv[n-1-i]
		}
	}
	sgn := 1.0
	if ts[n-1] < ts[0] {
		sgn = -1
	}
	at := func(t float64) geom.Vec2 {
		st := sgn * t
		for i := 0; i+1 < n; i++ {
			a, b := sgn*ts[i], sgn*ts[i+1]
			if st >= a-1e-12 && st <= b+1e-12 {
				if b == a {
					return uv[i]
				}
				return uv[i].Lerp(uv[i+1], (st-a)/(b-a))
			}
		}
		if st < sgn*ts[0] {
			return uv[0]
		}
		return uv[n-1]
	}
	out := []geom.Vec2{at(ta)}
	for i := 0; i < n; i++ {
		if st := sgn * ts[i]; st > sgn*ta+1e-12 && st < sgn*tb-1e-12 {
			out = append(out, uv[i])
		}
	}
	return append(out, at(tb))
}

// nearBoundary: q is on the face's boundary (an imprint along an edge).
func (f *bface) nearBoundary(q geom.Vec2, c *bctx) bool {
	tol := c.weldLoose * math.Max(f.su, f.sv) / math.Max(math.Min(f.su, f.sv), 1e-9)
	for _, s := range f.segs {
		ab := s.b.Sub(s.a)
		t := 0.0
		if l2 := ab.Dot(ab); l2 > 0 {
			t = math.Max(0, math.Min(1, q.Sub(s.a).Dot(ab)/l2))
		}
		if s.a.Add(ab.Scale(t)).Dist(q) < tol {
			return true
		}
	}
	return false
}

// classify says where each region is relative to the other body.
func (c *bctx) classify(rs []*region) {
	for _, r := range rs {
		m := c.meshes[1-r.f.body]
		in1 := windingNumber(m, r.p.Add(r.n.Scale(c.eps))) > 0.5
		in0 := windingNumber(m, r.p.Sub(r.n.Scale(c.eps))) > 0.5
		switch {
		case in0 && in1:
			r.cls = classIn
		case !in0 && !in1:
			r.cls = classOut
		case in0:
			r.cls = classOnSame
		default:
			r.cls = classOnOpp
		}
	}
}

// sew builds the result from the regions the operation keeps.
func (c *bctx) sew(rs []*region, op BoolOp, name string) (*Body, error) {
	keep := func(r *region) (bool, bool) { // keep, flip
		a := r.f.body == 0
		switch op {
		case Union:
			return r.cls == classOut || (a && r.cls == classOnSame), false
		case Subtract:
			if a {
				return r.cls == classOut || r.cls == classOnOpp, false
			}
			return r.cls == classIn, true
		case Intersect:
			return r.cls == classIn || (a && r.cls == classOnSame), false
		}
		return false, false
	}
	b := NewBuilder(name)
	verts := map[int]*Vertex{}
	vert := func(id int) *Vertex {
		if v, ok := verts[id]; ok {
			return v
		}
		v := b.Vertex(c.verts[id].p)
		verts[id] = v
		return v
	}
	edges := map[pkey]*Edge{}
	edge := func(pc *gpiece) *Edge {
		if e, ok := edges[pc.key]; ok {
			return e
		}
		va, vb := vert(pc.va), vert(pc.vb)
		var e *Edge
		if pc.key.kind == 0 {
			e = b.Edge(va, vb, pc.edge.Curve, pc.ta, pc.tb)
		} else {
			pts := append([]geom.Vec3(nil), pc.pts...)
			pts[0], pts[len(pts)-1] = va.P, vb.P
			if len(pts) == 2 {
				e = b.LineEdge(va, vb)
			} else {
				tol := c.ctol
				if pc.cut != nil {
					tol = math.Max(tol, curveTol(pc.cut.faces[0].s, pc.cut.faces[1].s, pts))
				}
				pl := geom.NewPolyline(pts, tol)
				e = b.Edge(va, vb, pl, 0, pl.Length())
			}
		}
		edges[pc.key] = e
		return e
	}
	n := 0
	for _, r := range rs {
		k, flip := keep(r)
		if !k {
			continue
		}
		var loops [][]Use
		for _, cyc := range r.cycles {
			var l []Use
			for _, u := range cyc {
				if u.pc == nil {
					continue
				}
				pc, fwd := u.pc, u.fwd
				if al, ok := c.alias[pc.key]; ok {
					pc, fwd = al.to, fwd == al.same
				}
				l = append(l, Use{E: edge(pc), Fwd: fwd})
			}
			if len(l) == 0 {
				continue
			}
			if r.f.f.Reversed != flip {
				l = reverseUses(l)
			}
			loops = append(loops, l)
		}
		if len(loops) == 0 {
			continue
		}
		b.Face(r.f.s, r.f.f.Reversed != flip, r.f.f.Name, loops...)
		n++
	}
	if n == 0 {
		return nil, ErrEmpty
	}
	body, err := b.Finish()
	if err != nil {
		return nil, fmt.Errorf("boolean %s failed: %v", op, err)
	}
	return body, nil
}

func reverseUses(l []Use) []Use {
	out := make([]Use, len(l))
	for i, u := range l {
		out[len(l)-1-i] = Use{E: u.E, Fwd: !u.Fwd}
	}
	return out
}
