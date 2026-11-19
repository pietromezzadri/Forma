package brep

import (
	"fmt"
	"math"

	"forma/geom"
)

// Weld merges mesh positions closer than tol, returning one index per
// mesh position and the merged points.
func (m *Mesh) Weld(tol float64) ([]int, []geom.Vec3) {
	idx := make([]int, len(m.Pos))
	var out []geom.Vec3
	cell := map[[3]int64][]int{}
	key := func(p geom.Vec3) [3]int64 {
		return [3]int64{int64(math.Floor(p.X / tol)), int64(math.Floor(p.Y / tol)), int64(math.Floor(p.Z / tol))}
	}
	for i, p := range m.Pos {
		k := key(p)
		found := -1
		for dx := int64(-1); dx <= 1 && found < 0; dx++ {
			for dy := int64(-1); dy <= 1 && found < 0; dy++ {
				for dz := int64(-1); dz <= 1 && found < 0; dz++ {
					for _, j := range cell[[3]int64{k[0] + dx, k[1] + dy, k[2] + dz}] {
						if out[j].Dist(p) <= tol {
							found = j
							break
						}
					}
				}
			}
		}
		if found < 0 {
			found = len(out)
			out = append(out, p)
			cell[k] = append(cell[k], found)
		}
		idx[i] = found
	}
	return idx, out
}

// Watertight checks that, once welded, every triangle edge is shared by
// exactly two triangles running it opposite ways (a closed, consistently
// oriented surface).
func (m *Mesh) Watertight(tol float64) error {
	idx, wp := m.Weld(tol)
	count := map[[2]int]int{}
	for _, t := range m.Tris {
		for k := 0; k < 3; k++ {
			a, b := idx[t[k]], idx[t[(k+1)%3]]
			if a == b {
				continue
			}
			count[[2]int{a, b}]++
		}
	}
	for e, n := range count {
		if n != 1 {
			return fmt.Errorf("edge %v used %d times one way", e, n)
		}
		if count[[2]int{e[1], e[0]}] != 1 {
			return fmt.Errorf("edge %v (%.4f → %.4f) has no opposite (a crack or a flipped triangle)", e, wp[e[0]], wp[e[1]])
		}
	}
	return nil
}
