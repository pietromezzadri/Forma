package ops

import (
	"fmt"
	"math"

	"forma/brep"
	"forma/geom"
)

// MeshSolidMax is the most triangles a mesh may have to be made a solid
// (every triangle left over is a face, and booleans grow with the faces).
const MeshSolidMax = 60000

// MeshSolid makes a solid of a closed triangle mesh (pos, and tris as
// index triples facing out): its vertices welded, each flat region of
// triangles that lie on one plane and have one outline merged into one
// face, the others kept as triangular faces. Faces are named
// "<name>/face <n>". It fails, saying why, on a mesh that is open, has an
// edge shared by more than two triangles, or has too many triangles.
func MeshSolid(name string, pos []geom.Vec3, tris [][3]int) (*brep.Body, error) {
	if len(tris) > MeshSolidMax {
		return nil, fmt.Errorf("%d triangles: too many to be a solid (at most %d)", len(tris), MeshSolidMax)
	}
	// weld points at the same place
	var bx geom.Box
	for _, p := range pos {
		bx.Add(p)
	}
	scale := math.Max(bx.Diagonal(), 1)
	q := scale * 1e-9
	key := func(p geom.Vec3) [3]int64 {
		return [3]int64{int64(math.Round(p.X / q)), int64(math.Round(p.Y / q)), int64(math.Round(p.Z / q))}
	}
	at := map[[3]int64]int{}
	var pts []geom.Vec3
	idx := make([]int, len(pos))
	for i, p := range pos {
		k := key(p)
		j, ok := at[k]
		if !ok {
			j = len(pts)
			at[k] = j
			pts = append(pts, p)
		}
		idx[i] = j
	}
	// triangles with area, and their edges
	type tri struct {
		v [3]int
		n geom.Vec3
	}
	var ts []tri
	for _, t := range tris {
		a, b, c := idx[t[0]], idx[t[1]], idx[t[2]]
		if a == b || b == c || a == c {
			continue
		}
		n := pts[b].Sub(pts[a]).Cross(pts[c].Sub(pts[a]))
		if n.Len() < 1e-14*scale*scale {
			continue
		}
		ts = append(ts, tri{[3]int{a, b, c}, n.Norm()})
	}
	if len(ts) < 4 {
		return nil, fmt.Errorf("it has only %d triangles: not a solid", len(ts))
	}
	edge := map[[2]int]int{} // directed edge → its triangle
	for i, t := range ts {
		for k := 0; k < 3; k++ {
			e := [2]int{t.v[k], t.v[(k+1)%3]}
			if _, dup := edge[e]; dup {
				return nil, fmt.Errorf("an edge is shared by more than two triangles, or two triangles face opposite ways (near %v): not a clean solid", pts[e[0]])
			}
			edge[e] = i
		}
	}
	for e := range edge {
		if _, ok := edge[[2]int{e[1], e[0]}]; !ok {
			return nil, fmt.Errorf("it isn't closed (an open edge near %v): not a solid", pts[e[0]])
		}
	}
	// flat regions: neighbours across an edge whose points all lie on the
	// region's first triangle's plane
	tol := 5e-7 * scale / 100
	tol = math.Max(tol, 5e-8)
	region := make([]int, len(ts))
	for i := range region {
		region[i] = -1
	}
	var regions [][]int
	for s := range ts {
		if region[s] >= 0 {
			continue
		}
		r := len(regions)
		n, o := ts[s].n, pts[ts[s].v[0]]
		on := func(t tri) bool {
			if t.n.Dot(n) < 1-1e-12 {
				return false
			}
			for _, v := range t.v {
				if math.Abs(pts[v].Sub(o).Dot(n)) > tol {
					return false
				}
			}
			return true
		}
		region[s] = r
		members := []int{s}
		for k := 0; k < len(members); k++ {
			t := ts[members[k]]
			for j := 0; j < 3; j++ {
				nb, ok := edge[[2]int{t.v[(j+1)%3], t.v[j]}]
				if ok && region[nb] < 0 && on(ts[nb]) {
					region[nb] = r
					members = append(members, nb)
				}
			}
		}
		regions = append(regions, members)
	}
	// each region one face when its outline is a single loop
	var faces [][]int
	for _, members := range regions {
		if len(members) > 1 {
			if loop, ok := outline(members, func(i int) [3]int { return ts[i].v }, region, edge); ok {
				faces = append(faces, loop)
				continue
			}
		}
		for _, i := range members {
			faces = append(faces, ts[i].v[:])
		}
	}
	names := make([]string, len(faces))
	for i := range faces {
		names[i] = fmt.Sprintf("%s/face %d", name, i+1)
	}
	b, err := Polyhedron(name, pts, faces, names)
	if err != nil {
		return nil, err
	}
	b.Name = name
	if err := brep.Check(b); err != nil {
		return nil, fmt.Errorf("the mesh doesn't make a valid solid: %v", err)
	}
	return b, nil
}

// outline is the boundary of a region of triangles as one loop of
// vertices (facing as they do), or false when it is several loops (a
// region with a hole) or pinched at a vertex.
func outline(members []int, verts func(int) [3]int, region []int, edge map[[2]int]int) ([]int, bool) {
	r := region[members[0]]
	next := map[int]int{}
	for _, i := range members {
		v := verts(i)
		for k := 0; k < 3; k++ {
			a, b := v[k], v[(k+1)%3]
			if o, ok := edge[[2]int{b, a}]; ok && region[o] == r {
				continue // inside the region
			}
			if _, dup := next[a]; dup {
				return nil, false // pinched
			}
			next[a] = b
		}
	}
	if len(next) < 3 {
		return nil, false
	}
	var start int
	for v := range next {
		start = v
		break
	}
	loop := []int{start}
	for v := next[start]; v != start; v = next[v] {
		loop = append(loop, v)
		if len(loop) > len(next) {
			return nil, false
		}
	}
	if len(loop) != len(next) {
		return nil, false // more than one loop
	}
	return loop, true
}
