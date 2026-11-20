package doc

// Body edges projected into a sketch: straight edges come in as lines,
// circles and arcs square to the sketch as circles and arcs, anything else
// as a chain of lines. Their points are locked; each entity keeps the
// edge's reference, and every rebuild of the sketch projects the edge
// again, so the geometry follows the body.

import (
	"encoding/json"
	"fmt"
	"math"

	"forma/brep"
	"forma/geom"
	"forma/sketch"
)

const projChain = 24 // lines in a chain standing for a curve

// ProjectEdge projects edge e (picked near p) into sketch feature f.
func ProjectEdge(f *Feature, e *brep.Edge, p geom.Vec3) error {
	if f == nil || f.Sketch == nil {
		return fmt.Errorf("no sketch")
	}
	ref, ok := RefForEdge(e, p)
	if !ok {
		return fmt.Errorf("that edge can't be referred to")
	}
	b, _ := json.Marshal(ref)
	key := string(b)
	s := f.Sketch
	for _, x := range s.Ents {
		if x.Proj == key {
			return fmt.Errorf("that edge is projected already")
		}
	}
	pl := s.Plane
	at := func(q geom.Vec3) geom.Vec2 {
		l := pl.ToLocal(q)
		return geom.Vec2{X: l.X, Y: l.Y}
	}
	point := func(q geom.Vec2) int {
		pt := s.AddPoint(q)
		pt.Proj = key
		s.Lock(pt.ID)
		return pt.ID
	}
	switch k := e.Curve.(type) {
	case geom.Line:
		a, c := at(e.Curve.Eval(e.T0)), at(e.Curve.Eval(e.T1))
		if a.Dist(c) < 1e-9 {
			point(a) // square to the sketch: a point
			return nil
		}
		l := s.AddLine(point(a), point(c))
		l.Proj = key
		return nil
	case geom.Circle:
		if math.Abs(math.Abs(k.F.Z.Dot(pl.Z))-1) < 1e-9 {
			ctr := point(at(k.F.O))
			if math.Abs(e.T1-e.T0) > 2*math.Pi-1e-9 {
				c := s.AddCircle(ctr, k.R)
				c.Proj = key
				s.LockRadius(c.ID)
				return nil
			}
			st, en := at(e.Curve.Eval(e.T0)), at(e.Curve.Eval(e.T1))
			mid := at(e.Curve.Eval((e.T0 + e.T1) / 2))
			cc := at(k.F.O)
			// sketch arcs run counter-clockwise from start to end
			a0 := st.Sub(cc).Angle()
			if geom.WrapAngle(mid.Sub(cc).Angle()-a0) > geom.WrapAngle(en.Sub(cc).Angle()-a0) {
				st, en = en, st
			}
			arc := s.AddArc(ctr, point(st), point(en))
			arc.Proj = key
			return nil
		}
	}
	// a chain of lines along it
	var prev int
	for i := 0; i <= projChain; i++ {
		q := point(at(e.Curve.Eval(e.T0 + (e.T1-e.T0)*float64(i)/projChain)))
		if i > 0 {
			l := s.AddLine(prev, q)
			l.Proj, l.ProjK = key, i
		}
		prev = q
	}
	return nil
}

// updateProjections projects every projected edge of a sketch again, from
// the bodies as they stand before it; an edge gone leaves its geometry
// where it was and reports so.
func (d *Document) updateProjections(s *sketch.Sketch) error {
	var gone error
	pl := s.Plane
	at := func(q geom.Vec3) geom.Vec2 {
		l := pl.ToLocal(q)
		return geom.Vec2{X: l.X, Y: l.Y}
	}
	done := map[string]bool{}
	for _, x := range s.Ents {
		if x.Proj == "" || done[x.Proj] || x.Kind == sketch.Point {
			continue
		}
		done[x.Proj] = true
		var ref EdgeRef
		if json.Unmarshal([]byte(x.Proj), &ref) != nil {
			continue
		}
		e := d.FindEdgeNow(ref)
		if e == nil {
			gone = fmt.Errorf("a projected edge (between %s and %s) is gone", shortName(ref.A), shortName(ref.B))
			continue
		}
		ev := func(f float64) geom.Vec2 { return at(e.Curve.Eval(e.T0 + (e.T1-e.T0)*f)) }
		switch x.Kind {
		case sketch.Line:
			if x.ProjK == 0 {
				a, b := ev(0), ev(1)
				// keep each end with the end it was
				if s.P(x.Pts[0]).Dist(b)+s.P(x.Pts[1]).Dist(a) < s.P(x.Pts[0]).Dist(a)+s.P(x.Pts[1]).Dist(b) {
					a, b = b, a
				}
				s.MoveLocked(x.Pts[0], a)
				s.MoveLocked(x.Pts[1], b)
				continue
			}
			// a chain: every line of it
			for _, y := range s.Ents {
				if y.Proj == x.Proj && y.Kind == sketch.Line && y.ProjK > 0 {
					s.MoveLocked(y.Pts[0], ev(float64(y.ProjK-1)/projChain))
					s.MoveLocked(y.Pts[1], ev(float64(y.ProjK)/projChain))
				}
			}
		case sketch.Circle, sketch.Arc:
			c, ok := e.Curve.(geom.Circle)
			if !ok {
				gone = fmt.Errorf("a projected circle (between %s and %s) isn't one any more", shortName(ref.A), shortName(ref.B))
				continue
			}
			s.MoveLocked(x.Pts[0], at(c.F.O))
			if x.Kind == sketch.Circle {
				s.SetLockedRadius(x.ID, c.R)
				continue
			}
			a, b := ev(0), ev(1)
			if s.P(x.Pts[1]).Dist(b)+s.P(x.Pts[2]).Dist(a) < s.P(x.Pts[1]).Dist(a)+s.P(x.Pts[2]).Dist(b) {
				a, b = b, a
			}
			s.MoveLocked(x.Pts[1], a)
			s.MoveLocked(x.Pts[2], b)
		}
	}
	// lone projected points (edges square to the sketch)
	for _, x := range s.Ents {
		if x.Kind != sketch.Point || x.Proj == "" || done[x.Proj] {
			continue
		}
		var ref EdgeRef
		if json.Unmarshal([]byte(x.Proj), &ref) != nil {
			continue
		}
		if used := projUsed(s, x); used {
			continue
		}
		if e := d.FindEdgeNow(ref); e != nil {
			s.MoveLocked(x.ID, at(e.Curve.Eval(e.T0)))
		}
	}
	return gone
}

// projUsed: a projected point belongs to a projected curve (moved with it).
func projUsed(s *sketch.Sketch, p *sketch.Entity) bool {
	for _, e := range s.Ents {
		if e.Proj == "" {
			continue
		}
		for _, q := range e.Pts {
			if q == p.ID {
				return true
			}
		}
	}
	return false
}
