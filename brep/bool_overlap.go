package brep

import (
	"math"

	"forma/geom"
)

// Edges of the two bodies that run along each other (two boxes standing
// side by side, a part joined to its mirror image at its own face) are one
// edge in the result: each is split where the other ends, and the pieces
// that lie on each other are sewn as one (aliasPieces), not imprinted.

// onEdge: q lies on e's curve within its range.
func (c *bctx) onEdge(e *Edge, q geom.Vec3) bool {
	t := e.Curve.Project(q)
	if e.Curve.Eval(t).Dist(q) > c.weldLoose {
		return false
	}
	if geom.CurvePeriod(e.Curve) > 0 {
		lo, hi := math.Min(e.T0, e.T1), math.Max(e.T0, e.T1)
		if hi-lo > 2*math.Pi-1e-9 {
			return true
		}
		for _, k := range []float64{-2, -1, 0, 1, 2} {
			tt := t + k*2*math.Pi
			if tt >= lo-1e-9 && tt <= hi+1e-9 {
				return true
			}
		}
		return false
	}
	lo, hi := math.Min(e.T0, e.T1), math.Max(e.T0, e.T1)
	return t >= lo-1e-9 && t <= hi+1e-9
}

// paramOn is q's parameter on e, in e's own range.
func paramOn(e *Edge, q geom.Vec3) float64 {
	t := e.Curve.Project(q)
	if geom.CurvePeriod(e.Curve) > 0 {
		lo, hi := math.Min(e.T0, e.T1), math.Max(e.T0, e.T1)
		for t < lo-1e-9 {
			t += 2 * math.Pi
		}
		for t > hi+1e-9 {
			t -= 2 * math.Pi
		}
	}
	return clampT(e, t)
}

// findOverlaps records which edges of a and b run along each other and
// splits each at the ends of the other.
func (c *bctx) findOverlaps() {
	c.overlap = map[*Edge][]*Edge{}
	ea, eb := c.a.Edges(), c.b.Edges()
	boxes := map[*Edge]geom.Box{}
	for _, list := range [][]*Edge{ea, eb} {
		for _, e := range list {
			var bx geom.Box
			for _, p := range c.samples[e].pts {
				bx.Add(p)
			}
			boxes[e] = bx
		}
	}
	for _, x := range ea {
		for _, y := range eb {
			if !boxesOverlap(boxes[x], boxes[y], c.weldLoose) {
				continue
			}
			if !c.runsAlong(x, y) && !c.runsAlong(y, x) {
				continue
			}
			c.overlap[x] = append(c.overlap[x], y)
			c.overlap[y] = append(c.overlap[y], x)
			for _, pair := range [][2]*Edge{{x, y}, {y, x}} {
				e, o := pair[0], pair[1]
				for _, v := range o.Verts {
					if c.onEdge(e, v.P) && !isEnd([]geom.Vec3{e.Verts[0].P, e.Verts[1].P}, v.P, c.weldLoose) {
						c.weld(v.P, 2, c.weldExact)
						c.splits[e] = append(c.splits[e], paramOn(e, v.P))
					}
				}
			}
		}
	}
}

// runsAlong: some stretch of y (two neighbouring samples and the middle
// between them) lies on x.
func (c *bctx) runsAlong(x, y *Edge) bool {
	pts := c.samples[y].pts
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		if a.Dist(b) < c.weldLoose {
			continue
		}
		if c.onEdge(x, a) && c.onEdge(x, b) && c.onEdge(x, a.Lerp(b, 0.5)) {
			return true
		}
	}
	return false
}

// overlapsFace: e runs along one of f's edges.
func (c *bctx) overlapsFace(e *Edge, f *Face) bool {
	for _, o := range c.overlap[e] {
		for _, fe := range faceEdges(f) {
			if fe == o {
				return true
			}
		}
	}
	return false
}

type pieceAlias struct {
	to   *gpiece
	same bool // runs the same way
}

// aliasPieces matches each piece of a b edge lying on a piece of an a edge.
func (c *bctx) aliasPieces() {
	c.alias = map[pkey]pieceAlias{}
	for _, eb := range c.b.Edges() {
		if len(c.overlap[eb]) == 0 {
			continue
		}
		for _, pb := range c.edgePieces(eb) {
			mid := pb.pts[len(pb.pts)/2]
			if len(pb.pts) == 2 {
				mid = pb.pts[0].Lerp(pb.pts[1], 0.5)
			}
			for _, ea := range c.overlap[eb] {
				for _, pa := range c.edgePieces(ea) {
					same := pa.va == pb.va && pa.vb == pb.vb
					opp := pa.va == pb.vb && pa.vb == pb.va
					if !same && !opp {
						continue
					}
					if !nearPolyline(pa.pts, mid, c.weldLoose*2) {
						continue
					}
					if same && opp { // a closed piece: compare the way they set off
						ta := pa.pts[1].Sub(pa.pts[0])
						tb := pb.pts[1].Sub(pb.pts[0])
						same = ta.Dot(tb) > 0
					}
					c.alias[pb.key] = pieceAlias{to: pa, same: same}
				}
			}
		}
	}
}

