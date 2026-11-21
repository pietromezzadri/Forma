package geom

import (
	"math"
	"sort"
)

// Triangulate makes a constrained Delaunay triangulation of pts in which
// every segment in segs (pairs of point indices) is an edge, keeping only
// the triangles inside the polygon the segments bound (even-odd rule).
// Triangles come back counter-clockwise as index triples.
func Triangulate(pts []Vec2, segs [][2]int) [][3]int {
	if len(pts) < 3 {
		return nil
	}
	t := newTriangulation(pts)
	for i := range pts {
		t.insert(i)
	}
	for _, s := range segs {
		if s[0] != s[1] {
			t.enforce(s[0], s[1])
		}
	}
	var out [][3]int
	n := len(pts)
	for _, tr := range t.tris {
		if tr.dead || tr.v[0] >= n || tr.v[1] >= n || tr.v[2] >= n {
			continue
		}
		a, b, c := pts[tr.v[0]], pts[tr.v[1]], pts[tr.v[2]]
		if math.Abs(orient(a, b, c)) < 1e-18 {
			continue
		}
		cen := a.Add(b).Add(c).Scale(1.0 / 3)
		if insideSegs(cen, pts, segs) {
			out = append(out, tr.v)
		}
	}
	return out
}

// orient > 0 when a, b, c turn counter-clockwise.
func orient(a, b, c Vec2) float64 { return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X) }

// insideSegs: even-odd point-in-polygon over a set of segments.
func insideSegs(p Vec2, pts []Vec2, segs [][2]int) bool {
	in := false
	for _, s := range segs {
		a, b := pts[s[0]], pts[s[1]]
		if (a.Y > p.Y) != (b.Y > p.Y) {
			x := a.X + (p.Y-a.Y)*(b.X-a.X)/(b.Y-a.Y)
			if x > p.X {
				in = !in
			}
		}
	}
	return in
}

type cdtTri struct {
	v    [3]int
	dead bool
}

type triangulation struct {
	pts  []Vec2
	tris []cdtTri
	// edge (a, b) → triangle holding the directed edge a→b
	edges map[[2]int]int
	// vt[i] is a triangle holding vertex i (the last one made with it)
	vt []int
}

func newTriangulation(pts []Vec2) *triangulation {
	var box Box
	for _, p := range pts {
		box.Add(Vec3{p.X, p.Y, 0})
	}
	c := box.Center()
	r := math.Max(box.Max.X-box.Min.X, box.Max.Y-box.Min.Y)*20 + 1
	all := append(append([]Vec2(nil), pts...),
		Vec2{c.X - r, c.Y - r}, Vec2{c.X + r, c.Y - r}, Vec2{c.X, c.Y + r})
	t := &triangulation{pts: all, edges: map[[2]int]int{}, vt: make([]int, len(all))}
	n := len(pts)
	t.add([3]int{n, n + 1, n + 2})
	return t
}

func (t *triangulation) add(v [3]int) int {
	if orient(t.pts[v[0]], t.pts[v[1]], t.pts[v[2]]) < 0 {
		v[1], v[2] = v[2], v[1]
	}
	id := len(t.tris)
	t.tris = append(t.tris, cdtTri{v: v})
	for i := 0; i < 3; i++ {
		t.edges[[2]int{v[i], v[(i+1)%3]}] = id
		t.vt[v[i]] = id
	}
	return id
}

func (t *triangulation) kill(id int) {
	tr := &t.tris[id]
	tr.dead = true
	for i := 0; i < 3; i++ {
		k := [2]int{tr.v[i], tr.v[(i+1)%3]}
		if t.edges[k] == id {
			delete(t.edges, k)
		}
	}
}

func (t *triangulation) inCircle(id int, p Vec2) bool {
	v := t.tris[id].v
	a, b, c := t.pts[v[0]], t.pts[v[1]], t.pts[v[2]]
	ax, ay := a.X-p.X, a.Y-p.Y
	bx, by := b.X-p.X, b.Y-p.Y
	cx, cy := c.X-p.X, c.Y-p.Y
	det := (ax*ax+ay*ay)*(bx*cy-cx*by) - (bx*bx+by*by)*(ax*cy-cx*ay) + (cx*cx+cy*cy)*(ax*by-bx*ay)
	return det > 1e-24
}

