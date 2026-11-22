package brep

import (
	"math"
	"sort"
)

// Sharp reports whether the faces either side of e meet in a crease: not a
// seam (one face both sides), not a tangent edge (where a round meets the
// face beside it), so a round or a chamfer can go there.
func Sharp(e *Edge) bool {
	if e.HE[0] == nil || e.HE[1] == nil {
		return false
	}
	fa, fb := e.HE[0].Loop.Face, e.HE[1].Loop.Face
	if fa == fb {
		return false
	}
	p := e.Curve.Eval((e.T0 + e.T1) / 2)
	ua, va := fa.Surf.Project(p)
	ub, vb := fb.Surf.Project(p)
	return fa.OutwardNormal(ua, va).Norm().Dot(fb.OutwardNormal(ub, vb).Norm()) < 1-2e-4
}

// TangentChain is e0 and the sharp edges running on from it smoothly at
// either end, and from those, and so on: the rim round a rounded slot, a
// filleted plate's top outline.
func TangentChain(b *Body, e0 *Edge) []*Edge {
	at := map[*Vertex][]*Edge{}
	for _, e := range b.Edges() {
		if !Sharp(e) {
			continue
		}
		for _, v := range e.Verts {
			at[v] = append(at[v], e)
		}
	}
	tangent := func(e *Edge, v *Vertex) (t0, t1 float64) {
		return e.T0, e.T1
	}
	dir := func(e *Edge, v *Vertex) []float64 {
		// the tangent at each end of e lying at v (a closed edge: both)
		t0, t1 := tangent(e, v)
		var out []float64
		if e.Verts[0] == v {
			out = append(out, t0)
		}
		if e.Verts[1] == v {
			out = append(out, t1)
		}
		return out
	}
	cos := math.Cos(2 * math.Pi / 180)
	in := map[*Edge]bool{e0: true}
	queue := []*Edge{e0}
	for len(queue) > 0 {
		e := queue[0]
		queue = queue[1:]
		for _, v := range e.Verts {
			for _, te := range dir(e, v) {
				de := e.Curve.Deriv(te).Norm()
				for _, o := range at[v] {
					if in[o] {
						continue
					}
					for _, to := range dir(o, v) {
						if math.Abs(de.Dot(o.Curve.Deriv(to).Norm())) > cos {
							in[o] = true
							queue = append(queue, o)
							break
						}
					}
				}
			}
		}
	}
	out := make([]*Edge, 0, len(in))
	for e := range in {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
