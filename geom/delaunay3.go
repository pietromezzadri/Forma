package geom

import (
	"math"
	"sort"
)

// Delaunay3 tetrahedralises points (Bowyer–Watson): every tetrahedron's
// circumsphere holds no other point. Points are inserted in Morton order
// (near ones one after another, so finding where each goes is a short
// walk). Tetrahedra come back positively oriented as index quadruples;
// points that couldn't be placed (duplicates) are in none.
func Delaunay3(pts []Vec3) [][4]int {
	if len(pts) < 4 {
		return nil
	}
	t := newTet3(pts)
	for _, i := range mortonOrder(pts) {
		t.insert(i)
	}
	n := len(pts)
	var out [][4]int
	for _, tt := range t.tets {
		if tt.dead || tt.v[0] >= n || tt.v[1] >= n || tt.v[2] >= n || tt.v[3] >= n {
			continue
		}
		out = append(out, tt.v)
	}
	return out
}

type tet3 struct {
	v    [4]int
	nb   [4]int // nb[i]: the tetrahedron across the face opposite v[i]
	dead bool
}

type tetMesh3 struct {
	p     []Vec3
	tets  []tet3
	mark  []int
	stamp int
	last  int
	scale float64
}

func newTet3(pts []Vec3) *tetMesh3 {
	var box Box
	for _, p := range pts {
		box.Add(p)
	}
	c := box.Center()
	r := box.Diagonal()*100000 + 1
	all := append(append([]Vec3(nil), pts...),
		Vec3{c.X - r, c.Y - r, c.Z - r}, Vec3{c.X + 3*r, c.Y - r, c.Z - r},
		Vec3{c.X - r, c.Y + 3*r, c.Z - r}, Vec3{c.X - r, c.Y - r, c.Z + 3*r})
	t := &tetMesh3{p: all, scale: box.Diagonal() + 1}
	n := len(pts)
	t.add([4]int{n, n + 1, n + 2, n + 3})
	return t
}

// orient3 > 0 when d lies on the side of plane abc that (b−a)×(c−a) points
// to (a, b, c, d positively oriented).
func orient3(a, b, c, d Vec3) float64 {
	return b.Sub(a).Cross(c.Sub(a)).Dot(d.Sub(a))
}

// tetFace are the vertex indices of a positively oriented tetrahedron's
// face opposite vertex i, ordered so the face faces out (vertex i on its
// negative side).
var tetFace = [4][3]int{{1, 2, 3}, {0, 3, 2}, {0, 1, 3}, {0, 2, 1}}

func (t *tetMesh3) add(v [4]int) int {
	if orient3(t.p[v[0]], t.p[v[1]], t.p[v[2]], t.p[v[3]]) < 0 {
		v[0], v[1] = v[1], v[0]
	}
	t.tets = append(t.tets, tet3{v: v, nb: [4]int{-1, -1, -1, -1}})
	return len(t.tets) - 1
}

func (t *tetMesh3) live(id int) bool { return id >= 0 && !t.tets[id].dead }

// inSphere: p inside the circumsphere of tetrahedron id.
func (t *tetMesh3) inSphere(id int, p Vec3) bool {
	v := t.tets[id].v
	a, b, c, d := t.p[v[0]].Sub(p), t.p[v[1]].Sub(p), t.p[v[2]].Sub(p), t.p[v[3]].Sub(p)
	la, lb, lc, ld := a.Dot(a), b.Dot(b), c.Dot(c), d.Dot(d)
	det := la*det3(b, c, d) - lb*det3(a, c, d) + lc*det3(a, b, d) - ld*det3(a, b, c)
	// positively oriented (orient3 > 0): inside when det > 0
	s := t.scale
	return det > 1e-14*s*s*s*s*s
}

func det3(a, b, c Vec3) float64 { return a.Dot(b.Cross(c)) }

// outside > 0: p lies beyond face f of tetrahedron id (away from it).
func (t *tetMesh3) outside(id, f int, p Vec3) float64 {
	v := t.tets[id].v
	fi := tetFace[f]
	return orient3(t.p[v[fi[0]]], t.p[v[fi[1]]], t.p[v[fi[2]]], p)
}

