package brep

// Merging faces after a boolean: faces on one plane facing the same way
// that share an edge (two blocks side by side, a pocket's floor met by a
// cut along it) become one face, the edges between them go, and edges left
// running on through a vertex with nothing else at it (two lines on one
// line, two arcs of one circle) become one edge.

import (
	"math"

	"forma/geom"
)

// MergeFaces returns b with its coplanar neighbouring faces merged, or b
// itself when there is nothing to merge (or merging would not make a valid
// body).
func MergeFaces(b *Body) *Body {
	faces := b.Faces()
	if len(faces) < 2 {
		return b
	}
	tol := 1e-9 * math.Max(1, b.Bounds().Diagonal())
	idx := map[*Face]int{}
	parent := make([]int, len(faces))
	for i, f := range faces {
		idx[f] = i
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	edges := b.Edges()
	inner := map[*Edge]bool{} // edges inside a merged face
	for _, e := range edges {
		if e.HE[0] == nil || e.HE[1] == nil {
			return b
		}
		f1, f2 := e.HE[0].Loop.Face, e.HE[1].Loop.Face
		if f1 == f2 || !samePlane(f1, f2, tol) {
			continue
		}
		inner[e] = true
		parent[find(idx[f1])] = find(idx[f2])
	}
	if len(inner) == 0 {
		return b
	}
	// an edge whose two sides ended up in one merged face goes too
	for _, e := range edges {
		if !inner[e] && find(idx[e.HE[0].Loop.Face]) == find(idx[e.HE[1].Loop.Face]) && e.HE[0].Loop.Face != e.HE[1].Loop.Face {
			inner[e] = true
		}
	}

	// the loops of each merged face: walk the kept half-edges, crossing the
	// edges that go round the vertex they meet at
	groups := map[int][]*Face{}
	var order []int
	for i, f := range faces {
		r := find(i)
		if _, ok := groups[r]; !ok {
			order = append(order, r)
		}
		groups[r] = append(groups[r], f)
	}
	loopsOf := map[int][][]*HalfEdge{}
	for _, r := range order {
		fs := groups[r]
		if len(fs) == 1 {
			for _, l := range fs[0].Loops {
				if l.First == nil {
					return b
				}
				loopsOf[r] = append(loopsOf[r], l.HalfEdges())
			}
			continue
		}
		used := map[*HalfEdge]bool{}
		var loops [][]*HalfEdge
		for _, f := range fs {
			for _, l := range f.Loops {
				if l.First == nil {
					return b
				}
				for _, h := range l.HalfEdges() {
					if used[h] || inner[h.Edge] {
						continue
					}
					var loop []*HalfEdge
					cur := h
					for !used[cur] {
						used[cur] = true
						loop = append(loop, cur)
						next := cur.Next
						for k := 0; inner[next.Edge]; k++ {
							if k > 1000 {
								return b
							}
							next = next.Twin().Next
						}
						cur = next
						if len(loop) > 1<<16 {
							return b
						}
					}
					if cur != h {
						return b // ran into another loop: not a plain face
					}
					loops = append(loops, loop)
				}
			}
		}
		// the outer loop first: the one going round anticlockwise seen
		// from outside
		pl := fs[0].Surf.(geom.Plane)
		n := pl.F.Z
		if fs[0].Reversed {
			n = n.Neg()
		}
		outer := -1
		for i, l := range loops {
			if loopArea(l, pl.F, n) > 0 {
				if outer >= 0 {
					return b // two pieces: not one face
				}
				outer = i
			}
		}
		if outer < 0 {
			return b
		}
		loops[0], loops[outer] = loops[outer], loops[0]
		loopsOf[r] = loops
	}

	// edges running on through a vertex with nothing else at it join
	kept := []*Edge{}
	at := map[*Vertex][]*Edge{}
	for _, e := range edges {
		if inner[e] {
			continue
		}
		kept = append(kept, e)
		at[e.Verts[0]] = append(at[e.Verts[0]], e)
		if e.Verts[1] != e.Verts[0] {
			at[e.Verts[1]] = append(at[e.Verts[1]], e)
		}
	}
	joinAt := map[*Vertex]bool{}
	for v, es := range at {
		if len(es) == 2 && es[0] != es[1] && joinable(es[0], es[1], tol) {
			joinAt[v] = true
		}
	}
	// chains of edges through joinable vertices
	type chainEdge struct {
		e   *Edge
		fwd bool // runs the chain's way
	}
	type chain struct {
		parts      []chainEdge
		start, end *Vertex
		closed     bool
	}
	chainOf := map[*Edge]int{}
	var chains []*chain
	other := func(e *Edge, v *Vertex) *Vertex {
		if e.Verts[0] == v {
			return e.Verts[1]
		}
		return e.Verts[0]
	}
	nextAt := func(e *Edge, v *Vertex) *Edge {
		for _, o := range at[v] {
			if o != e {
				return o
			}
		}
		return nil
	}
	for _, e := range kept {
		if _, ok := chainOf[e]; ok {
			continue
		}
		// back up to the chain's start
		first, from := e, e.Verts[0]
		for steps := 0; joinAt[from] && steps < len(kept); steps++ {
			p := nextAt(first, from)
			if p == e {
				break // a closed chain
			}
			from = other(p, from)
			first = p
		}
		c := &chain{start: from}
		cur, v := first, from
		for steps := 0; steps <= len(kept); steps++ {
			chainOf[cur] = len(chains)
			c.parts = append(c.parts, chainEdge{cur, cur.Verts[0] == v})
			v = other(cur, v)
			if !joinAt[v] {
				break
			}
			nx := nextAt(cur, v)
			if nx == first {
				c.closed = true
				break
			}
			cur = nx
		}
		c.end = v
		chains = append(chains, c)
	}

	nb := NewBuilder(b.Name)
	verts := map[*Vertex]*Vertex{}
	vert := func(v *Vertex) *Vertex {
		if nv, ok := verts[v]; ok {
			return nv
		}
		nv := nb.Vertex(v.P)
		verts[v] = nv
		return nv
	}
	newEdges := make([]*Edge, len(chains))
	edgeOf := func(ci int) *Edge {
		if newEdges[ci] != nil {
			return newEdges[ci]
		}
		c := chains[ci]
		var ne *Edge
		if len(c.parts) == 1 {
			e := c.parts[0].e
			ne = nb.Edge(vert(e.Verts[0]), vert(e.Verts[1]), e.Curve, e.T0, e.T1)
		} else {
			ne = joinChain(nb, vert(c.start), vert(c.end), c.parts[0].e, c.parts[0].fwd, c.closed)
		}
		newEdges[ci] = ne
		return ne
	}
	// a half-edge's use of its chain's new edge
	use := func(h *HalfEdge) (int, bool) {
		ci := chainOf[h.Edge]
		c := chains[ci]
		if len(c.parts) == 1 {
			return ci, h.Fwd
		}
		for _, p := range c.parts {
			if p.e == h.Edge {
				return ci, h.Fwd == p.fwd
			}
		}
		return ci, h.Fwd
	}
	for _, r := range order {
		fs := groups[r]
		var loops [][]Use
		for _, l := range loopsOf[r] {
			// start the loop where its chain changes, then collapse runs
			n := len(l)
			s := 0
			for i := 0; i < n; i++ {
				a, _ := use(l[(i+n-1)%n])
				c, _ := use(l[i])
				if a != c {
					s = i
					break
				}
			}
			var uses []Use
			last := -1
			for i := 0; i < n; i++ {
				ci, fwd := use(l[(s+i)%n])
				if ci == last {
					continue
				}
				last = ci
				uses = append(uses, Use{E: edgeOf(ci), Fwd: fwd})
			}
			loops = append(loops, uses)
		}
		nb.Face(fs[0].Surf, fs[0].Reversed, fs[0].Name, loops...)
	}
	out, err := nb.Finish()
	if err != nil {
		return b
	}
	return out
}

// samePlane: two faces lie on one plane and face the same way.
func samePlane(f1, f2 *Face, tol float64) bool {
	p1, ok1 := f1.Surf.(geom.Plane)
	p2, ok2 := f2.Surf.(geom.Plane)
	if !ok1 || !ok2 {
		return false
	}
	n1, n2 := p1.F.Z, p2.F.Z
	if f1.Reversed {
		n1 = n1.Neg()
	}
	if f2.Reversed {
		n2 = n2.Neg()
	}
	return n1.Dot(n2) > 1-1e-12 && math.Abs(p2.F.O.Sub(p1.F.O).Dot(n1)) < tol
}

// joinable: two edges meeting at a vertex lie on one line, circle or ellipse.
func joinable(a, b *Edge, tol float64) bool {
	switch ca := a.Curve.(type) {
	case geom.Line:
		cb, ok := b.Curve.(geom.Line)
		return ok && math.Abs(math.Abs(ca.D.Dot(cb.D))-1) < 1e-12 && ca.Eval(ca.Project(cb.P)).Dist(cb.P) < tol
	case geom.Circle:
		cb, ok := b.Curve.(geom.Circle)
		return ok && ca.F.O.Dist(cb.F.O) < tol && math.Abs(ca.R-cb.R) < tol &&
			math.Abs(math.Abs(ca.F.Z.Dot(cb.F.Z))-1) < 1e-12
	case geom.Ellipse:
		cb, ok := b.Curve.(geom.Ellipse)
		return ok && ca.F.O.Dist(cb.F.O) < tol && math.Abs(ca.A-cb.A) < tol && math.Abs(ca.B-cb.B) < tol &&
			math.Abs(math.Abs(ca.F.Z.Dot(cb.F.Z))-1) < 1e-12 && math.Abs(math.Abs(ca.F.X.Dot(cb.F.X))-1) < 1e-12
	}
	return false
}

// joinChain is one edge from va to vb along first's curve, the way first
// runs from va (fwd: first runs along its curve from va).
func joinChain(nb *Builder, va, vb *Vertex, first *Edge, fwd, closed bool) *Edge {
	if _, ok := first.Curve.(geom.Line); ok {
		return nb.LineEdge(va, vb)
	}
	// a point a little way along first from va
	t0, t1 := first.T0, first.T1
	if !fwd {
		t0, t1 = t1, t0
	}
	mid := first.Curve.Eval(t0 + (t1-t0)/2)
	arc := geom.StartAt(first.Curve, va.P, false)
	full := closed || va == vb
	end := geom.ArcEnd(arc, vb.P, full)
	if tm := arc.Project(mid); full && tm > math.Pi || !full && tm > end {
		arc = geom.StartAt(first.Curve, va.P, true)
		end = geom.ArcEnd(arc, vb.P, full)
	}
	return nb.Edge(va, vb, arc, 0, end)
}

// loopArea is the signed area of a loop in plane frame f, positive when it
// goes anticlockwise about n.
func loopArea(l []*HalfEdge, f geom.Frame, n geom.Vec3) float64 {
	var pts []geom.Vec2
	for _, h := range l {
		e := h.Edge
		ts := geom.SampleParams(e.Curve, e.T0, e.T1, 1e-3*math.Max(1, e.Verts[0].P.Dist(e.Verts[1].P)), math.Inf(1))
		if !h.Fwd {
			for i, j := 0, len(ts)-1; i < j; i, j = i+1, j-1 {
				ts[i], ts[j] = ts[j], ts[i]
			}
		}
		for _, t := range ts[:len(ts)-1] {
			l := f.ToLocal(e.Curve.Eval(t))
			pts = append(pts, geom.V2(l.X, l.Y))
		}
	}
	a := 0.0
	for i := range pts {
		a += pts[i].Cross(pts[(i+1)%len(pts)])
	}
	if f.Z.Dot(n) < 0 {
		a = -a
	}
	return a / 2
}
