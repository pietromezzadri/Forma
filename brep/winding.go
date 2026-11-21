package brep

import (
	"math"

	"forma/geom"
)

// windTree answers winding-number queries on a closed mesh quickly (after
// Barill et al., "Fast Winding Numbers for Soups and Clouds"): triangles in
// a box tree, and a cluster far enough from the query point (more than
// windFar times its radius) taken as one dipole, its area vector at its
// area centroid; near ones summed exactly.
type windTree struct {
	m     *Mesh
	nodes []windNode
	order []int32 // triangle indices, each node a run of them
}

type windNode struct {
	c      geom.Vec3 // area-weighted centroid
	r      float64   // radius about c holding every vertex
	n      geom.Vec3 // Σ area · normal
	lo, hi int32     // the node's run in order
	kid    [2]int32  // children, -1 at a leaf
}

const (
	windLeaf = 8
	windFar  = 3.0
)

func newWindTree(m *Mesh) *windTree {
	t := &windTree{m: m, order: make([]int32, len(m.Tris))}
	cen := make([]geom.Vec3, len(m.Tris))
	for i, tr := range m.Tris {
		t.order[i] = int32(i)
		cen[i] = m.Pos[tr[0]].Add(m.Pos[tr[1]]).Add(m.Pos[tr[2]]).Scale(1.0 / 3)
	}
	if len(m.Tris) > 0 {
		t.build(0, int32(len(m.Tris)), cen)
	}
	return t
}

func (t *windTree) build(lo, hi int32, cen []geom.Vec3) int32 {
	id := int32(len(t.nodes))
	t.nodes = append(t.nodes, windNode{lo: lo, hi: hi, kid: [2]int32{-1, -1}})
	var nsum, csum geom.Vec3
	area := 0.0
	var box geom.Box
	for _, ti := range t.order[lo:hi] {
		tr := t.m.Tris[ti]
		a, b, c := t.m.Pos[tr[0]], t.m.Pos[tr[1]], t.m.Pos[tr[2]]
		an := b.Sub(a).Cross(c.Sub(a)).Scale(0.5)
		nsum = nsum.Add(an)
		w := an.Len()
		area += w
		csum = csum.Add(cen[ti].Scale(w))
		box.Add(cen[ti])
	}
	c := box.Center()
	if area > 0 {
		c = csum.Scale(1 / area)
	}
	r := 0.0
	for _, ti := range t.order[lo:hi] {
		for _, k := range t.m.Tris[ti] {
			r = math.Max(r, t.m.Pos[k].Dist(c))
		}
	}
	t.nodes[id].c, t.nodes[id].r, t.nodes[id].n = c, r, nsum
	if hi-lo <= windLeaf {
		return id
	}
	// split at the median along the boxes' longest side
	d := box.Max.Sub(box.Min)
	ax := 0
	switch {
	case d.Y >= d.X && d.Y >= d.Z:
		ax = 1
	case d.Z >= d.X && d.Z >= d.Y:
		ax = 2
	}
	mid := lo + (hi-lo)/2
	selectNth(t.order[lo:hi], int(mid-lo), func(i int32) float64 {
		p := cen[i]
		return [3]float64{p.X, p.Y, p.Z}[ax]
	})
	k0 := t.build(lo, mid, cen)
	k1 := t.build(mid, hi, cen)
	t.nodes[id].kid = [2]int32{k0, k1}
	return id
}

// at is the winding number about q: about 1 inside, 0 outside.
func (t *windTree) at(q geom.Vec3) float64 {
	if len(t.nodes) == 0 {
		return 0
	}
	return t.solid(0, q) / (4 * math.Pi)
}

func (t *windTree) solid(id int32, q geom.Vec3) float64 {
	nd := &t.nodes[id]
	d := nd.c.Sub(q)
	l := d.Len()
	if l > windFar*nd.r {
		return d.Dot(nd.n) / (l * l * l)
	}
	if nd.kid[0] < 0 {
		s := 0.0
		for _, ti := range t.order[nd.lo:nd.hi] {
			s += triSolid(t.m, t.m.Tris[ti], q)
		}
		return s
	}
	return t.solid(nd.kid[0], q) + t.solid(nd.kid[1], q)
}

// triSolid is the signed solid angle of a triangle seen from q
// (Van Oosterom and Strackee).
func triSolid(m *Mesh, t [3]int32, q geom.Vec3) float64 {
	a, b, c := m.Pos[t[0]].Sub(q), m.Pos[t[1]].Sub(q), m.Pos[t[2]].Sub(q)
	la, lb, lc := a.Len(), b.Len(), c.Len()
	num := a.Dot(b.Cross(c))
	den := la*lb*lc + a.Dot(b)*lc + b.Dot(c)*la + c.Dot(a)*lb
	return 2 * math.Atan2(num, den)
}

// selectNth reorders run so its k-th element (by key) is in place, the
// smaller ones before it and the larger after (quickselect).
func selectNth(run []int32, k int, key func(int32) float64) {
	ks := make([]float64, len(run))
	for i, x := range run {
		ks[i] = key(x)
	}
	swap := func(i, j int) {
		run[i], run[j] = run[j], run[i]
		ks[i], ks[j] = ks[j], ks[i]
	}
	lo, hi := 0, len(run)-1
	for lo < hi {
		// median of three as the pivot, moved to hi
		m := lo + (hi-lo)/2
		if ks[m] < ks[lo] {
			swap(m, lo)
		}
		if ks[hi] < ks[lo] {
			swap(hi, lo)
		}
		if ks[m] < ks[hi] {
			swap(m, hi)
		}
		// three ways: [lo, lt) below the pivot, [lt, gt] equal, (gt, hi] above
		p := ks[hi]
		lt, i, gt := lo, lo, hi
		for i <= gt {
			switch {
			case ks[i] < p:
				swap(i, lt)
				lt++
				i++
			case ks[i] > p:
				swap(i, gt)
				gt--
			default:
				i++
			}
		}
		switch {
		case k < lt:
			hi = lt - 1
		case k > gt:
			lo = gt + 1
		default:
			return
		}
	}
}
