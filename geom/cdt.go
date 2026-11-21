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
	bands := NewSegBands(pts, segs)
	for _, tr := range t.tris {
		if tr.dead || tr.v[0] >= n || tr.v[1] >= n || tr.v[2] >= n {
			continue
		}
		a, b, c := pts[tr.v[0]], pts[tr.v[1]], pts[tr.v[2]]
		if math.Abs(orient(a, b, c)) < 1e-18 {
			continue
		}
		cen := a.Add(b).Add(c).Scale(1.0 / 3)
		if bands.Inside(cen) {
			out = append(out, tr.v)
		}
	}
	return out
}

// SegBands holds the segments in horizontal bands, each segment in every
// band its y range touches, for the even-odd test at a point.
type SegBands struct {
	pts    []Vec2
	segs   [][2]int
	y0, dy float64
	bands  [][]int32
}

func NewSegBands(pts []Vec2, segs [][2]int) *SegBands {
	b := &SegBands{pts: pts, segs: segs}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, s := range segs {
		for _, k := range s {
			lo, hi = math.Min(lo, pts[k].Y), math.Max(hi, pts[k].Y)
		}
	}
	nb := int(math.Sqrt(float64(len(segs)))) + 1
	if len(segs) == 0 || hi <= lo {
		nb = 1
	}
	b.y0, b.dy = lo, (hi-lo)/float64(nb)
	if b.dy <= 0 {
		b.dy = 1
	}
	b.bands = make([][]int32, nb)
	for i, s := range segs {
		ya, yb := pts[s[0]].Y, pts[s[1]].Y
		if ya > yb {
			ya, yb = yb, ya
		}
		for k := b.band(ya); k <= b.band(yb); k++ {
			b.bands[k] = append(b.bands[k], int32(i))
		}
	}
	return b
}

func (b *SegBands) band(y float64) int {
	k := int((y - b.y0) / b.dy)
	if k < 0 {
		return 0
	}
	if k >= len(b.bands) {
		return len(b.bands) - 1
	}
	return k
}

// Inside: even-odd point-in-polygon over the segments.
func (b *SegBands) Inside(p Vec2) bool {
	in := false
	for _, si := range b.bands[b.band(p.Y)] {
		s := b.segs[si]
		a, c := b.pts[s[0]], b.pts[s[1]]
		if (a.Y > p.Y) != (c.Y > p.Y) {
			x := a.X + (p.Y-a.Y)*(c.X-a.X)/(c.Y-a.Y)
			if x > p.X {
				in = !in
			}
		}
	}
	return in
}

// orient > 0 when a, b, c turn counter-clockwise.
func orient(a, b, c Vec2) float64 { return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X) }

type cdtTri struct {
	v [3]int
	// nb[k] is the triangle across the edge v[k]→v[k+1] (-1: none)
	nb   [3]int
	dead bool
}

type triangulation struct {
	pts  []Vec2
	tris []cdtTri
	// vt[i] is a triangle holding vertex i (the last one made with it)
	vt []int
	// mark[id] == stamp: triangle id is in the cavity being grown
	mark []int
	// per vertex, while a fan is linked: the new triangle leaving it, the
	// one coming into it, and which of those are set this time
	vfrom, vto, vmark []int
	vstamp            int
	// scratch kept between inserts
	bufIDs, bufStack, bufNews []int
	bufBnd                    [][3]int
	stamp                     int
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
	t := &triangulation{pts: all, vt: make([]int, len(all)), tris: make([]cdtTri, 0, 4*len(all))}
	for i := range t.vt {
		t.vt[i] = -1
	}
	n := len(pts)
	t.add([3]int{n, n + 1, n + 2})
	return t
}

// add makes a triangle (counter-clockwise), not yet linked to its
// neighbours (link does that).
func (t *triangulation) add(v [3]int) int {
	if orient(t.pts[v[0]], t.pts[v[1]], t.pts[v[2]]) < 0 {
		v[1], v[2] = v[2], v[1]
	}
	id := len(t.tris)
	t.tris = append(t.tris, cdtTri{v: v, nb: [3]int{-1, -1, -1}})
	for i := 0; i < 3; i++ {
		t.vt[v[i]] = id
	}
	return id
}

func (t *triangulation) kill(id int) { t.tris[id].dead = true }

