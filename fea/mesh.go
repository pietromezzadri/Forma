package fea

import (
	"fmt"
	"math"
	"sort"

	"forma/brep"
	"forma/geom"
)

// Mesh is a solid filled with 10-node tetrahedra.
type Mesh struct {
	Nodes []geom.Vec3
	Tets  [][10]int
	// Bound are the boundary triangles (6 nodes each: 3 corners then the
	// mids of edges 01, 12, 20) with the B-rep face they lie on (0 if
	// they straddle faces).
	Bound []BoundTri
	Size  float64 // the element size it was made at
}

// BoundTri is a boundary triangle of the mesh.
type BoundTri struct {
	N    [6]int
	Face int
}

// AutoSize is an element size for a body: a quarter of its thinnest
// extent, no more than a thirtieth of its size.
func AutoSize(b *brep.Body) float64 {
	bb := b.Bounds()
	d := bb.Max.Sub(bb.Min)
	thin := math.Min(d.X, math.Min(d.Y, d.Z))
	return math.Max(math.Min(thin/4, bb.Diagonal()/30), bb.Diagonal()/400)
}

// MeshBody fills a solid with tetrahedra about h across: points on its
// faces (the surface mesh, with points inside flat faces too), a jittered
// lattice inside kept clear of the surface, and a layer outside so the
// body's surface isn't the hull; their Delaunay tetrahedra whose centre is
// inside the body, made quadratic, the middle of each boundary edge put
// back on the face it lies on.
func MeshBody(b *brep.Body, h float64) (*Mesh, error) {
	if h <= 0 {
		h = AutoSize(b)
	}
	bb := b.Bounds()
	diag := bb.Diagonal()
	sm := brep.Tessellate(b, brep.MeshOptions{ChordTol: math.Min(h/12, diag*1e-3), MaxEdge: h, Interior: h})
	if len(sm.Tris) == 0 {
		return nil, fmt.Errorf("the body has no faces to mesh")
	}
	inside := brep.Winding(sm)
	faces := map[int]*brep.Face{}
	for _, f := range b.Faces() {
		faces[f.ID] = f
	}

	// surface points, welded, with the faces each lies on
	var pts []geom.Vec3
	var nodeFaces [][]int
	weld := map[[3]int64]int{}
	cell := diag * 1e-7
	key := func(p geom.Vec3) [3]int64 {
		return [3]int64{int64(math.Round(p.X / cell)), int64(math.Round(p.Y / cell)), int64(math.Round(p.Z / cell))}
	}
	addFace := func(i, f int) {
		for _, g := range nodeFaces[i] {
			if g == f {
				return
			}
		}
		nodeFaces[i] = append(nodeFaces[i], f)
	}
	surfOf := make([]int, len(sm.Pos))
	for i, p := range sm.Pos {
		k := key(p)
		j, ok := weld[k]
		if !ok {
			j = len(pts)
			weld[k] = j
			pts = append(pts, p)
			nodeFaces = append(nodeFaces, nil)
		}
		surfOf[i] = j
	}
	for ti, t := range sm.Tris {
		for _, k := range t {
			addFace(surfOf[k], sm.TriFace[ti])
		}
	}

	// the surface triangles hashed by the cells their boxes touch, for
	// keeping the lattice off the surface (its points can be h apart, so
	// a lattice point between them may lie a hair above a face)
	grid := map[[3]int64][]int{}
	gk := func(p geom.Vec3) [3]int64 {
		return [3]int64{int64(math.Floor(p.X / h)), int64(math.Floor(p.Y / h)), int64(math.Floor(p.Z / h))}
	}
	for ti, t := range sm.Tris {
		var tb geom.Box
		for _, k := range t {
			tb.Add(sm.Pos[k])
		}
		lo, hi := gk(tb.Min), gk(tb.Max)
		for x := lo[0]; x <= hi[0]; x++ {
			for y := lo[1]; y <= hi[1]; y++ {
				for z := lo[2]; z <= hi[2]; z++ {
					grid[[3]int64{x, y, z}] = append(grid[[3]int64{x, y, z}], ti)
				}
			}
		}
	}
	nearSurf := func(p geom.Vec3, r float64) bool {
		k := gk(p)
		reach := int64(math.Ceil(r / h))
		for dx := -reach; dx <= reach; dx++ {
			for dy := -reach; dy <= reach; dy++ {
				for dz := -reach; dz <= reach; dz++ {
					for _, ti := range grid[[3]int64{k[0] + dx, k[1] + dy, k[2] + dz}] {
						t := sm.Tris[ti]
						if triDist(p, sm.Pos[t[0]], sm.Pos[t[1]], sm.Pos[t[2]]) < r {
							return true
						}
					}
				}
			}
		}
		return false
	}
	// a body-centred cubic lattice, jittered a little (regular points lie
	// on common spheres, which Delaunay handles badly)
	jit := func(i, j, k, c int) geom.Vec3 {
		u := func(s int) float64 {
			x := uint32(i*73856093 ^ j*19349663 ^ k*83492791 ^ c*2654435761 ^ s*97)
			x ^= x >> 13
			x *= 0x5bd1e995
			x ^= x >> 15
			return float64(x%20001)/10000 - 1
		}
		return geom.V3(u(1), u(2), u(3)).Scale(0.08 * h)
	}
	pad := 2 * h
	lo, hi := bb.Min.Sub(geom.V3(pad, pad, pad)), bb.Max.Add(geom.V3(pad, pad, pad))
	nInner := 0
	var outer []geom.Vec3
	ni := int(math.Ceil((hi.X - lo.X) / h))
	nj := int(math.Ceil((hi.Y - lo.Y) / h))
	nk := int(math.Ceil((hi.Z - lo.Z) / h))
	if ni*nj*nk > 4e6 {
		return nil, fmt.Errorf("an element size of %.3g mm makes too many elements: try a larger size", h)
	}
	for i := 0; i <= ni; i++ {
		for j := 0; j <= nj; j++ {
			for k := 0; k <= nk; k++ {
				for c := 0; c < 2; c++ {
					p := lo.Add(geom.V3(float64(i), float64(j), float64(k)).Scale(h))
					if c == 1 {
						p = p.Add(geom.V3(h/2, h/2, h/2))
					}
					p = p.Add(jit(i, j, k, c))
					in := inside(p) > 0.5
					switch {
					case in && !nearSurf(p, 0.55*h):
						pts = append(pts, p)
						nodeFaces = append(nodeFaces, nil)
						nInner++
					case !in && c == 0 && (i+j+k)%2 == 0 && !nearSurf(p, 1.2*h):
						outer = append(outer, p) // the outside layer, sparser
					}
				}
			}
		}
	}
	nBody := len(pts)
	pts = append(pts, outer...)
	tets := geom.Delaunay3(pts)

	// the tetrahedra inside the body
	var keep [][4]int
	for _, t := range tets {
		if t[0] >= nBody || t[1] >= nBody || t[2] >= nBody || t[3] >= nBody {
			continue
		}
		a, b2, c, d := pts[t[0]], pts[t[1]], pts[t[2]], pts[t[3]]
		vol := b2.Sub(a).Cross(c.Sub(a)).Dot(d.Sub(a)) / 6
		l := math.Max(a.Dist(b2), math.Max(a.Dist(c), math.Max(a.Dist(d), math.Max(b2.Dist(c), math.Max(b2.Dist(d), c.Dist(d))))))
		if vol < 1e-6*l*l*l {
			continue // flat: four points on one face
		}
		cen := a.Add(b2).Add(c).Add(d).Scale(0.25)
		if inside(cen) > 0.5 {
			keep = append(keep, t)
		}
	}
	if len(keep) == 0 {
		return nil, fmt.Errorf("no elements came out: the element size may be too big for the part")
	}

	// renumber the corners used; add the edge mid nodes
	m := &Mesh{Size: h}
	idx := map[int]int{}
	var faceOf [][]int
	node := func(i int) int {
		if j, ok := idx[i]; ok {
			return j
		}
		j := len(m.Nodes)
		idx[i] = j
		m.Nodes = append(m.Nodes, pts[i])
		faceOf = append(faceOf, nodeFaces[i])
		return j
	}
	mid := map[[2]int]int{}
	edgeNode := func(a, b int) int {
		k := [2]int{a, b}
		if a > b {
			k = [2]int{b, a}
		}
		if j, ok := mid[k]; ok {
			return j
		}
		j := len(m.Nodes)
		mid[k] = j
		m.Nodes = append(m.Nodes, m.Nodes[a].Lerp(m.Nodes[b], 0.5))
		faceOf = append(faceOf, nil)
		return j
	}
	for _, t := range keep {
		var e [10]int
		for i := 0; i < 4; i++ {
			e[i] = node(t[i])
		}
		for k, ed := range tetEdges {
			e[4+k] = edgeNode(e[ed[0]], e[ed[1]])
		}
		m.Tets = append(m.Tets, e)
	}

	// boundary triangles: faces of one tetrahedron
	type fk [3]int
	count := map[fk]int{}
	owner := map[fk][2]int{} // tet, face
	for ti, e := range m.Tets {
		for f := 0; f < 4; f++ {
			c := cornerFace(e, f)
			s := c
			sort.Ints(s[:])
			count[s]++
			owner[s] = [2]int{ti, f}
		}
	}
	var bkeys []fk
	for k, n := range count {
		if n == 1 {
			bkeys = append(bkeys, k)
		}
	}
	sort.Slice(bkeys, func(i, j int) bool {
		a, b := bkeys[i], bkeys[j]
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		if a[1] != b[1] {
			return a[1] < b[1]
		}
		return a[2] < b[2]
	})
	common := func(ns ...int) int {
		for _, f := range faceOf[ns[0]] {
			all := true
			for _, n := range ns[1:] {
				found := false
				for _, g := range faceOf[n] {
					found = found || g == f
				}
				all = all && found
			}
			if all {
				return f
			}
		}
		return 0
	}
	for _, k := range bkeys {
		o := owner[k]
		e := m.Tets[o[0]]
		c := cornerFace(e, o[1])
		bt := BoundTri{Face: common(c[0], c[1], c[2])}
		bt.N[0], bt.N[1], bt.N[2] = c[0], c[1], c[2]
		for s := 0; s < 3; s++ {
			bt.N[3+s] = mid[sortPair(c[s], c[(s+1)%3])]
		}
		m.Bound = append(m.Bound, bt)
	}
	// the middle of a boundary edge lying on a face goes onto the surface
	moved := map[int]geom.Vec3{}
	for _, bt := range m.Bound {
		for s := 0; s < 3; s++ {
			a, b2 := bt.N[s], bt.N[(s+1)%3]
			f := common(a, b2)
			if f == 0 || faces[f] == nil {
				continue
			}
			mn := bt.N[3+s]
			if _, done := moved[mn]; done {
				continue
			}
			p := m.Nodes[mn]
			u, v := faces[f].Surf.Project(p)
			q := faces[f].Surf.Eval(u, v)
			if q.Dist(p) < 0.25*m.Nodes[a].Dist(m.Nodes[b2]) {
				moved[mn] = p
				m.Nodes[mn] = q
			}
		}
	}
	// an element the snapping turned inside out gets its straight edges back
	for ti := range m.Tets {
		x := m.elemNodes(ti)
		if !positive(&x) {
			for k := 4; k < 10; k++ {
				if p, ok := moved[m.Tets[ti][k]]; ok {
					m.Nodes[m.Tets[ti][k]] = p
				}
			}
		}
	}
	for ti := range m.Tets {
		x := m.elemNodes(ti)
		if !positive(&x) {
			return nil, fmt.Errorf("element %d is inside out", ti)
		}
	}
	return m, nil
}