// insert adds point i by Bowyer–Watson: find the triangle holding it, grow
// the cavity of triangles whose circumcircle contains it, refill with a fan.
func (t *triangulation) insert(i int) {
	p := t.pts[i]
	start := t.locate(p)
	for id := len(t.tris) - 1; start < 0 && id >= 0; id-- {
		tr := t.tris[id]
		if tr.dead {
			continue
		}
		a, b, c := t.pts[tr.v[0]], t.pts[tr.v[1]], t.pts[tr.v[2]]
		if orient(a, b, p) >= 0 && orient(b, c, p) >= 0 && orient(c, a, p) >= 0 {
			start = id
			break
		}
	}
	if start < 0 {
		CDTStats.Unlocated++
		return
	}
	cavity := map[int]bool{start: true}
	stack := []int{start}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		v := t.tris[id].v
		for k := 0; k < 3; k++ {
			nb, ok := t.edges[[2]int{v[(k+1)%3], v[k]}]
			if ok && !cavity[nb] && t.inCircle(nb, p) {
				cavity[nb] = true
				stack = append(stack, nb)
			}
		}
	}
	// The cavity must be star-shaped from p: rounding on near co-circular
	// points can take in a triangle p can't see the far side of. Drop such
	// triangles (or, when p sits on an edge of the first one, take in its
	// neighbour) until every boundary edge faces p.
	var boundary [][2]int
	for pass := 0; ; pass++ {
		boundary = boundary[:0]
		bad := -1
		across := -1
		for _, id := range sortedKeys(cavity) {
			v := t.tris[id].v
			for k := 0; k < 3; k++ {
				a, b := v[k], v[(k+1)%3]
				nb, ok := t.edges[[2]int{b, a}]
				if ok && cavity[nb] {
					continue
				}
				boundary = append(boundary, [2]int{a, b})
				if bad < 0 && orient(t.pts[a], t.pts[b], p) <= 0 {
					bad = id
					if ok {
						across = nb
					} else {
						across = -1
					}
				}
			}
		}
		if bad < 0 || pass > 64 {
			break
		}
		if bad != start {
			delete(cavity, bad)
		} else if across >= 0 {
			cavity[across] = true
		} else {
			break
		}
	}
	ids := make([]int, 0, len(cavity))
	for id := range cavity {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		t.kill(id)
	}
	for _, e := range boundary {
		if e[0] == i || e[1] == i {
			continue
		}
		t.add([3]int{e[0], e[1], i})
	}
}

func segsCross(a, b, c, d Vec2) bool {
	d1 := orient(a, b, c)
	d2 := orient(a, b, d)
	d3 := orient(c, d, a)
	d4 := orient(c, d, b)
	return ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) && ((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0))
}

// enforce makes a–b an edge by flipping the edges that cross it (Sloan):
// the crossing edges found by walking along a–b from a, flipped until none
// crosses. A point lying on a–b splits it in two.
func (t *triangulation) enforce(a, b int) { t.enforceDepth(a, b, 0) }

func (t *triangulation) enforceDepth(a, b, depth int) {
	if _, ok := t.edges[[2]int{a, b}]; ok {
		return
	}
	if _, ok := t.edges[[2]int{b, a}]; ok {
		return
	}
	cross, mid, ok := t.walk(a, b)
	if !ok {
		t.enforceScan(a, b)
		return
	}
	if mid >= 0 {
		if depth > 64 {
			CDTStats.Unrecovered++
			return
		}
		t.enforceDepth(a, mid, depth+1)
		t.enforceDepth(mid, b, depth+1)
		return
	}
	pa, pb := t.pts[a], t.pts[b]
	queue := cross
	for guard := 0; len(queue) > 0; guard++ {
		if guard > 64*len(cross)+1000 {
			t.enforceScan(a, b)
			return
		}
		k := queue[0]
		queue = queue[1:]
		t1, ok1 := t.edges[k]
		t2, ok2 := t.edges[[2]int{k[1], k[0]}]
		if !ok1 || !ok2 || !segsCross(pa, pb, t.pts[k[0]], t.pts[k[1]]) {
			continue
		}
		p := third(t.tris[t1].v, k[0], k[1])
		q := third(t.tris[t2].v, k[1], k[0])
		if !t.flip(k[0], k[1]) {
			queue = append(queue, k) // not convex yet: later
			continue
		}
		if segsCross(pa, pb, t.pts[p], t.pts[q]) {
			queue = append(queue, [2]int{p, q})
		}
	}
}