// live is whether id is a triangle still in the triangulation.
func (t *triangulation) live(id int) bool { return id >= 0 && !t.tris[id].dead }

// link joins new triangles to each other and to the live triangles round
// them (outers) wherever two of them share an edge the opposite way.
func (t *triangulation) link(news, outers []int) {
	match := func(x, k int, ys []int) bool {
		u, w := t.tris[x].v[k], t.tris[x].v[(k+1)%3]
		for _, y := range ys {
			if y == x || !t.live(y) {
				continue
			}
			v := t.tris[y].v
			for j := 0; j < 3; j++ {
				if v[j] == w && v[(j+1)%3] == u {
					t.tris[x].nb[k] = y
					t.tris[y].nb[j] = x
					return true
				}
			}
		}
		return false
	}
	for _, x := range news {
		for k := 0; k < 3; k++ {
			if !match(x, k, news) && !match(x, k, outers) {
				t.tris[x].nb[k] = -1
			}
		}
	}
}

// edge is the triangle holding the directed edge a→b, found by going round
// a's triangles.
func (t *triangulation) edge(a, b int) (int, bool) {
	start := t.vt[a]
	if !t.live(start) || !holds(t.tris[start].v, a) {
		for id := len(t.tris) - 1; id >= 0; id-- {
			if t.live(id) && holds(t.tris[id].v, a) {
				start = id
				t.vt[a] = id
				break
			}
		}
		if !t.live(start) {
			return -1, false
		}
	}
	at := func(id int) int {
		v := t.tris[id].v
		for k := 0; k < 3; k++ {
			if v[k] == a {
				return k
			}
		}
		return -1
	}
	// one way round (across the edge coming into a), then, if a lies on
	// the hull, the other way
	for dir := 0; dir < 2; dir++ {
		id := start
		for n := 0; n < 1024; n++ {
			k := at(id)
			if k < 0 {
				break
			}
			if t.tris[id].v[(k+1)%3] == b {
				return id, true
			}
			var nb int
			if dir == 0 {
				nb = t.tris[id].nb[(k+2)%3]
			} else {
				nb = t.tris[id].nb[k]
			}
			if !t.live(nb) {
				break
			}
			if nb == start {
				return -1, false
			}
			id = nb
		}
	}
	return -1, false
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
	if len(t.mark) < len(t.tris) {
		t.mark = append(t.mark, make([]int, len(t.tris)-len(t.mark)+len(t.tris)/2)...)
	}
	t.stamp++
	in := func(id int) bool { return t.mark[id] == t.stamp }
	t.mark[start] = t.stamp
	ids := append(t.bufIDs[:0], start)
	stack := append(t.bufStack[:0], start)
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for k := 0; k < 3; k++ {
			nb := t.tris[id].nb[k]
			ok := t.live(nb)
			if ok && !in(nb) && t.inCircle(nb, p) {
				t.mark[nb] = t.stamp
				ids = append(ids, nb)
				stack = append(stack, nb)
			}
		}
	}
	sort.Ints(ids)
	// The cavity must be star-shaped from p: rounding on near co-circular
	// points can take in a triangle p can't see the far side of. Drop such
	// triangles (or, when p sits on an edge of the first one, take in its
	// neighbour) until every boundary edge faces p.
	boundary := t.bufBnd[:0] // a, b, the triangle beyond a→b (-1: none)
	for pass := 0; ; pass++ {
		boundary = boundary[:0]
		bad := -1
		across := -1
		for _, id := range ids {
			v := t.tris[id].v
			for k := 0; k < 3; k++ {
				a, b := v[k], v[(k+1)%3]
				nb := t.tris[id].nb[k]
				ok := t.live(nb)
				if ok && in(nb) {
					continue
				}
				out := -1
				if ok {
					out = nb
				}
				boundary = append(boundary, [3]int{a, b, out})
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
			t.mark[bad] = 0
			j := sort.SearchInts(ids, bad)
			ids = append(ids[:j], ids[j+1:]...)
		} else if across >= 0 {
			t.mark[across] = t.stamp
			j := sort.SearchInts(ids, across)
			ids = append(ids, 0)
			copy(ids[j+1:], ids[j:])
			ids[j] = across
		} else {
			break
		}
	}
	for _, id := range ids {
		t.kill(id)
	}
	t.fan(i, boundary)
	t.bufIDs, t.bufStack, t.bufBnd = ids, stack, boundary
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
	if _, ok := t.edge(a, b); ok {
		return
	}
	if _, ok := t.edge(b, a); ok {
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
		t1, ok1 := t.edge(k[0], k[1])
		t2, ok2 := t.edge(k[1], k[0])
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
		nb, has := t.edge(a, y)
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
		nb, has := t.edge(y, x)
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
	if _, ok := t.edge(a, b); ok {
		return
	}
	if _, ok := t.edge(b, a); ok {
		return
	}
	pa, pb := t.pts[a], t.pts[b]
	for iter := 0; iter < 10000; iter++ {
		// collect crossing edges
		var crossing [][2]int
		for _, tr := range t.tris {
			if tr.dead {
				continue
			}
			for j := 0; j < 3; j++ {
				k := [2]int{tr.v[j], tr.v[(j+1)%3]}
				if k[0] > k[1] || !t.live(tr.nb[j]) {
					continue
				}
				if k[0] == a || k[0] == b || k[1] == a || k[1] == b {
					continue
				}
				if segsCross(pa, pb, t.pts[k[0]], t.pts[k[1]]) {
					crossing = append(crossing, k)
				}
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
	t1, ok1 := t.edge(u, v)
	t2, ok2 := t.edge(v, u)
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
	var outers []int
	for _, x := range []int{t1, t2} {
		for _, o := range t.tris[x].nb {
			if o != t1 && o != t2 && t.live(o) {
				outers = append(outers, o)
			}
		}
	}
	t.kill(t1)
	t.kill(t2)
	t.link([]int{t.add([3]int{p, q, v}), t.add([3]int{q, p, u})}, outers)
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
				nb := t.tris[id].nb[k]
				if !t.live(nb) {
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

// fan fills a cavity with triangles (a, b, i) over its boundary edges and
// links them: each to the triangle beyond its boundary edge and to its two
// neighbours in the fan. A fan that came out odd (a boundary edge facing
// away from i, a vertex twice) is linked the slow general way.
func (t *triangulation) fan(i int, boundary [][3]int) {
	if len(t.vfrom) < len(t.pts) {
		t.vfrom = make([]int, len(t.pts))
		t.vto = make([]int, len(t.pts))
		t.vmark = make([]int, len(t.pts))
	}
	t.vstamp++
	s := t.vstamp
	news := t.bufNews[:0]
	defer func() { t.bufNews = news }()
	odd := false
	for _, e := range boundary {
		a, b := e[0], e[1]
		if a == i || b == i {
			continue
		}
		id := t.add([3]int{a, b, i})
		news = append(news, id)
		if t.tris[id].v != [3]int{a, b, i} || t.vmark[a]&1 == 1 && t.vmark[a]>>2 == s {
			odd = true
		}
		// vmark: stamp << 2 | 1 (vfrom set) | 2 (vto set)
		if t.vmark[a]>>2 != s {
			t.vmark[a] = s << 2
		}
		if t.vmark[b]>>2 != s {
			t.vmark[b] = s << 2
		}
		if t.vmark[b]&2 == 2 {
			odd = true
		}
		t.vfrom[a], t.vmark[a] = id, t.vmark[a]|1
		t.vto[b], t.vmark[b] = id, t.vmark[b]|2
	}
	if odd {
		var outers []int
		for _, e := range boundary {
			if e[2] >= 0 {
				outers = append(outers, e[2])
			}
		}
		t.link(news, outers)
		return
	}
	n := 0
	for _, e := range boundary {
		a, b, o := e[0], e[1], e[2]
		if a == i || b == i {
			continue
		}
		id := news[n]
		n++
		tr := &t.tris[id]
		tr.nb = [3]int{-1, -1, -1}
		if t.live(o) {
			ov := t.tris[o].v
			for j := 0; j < 3; j++ {
				if ov[j] == b && ov[(j+1)%3] == a {
					tr.nb[0], t.tris[o].nb[j] = o, id
				}
			}
		}
		if t.vmark[b]&1 == 1 {
			tr.nb[1] = t.vfrom[b]
		}
		if t.vmark[a]&2 == 2 {
			tr.nb[2] = t.vto[a]
		}
	}
}