func sortPair(a, b int) [2]int {
	if a > b {
		return [2]int{b, a}
	}
	return [2]int{a, b}
}

// cornerFace is face f of a tetrahedron (opposite corner f) as corner node
// numbers, ordered to face out.
func cornerFace(e [10]int, f int) [3]int {
	fo := [4][3]int{{1, 2, 3}, {0, 3, 2}, {0, 1, 3}, {0, 2, 1}}[f]
	return [3]int{e[fo[0]], e[fo[1]], e[fo[2]]}
}

func (m *Mesh) elemNodes(ti int) [10]geom.Vec3 {
	var x [10]geom.Vec3
	for k, n := range m.Tets[ti] {
		x[k] = m.Nodes[n]
	}
	return x
}

// positive: the element's Jacobian is positive at its Gauss points and
// corners.
func positive(x *[10]geom.Vec3) bool {
	for _, gp := range gauss4 {
		if _, d := gradients(x, gp[0], gp[1], gp[2]); d <= 0 {
			return false
		}
	}
	for c := 0; c < 4; c++ {
		if _, d := gradients(x, nodeXi[c][0], nodeXi[c][1], nodeXi[c][2]); d <= 0 {
			return false
		}
	}
	return true
}

// Volume is the mesh's volume.
func (m *Mesh) Volume() float64 {
	v := 0.0
	for ti := range m.Tets {
		x := m.elemNodes(ti)
		for _, gp := range gauss4 {
			_, d := gradients(&x, gp[0], gp[1], gp[2])
			v += gp[3] * d
		}
	}
	return v
}