// walk lists the edges crossing a–b, from a's triangles across to b; mid is
// a point lying on a–b on the way (the list then stops there), -1 if none.
// ok is false if the walk got lost.
func (t *triangulation) walk(a, b int) (cross [][2]int, mid int, ok bool) {
	pa, pb := t.pts[a], t.pts[b]
	start := t.vt[a]
	if start < 0 || start >= len(t.tris) || t.tris[start].dead || !holds(t.tris[start].v, a) {
		return nil, -1, false
	}
	l2 := pa.Sub(pb).Dot(pa.Sub(pb))
	on := func(x int) bool {
		px := t.pts[x]
		if math.Abs(orient(pa, pb, px)) > 1e-12*l2 {
			return false
		}
		s := px.Sub(pa).Dot(pb.Sub(pa)) / l2
		return s > 0 && s < 1
	}
	// round a to the triangle whose far edge a–b goes through
	var x, y int
	found := false
	id := start
	for n := 0; n < len(t.tris)+3; n++ {
		v := t.tris[id].v
		k := 0
		for v[k] != a {
			k++
		}
		x, y = v[(k+1)%3], v[(k+2)%3]
		if x == b || y == b {
			return nil, -1, true
		}
		if on(x) {
			return nil, x, true
		}
		if segsCross(pa, pb, t.pts[x], t.pts[y]) {
			found = true
			break
		}
		nb, has := t.edges[[2]int{a, y}]
		if !has || nb == start {
			break
		}
		id = nb
	}
	if !found {
		return nil, -1, false
	}
	// across x→y and on toward b
	for n := 0; n < len(t.tris)+3; n++ {
		cross = append(cross, [2]int{x, y})
		nb, has := t.edges[[2]int{y, x}]
		if !has {
			return nil, -1, false
		}
		z := third(t.tris[nb].v, y, x)
		if z == b {
			return cross, -1, true
		}
		if on(z) {
			return nil, z, true
		}
		// the triangle runs y → x → z: on across x→z or z→y
		switch {
		case segsCross(pa, pb, t.pts[x], t.pts[z]):
			y = z
		case segsCross(pa, pb, t.pts[z], t.pts[y]):
			x = z
		default:
			return nil, -1, false
		}
	}
	return nil, -1, false
}

func holds(v [3]int, a int) bool { return v[0] == a || v[1] == a || v[2] == a }

// enforceScan is enforce scanning every edge for the crossing ones (when
// the walk gets lost).
func (t *triangulation) enforceScan(a, b int) {
	if _, ok := t.edges[[2]int{a, b}]; ok {
		return
	}
	if _, ok := t.edges[[2]int{b, a}]; ok {
		return
	}
	pa, pb := t.pts[a], t.pts[b]
	for iter := 0; iter < 10000; iter++ {
		// collect crossing edges
		var crossing [][2]int
		for k, id := range t.edges {
			if k[0] > k[1] {
				continue
			}
			if _, ok := t.edges[[2]int{k[1], k[0]}]; !ok {
				continue
			}
			_ = id
			if k[0] == a || k[0] == b || k[1] == a || k[1] == b {
				continue
			}
			if segsCross(pa, pb, t.pts[k[0]], t.pts[k[1]]) {
				crossing = append(crossing, k)
			}
		}
		if len(crossing) == 0 {
			return
		}
		sort.Slice(crossing, func(i, j int) bool {
			if crossing[i][0] != crossing[j][0] {
				return crossing[i][0] < crossing[j][0]
			}
			return crossing[i][1] < crossing[j][1]
		})
		flipped := false
		for _, k := range crossing {
			if t.flip(k[0], k[1]) {
				flipped = true
			}
		}
		if !flipped {
			CDTStats.Unrecovered++
			return // can't recover (collinear points); leave as is
		}
	}
}

// flip turns the diagonal u–v of the quad its two triangles make, if convex.
func (t *triangulation) flip(u, v int) bool {
	t1, ok1 := t.edges[[2]int{u, v}]
	t2, ok2 := t.edges[[2]int{v, u}]
	if !ok1 || !ok2 {
		return false
	}
	p := third(t.tris[t1].v, u, v)
	q := third(t.tris[t2].v, v, u)
	pu, pv, pp, pq := t.pts[u], t.pts[v], t.pts[p], t.pts[q]
	// convex quad u, q, v, p: p and q on opposite sides of u–v and u, v on opposite sides of p–q
	if !segsCross(pu, pv, pp, pq) {
		return false
	}
	t.kill(t1)
	t.kill(t2)
	t.add([3]int{p, q, v})
	t.add([3]int{q, p, u})
	return true
}

func third(v [3]int, a, b int) int {
	for _, x := range v {
		if x != a && x != b {
			return x
		}
	}
	return -1
}

func sortedKeys(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

// locate walks from the newest live triangle toward p, crossing the edge p
// lies beyond each time; -1 if the walk gets lost (the caller then scans).
func (t *triangulation) locate(p Vec2) int {
	id := -1
	for k := len(t.tris) - 1; k >= 0 && k >= len(t.tris)-8; k-- {
		if !t.tris[k].dead {
			id = k
			break
		}
	}
	if id < 0 {
		return -1
	}
	for steps := 0; steps < 4*len(t.tris)+16; steps++ {
		v := t.tris[id].v
		moved := false
		for k := 0; k < 3; k++ {
			a, b := v[k], v[(k+1)%3]
			if orient(t.pts[a], t.pts[b], p) < 0 {
				nb, ok := t.edges[[2]int{b, a}]
				if !ok {
					return -1
				}
				id, moved = nb, true
				break
			}
		}
		if !moved {
			return id
		}
	}
	return -1
}
