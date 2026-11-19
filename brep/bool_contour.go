package brep

import (
	"math"
	"sort"

	"forma/geom"
)

// contour traces the zero set of g over a grid (marching squares): nx × ny
// cells of size h from (u0, v0). It returns polylines (closed ones repeat
// their first point at the end).
func contour(g func(u, v float64) float64, u0, v0, h float64, nx, ny int) [][]geom.Vec2 {
	val := make([]float64, (nx+1)*(ny+1))
	at := func(i, j int) float64 { return val[j*(nx+1)+i] }
	for j := 0; j <= ny; j++ {
		for i := 0; i <= nx; i++ {
			x := g(u0+float64(i)*h, v0+float64(j)*h)
			if x == 0 {
				x = 1e-300 // zero counts as positive
			}
			val[j*(nx+1)+i] = x
		}
	}
	// crossing points on grid edges: key = (i, j, dir) with dir 0 along u, 1 along v
	type ekey struct{ i, j, d int }
	pt := func(k ekey) geom.Vec2 {
		a := at(k.i, k.j)
		var b float64
		if k.d == 0 {
			b = at(k.i+1, k.j)
		} else {
			b = at(k.i, k.j+1)
		}
		t := a / (a - b)
		x, y := u0+float64(k.i)*h, v0+float64(k.j)*h
		if k.d == 0 {
			return geom.Vec2{X: x + t*h, Y: y}
		}
		return geom.Vec2{X: x, Y: y + t*h}
	}
	adj := map[ekey][]ekey{}
	link := func(a, b ekey) {
		adj[a] = append(adj[a], b)
		adj[b] = append(adj[b], a)
	}
	for j := 0; j < ny; j++ {
		for i := 0; i < nx; i++ {
			c0, c1, c2, c3 := at(i, j), at(i+1, j), at(i+1, j+1), at(i, j+1)
			idx := 0
			if c0 > 0 {
				idx |= 1
			}
			if c1 > 0 {
				idx |= 2
			}
			if c2 > 0 {
				idx |= 4
			}
			if c3 > 0 {
				idx |= 8
			}
			bot, right := ekey{i, j, 0}, ekey{i + 1, j, 1}
			top, left := ekey{i, j + 1, 0}, ekey{i, j, 1}
			switch idx {
			case 0, 15:
			case 1, 14:
				link(left, bot)
			case 2, 13:
				link(bot, right)
			case 3, 12:
				link(left, right)
			case 4, 11:
				link(right, top)
			case 6, 9:
				link(bot, top)
			case 7, 8:
				link(left, top)
			case 5, 10:
				center := (c0 + c1 + c2 + c3) / 4
				if (center > 0) == (idx == 5) {
					link(left, top)
					link(bot, right)
				} else {
					link(left, bot)
					link(right, top)
				}
			}
		}
	}
	used := map[ekey]bool{}
	var out [][]geom.Vec2
	walk := func(start ekey) []geom.Vec2 {
		line := []geom.Vec2{pt(start)}
		used[start] = true
		prev, cur := ekey{-1, -1, -1}, start
		for {
			var next *ekey
			for _, n := range adj[cur] {
				if n != prev && !used[n] {
					nn := n
					next = &nn
					break
				}
			}
			if next == nil {
				// closed if it comes back to the start
				for _, n := range adj[cur] {
					if n == start && n != prev && len(line) > 2 {
						line = append(line, line[0])
					}
				}
				return line
			}
			used[*next] = true
			line = append(line, pt(*next))
			prev, cur = cur, *next
		}
	}
	// open chains first (from their ends), then loops; in a fixed order
	keys := make([]ekey, 0, len(adj))
	for k := range adj {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool {
		ka, kb := keys[a], keys[b]
		if ka.j != kb.j {
			return ka.j < kb.j
		}
		if ka.i != kb.i {
			return ka.i < kb.i
		}
		return ka.d < kb.d
	})
	for _, k := range keys {
		if len(adj[k]) == 1 && !used[k] {
			out = append(out, walk(k))
		}
	}
	for _, k := range keys {
		if !used[k] {
			out = append(out, walk(k))
		}
	}
	return out
}

// windingNumber is the generalized winding number of a closed mesh about q:
// about 1 inside, 0 outside (robust to small gaps).
func windingNumber(m *Mesh, q geom.Vec3) float64 {
	s := 0.0
	for _, t := range m.Tris {
		a, b, c := m.Pos[t[0]].Sub(q), m.Pos[t[1]].Sub(q), m.Pos[t[2]].Sub(q)
		la, lb, lc := a.Len(), b.Len(), c.Len()
		num := a.Dot(b.Cross(c))
		den := la*lb*lc + a.Dot(b)*lc + b.Dot(c)*la + c.Dot(a)*lb
		s += 2 * math.Atan2(num, den)
	}
	return s / (4 * math.Pi)
}