func nearPolyline(pts []geom.Vec3, q geom.Vec3, tol float64) bool {
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		ab := b.Sub(a)
		l2 := ab.Dot(ab)
		s := 0.0
		if l2 > 0 {
			s = math.Max(0, math.Min(1, q.Sub(a).Dot(ab)/l2))
		}
		if a.Add(ab.Scale(s)).Dist(q) <= tol {
			return true
		}
	}
	return false
}

// boundaryEvent is the point of F's boundary at q (uv) / p, as an event on
// the boundary edge there (nil if none is within reach).
func (c *bctx) boundaryEvent(F *bface, q geom.Vec2, p geom.Vec3) *bevent {
	best, bd := -1, math.Inf(1)
	var br float64
	for i, s := range F.segs {
		ab := s.b.Sub(s.a)
		r := 0.0
		if l2 := ab.Dot(ab); l2 > 0 {
			r = math.Max(0, math.Min(1, q.Sub(s.a).Dot(ab)/l2))
		}
		if d := s.a.Add(ab.Scale(r)).Dist(q); d < bd {
			best, bd, br = i, d, r
		}
	}
	if best < 0 || F.segs[best].e == nil {
		return nil
	}
	s := F.segs[best]
	e := s.e
	t := clampT(e, e.Curve.Project(p))
	if geom.CurvePeriod(e.Curve) > 0 {
		t = s.ta + (s.tb-s.ta)*br
		t = paramOn(e, e.Curve.Eval(t))
		t = clampT(e, t+wrapNear(e.Curve.Project(p)-t))
	}
	return &bevent{e: e, t: t, p: e.Curve.Eval(t)}
}

// wrapNear is an angle difference brought into (−π, π].
func wrapNear(d float64) float64 {
	for d > math.Pi {
		d -= 2 * math.Pi
	}
	for d <= -math.Pi {
		d += 2 * math.Pi
	}
	return d
}

// onBoundary: p lies on one of F's edges.
func (c *bctx) onBoundary(F *bface, p geom.Vec3) bool {
	for _, e := range faceEdges(F.f) {
		if c.onEdge(e, p) {
			return true
		}
	}
	return false
}

// isSeam: an edge with the same face on both sides (or none: a pole gap).
func isSeam(e *Edge) bool {
	return e == nil || e.HE[0] == nil || e.HE[1] == nil || e.HE[0].Loop.Face == e.HE[1].Loop.Face
}

// splitPlanes: for two cylinders of the same radius whose axes cross, the
// two planes their intersection lies in (through the crossing, bisecting
// the axes); nil otherwise.
func splitPlanes(a, b geom.Surface) []geom.Plane {
	ca, ok1 := a.(geom.Cylinder)
	cb, ok2 := b.(geom.Cylinder)
	if !ok1 || !ok2 || math.Abs(ca.R-cb.R) > 1e-9*(1+ca.R) {
		return nil
	}
	da, db := ca.F.Z, cb.F.Z
	n := da.Cross(db)
	if n.Len() < 1e-6 {
		return nil // parallel
	}
	// the axes must meet: the gap between them along n
	if gap := math.Abs(cb.F.O.Sub(ca.F.O).Dot(n.Norm())); gap > 1e-7*(1+ca.R) {
		return nil
	}
	// the crossing point: closest points of the two lines
	w := ca.F.O.Sub(cb.F.O)
	A, B, C := da.Dot(da), da.Dot(db), db.Dot(db)
	D, E := da.Dot(w), db.Dot(w)
	den := A*C - B*B
	s := (B*E - C*D) / den
	p := ca.F.O.Add(da.Scale(s))
	// a point on both cylinders satisfies |x × da| = |x × db| (x from p), so
	// (x·da)² = (x·db)²: x·(da − db) = 0 or x·(da + db) = 0
	var out []geom.Plane
	for _, nn := range []geom.Vec3{da.Sub(db), da.Add(db)} {
		out = append(out, geom.Plane{F: geom.FrameZ(p, nn, nn.AnyPerp())})
	}
	return out
}
