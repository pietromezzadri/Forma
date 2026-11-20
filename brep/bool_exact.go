package brep

// Exact intersection edges: a cut whose two surfaces meet in a line, a
// circle or an ellipse (a hole's rim, a boss's root, a plane through a
// sphere, a slanted cut through a cylinder) becomes an edge on that curve
// instead of a polyline through its points. The polyline is still what the
// faces are split along; only the edge in the result is exact.

import (
	"math"

	"forma/geom"
)

// exactCurve is the cut's curve in closed form, nil if it has none.
func (c *bctx) exactCurve(k *bcut) geom.Curve {
	if k.exactDone {
		return k.exact
	}
	k.exactDone = true
	pts := k.pts
	if k.closed && len(pts) > 1 && pts[0].Dist(pts[len(pts)-1]) < 1e-12 {
		pts = pts[:len(pts)-1]
	}
	if cv, ok := geom.ExactCurve(k.faces[0].s, k.faces[1].s, pts, k.closed, c.exactTol()); ok {
		k.exact = cv
	}
	return k.exact
}

// exactTol is how far a cut's points may lie off the curve recognised in
// them: they are on both surfaces to tolOn.
func (c *bctx) exactTol() float64 { return math.Max(c.tolOn*20, 1e-9) }

// exactEdge makes the edge of piece pc on the cut's exact curve, or reports
// that it can't (no closed form, or the vertices were welded off it).
func (c *bctx) exactEdge(b *Builder, va, vb *Vertex, pc *gpiece) (*Edge, bool) {
	if pc.cut == nil {
		return nil, false
	}
	cv := c.exactCurve(pc.cut)
	if cv == nil {
		return nil, false
	}
	tol := math.Max(c.exactTol(), 1e-7)
	for _, v := range []*Vertex{va, vb} {
		if cv.Eval(cv.Project(v.P)).Dist(v.P) > tol {
			return nil, false
		}
	}
	pts := pc.pts
	mid := pts[0].Lerp(pts[len(pts)-1], 0.5)
	if len(pts) > 2 {
		mid = pts[len(pts)/2]
	}
	full := pc.closed || va == vb
	if full {
		mid = pts[1]
		if len(pts) > 3 {
			mid = pts[len(pts)/4]
		}
	}
	if _, ok := cv.(geom.Line); ok {
		if full {
			return nil, false
		}
		return b.LineEdge(va, vb), true
	}
	if geom.CurvePeriod(cv) == 0 {
		return nil, false
	}
	// starting at va, the way that passes mid
	arc := geom.StartAt(cv, va.P, false)
	t1 := geom.ArcEnd(arc, vb.P, full)
	if tm := arc.Project(mid); full && tm > math.Pi || !full && tm > t1 {
		arc = geom.StartAt(cv, va.P, true)
		t1 = geom.ArcEnd(arc, vb.P, full)
	}
	return b.Edge(va, vb, arc, 0, t1), true
}