// locate walks from the last tetrahedron toward p.
func (t *tetMesh3) locate(p Vec3) int {
	id := t.last
	if !t.live(id) {
		for k := len(t.tets) - 1; k >= 0; k-- {
			if t.live(k) {
				id = k
				break
			}
		}
	}
	for steps := 0; steps < 4*len(t.tets)+16; steps++ {
		moved := false
		for f := 0; f < 4; f++ {
			// p beyond face f (on the far side from the tetrahedron)
			if t.outside(id, f, p) > 0 {
				nb := t.tets[id].nb[f]
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

type cavFace struct {
	v   [3]int // positively facing the inserted point
	out int    // the tetrahedron beyond (-1: none)
}

func (t *tetMesh3) insert(i int) {
	p := t.p[i]
	start := t.locate(p)
	if start < 0 {
		for k := len(t.tets) - 1; k >= 0 && start < 0; k-- {
			if !t.live(k) {
				continue
			}
			in := true
			for f := 0; f < 4 && in; f++ {
				in = t.outside(k, f, p) <= 0
			}
			if in {
				start = k
			}
		}
		if start < 0 {
			return
		}
	}
	for _, x := range t.tets[start].v {
		if t.p[x].Dist(p) < 1e-12*t.scale {
			return // a duplicate
		}
	}
	if len(t.mark) < len(t.tets) {
		t.mark = append(t.mark, make([]int, len(t.tets)-len(t.mark)+len(t.tets)/2+16)...)
	}
	t.stamp++
	in := func(id int) bool { return t.mark[id] == t.stamp }
	t.mark[start] = t.stamp
	cav := []int{start}
	for k := 0; k < len(cav); k++ {
		for _, nb := range t.tets[cav[k]].nb {
			if t.live(nb) && !in(nb) && t.inSphere(nb, p) {
				t.mark[nb] = t.stamp
				cav = append(cav, nb)
			}
		}
	}
	// the cavity must be star-shaped from p: drop tetrahedra whose outer
	// faces p can't see (rounding near co-spherical points)
	var faces []cavFace
	for pass := 0; pass < 64; pass++ {
		faces = faces[:0]
		bad := -1
		for _, id := range cav {
			if !in(id) {
				continue
			}
			for f := 0; f < 4; f++ {
				nb := t.tets[id].nb[f]
				if t.live(nb) && in(nb) {
					continue
				}
				v := t.tets[id].v
				fi := tetFace[f]
				// the face seen from inside the cavity: p must be on the
				// tetrahedron's side of it
				if t.outside(id, f, p) >= 0 && id != start && bad < 0 {
					bad = id
				}
				faces = append(faces, cavFace{v: [3]int{v[fi[0]], v[fi[1]], v[fi[2]]}, out: nb})
			}
		}
		if bad < 0 {
			break
		}
		t.mark[bad] = 0
	}
	var keep []int
	for _, id := range cav {
		if in(id) {
			t.tets[id].dead = true
			keep = append(keep, id)
		}
	}
	// new tetrahedra: each boundary face with p; linked to the tetrahedron
	// beyond the face and to each other across their shared edges
	type half struct{ tet, face int }
	edges := make(map[[2]int]half, len(faces)*3)
	var news []int
	for _, cf := range faces {
		// tet (a, b, c, p) with p on the positive side: face (a, c, b)
		// order puts... use add() to orient, then find faces by vertex
		id := t.add([4]int{cf.v[0], cf.v[1], cf.v[2], i})
		news = append(news, id)
		tt := &t.tets[id]
		// the face opposite p links to the outer tetrahedron
		pi := 0
		for k := 0; k < 4; k++ {
			if tt.v[k] == i {
				pi = k
			}
		}
		tt.nb[pi] = cf.out
		if t.live(cf.out) {
			ov := t.tets[cf.out].v
			for f := 0; f < 4; f++ {
				fi := tetFace[f]
				if sameTri(ov[fi[0]], ov[fi[1]], ov[fi[2]], cf.v) {
					t.tets[cf.out].nb[f] = id
				}
			}
		}
		// the other three faces hold p and an edge of the boundary face
		for f := 0; f < 4; f++ {
			if f == pi {
				continue
			}
			var e [2]int
			k := 0
			for _, x := range tetFace[f] {
				if tt.v[x] != i {
					e[k] = tt.v[x]
					k++
				}
			}
			if e[0] > e[1] {
				e[0], e[1] = e[1], e[0]
			}
			if o, ok := edges[e]; ok {
				t.tets[id].nb[f] = o.tet
				t.tets[o.tet].nb[o.face] = id
				delete(edges, e)
			} else {
				edges[e] = half{id, f}
			}
		}
	}
	if len(news) > 0 {
		t.last = news[0]
	}
}

func sameTri(a, b, c int, v [3]int) bool {
	x := [3]int{a, b, c}
	y := v
	sort.Ints(x[:])
	sort.Ints(y[:])
	return x == y
}

// mortonOrder sorts points along a Z-order curve on a 2^10 grid.
func mortonOrder(pts []Vec3) []int {
	var box Box
	for _, p := range pts {
		box.Add(p)
	}
	d := box.Max.Sub(box.Min)
	sz := math.Max(d.X, math.Max(d.Y, d.Z))
	if sz <= 0 {
		sz = 1
	}
	spread := func(x uint64) uint64 {
		x &= 0x3ff
		x = (x | x<<16) & 0x30000ff
		x = (x | x<<8) & 0x300f00f
		x = (x | x<<4) & 0x30c30c3
		x = (x | x<<2) & 0x9249249
		return x
	}
	keys := make([]uint64, len(pts))
	idx := make([]int, len(pts))
	for i, p := range pts {
		q := p.Sub(box.Min).Scale(1023 / sz)
		keys[i] = spread(uint64(q.X)) | spread(uint64(q.Y))<<1 | spread(uint64(q.Z))<<2
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return keys[idx[a]] < keys[idx[b]] })
	return idx
}