// triDist is the distance from p to triangle abc (Ericson's closest point).
func triDist(p, a, b, c geom.Vec3) float64 {
	ab, ac, ap := b.Sub(a), c.Sub(a), p.Sub(a)
	d1, d2 := ab.Dot(ap), ac.Dot(ap)
	if d1 <= 0 && d2 <= 0 {
		return p.Dist(a)
	}
	bp := p.Sub(b)
	d3, d4 := ab.Dot(bp), ac.Dot(bp)
	if d3 >= 0 && d4 <= d3 {
		return p.Dist(b)
	}
	vc := d1*d4 - d3*d2
	if vc <= 0 && d1 >= 0 && d3 <= 0 {
		return p.Dist(a.Add(ab.Scale(d1 / (d1 - d3))))
	}
	cp := p.Sub(c)
	d5, d6 := ab.Dot(cp), ac.Dot(cp)
	if d6 >= 0 && d5 <= d6 {
		return p.Dist(c)
	}
	vb := d5*d2 - d1*d6
	if vb <= 0 && d2 >= 0 && d6 <= 0 {
		return p.Dist(a.Add(ac.Scale(d2 / (d2 - d6))))
	}
	va := d3*d6 - d5*d4
	if va <= 0 && d4-d3 >= 0 && d5-d6 >= 0 {
		w := (d4 - d3) / ((d4 - d3) + (d5 - d6))
		return p.Dist(b.Add(c.Sub(b).Scale(w)))
	}
	den := 1 / (va + vb + vc)
	v, w := vb*den, vc*den
	return p.Dist(a.Add(ab.Scale(v)).Add(ac.Scale(w)))
}
