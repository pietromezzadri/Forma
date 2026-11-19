package sketch

import (
	"fmt"
	"math"

	"forma/geom"
)

type ConKind int

const (
	Coincident    ConKind = iota // point, point
	PointOnLine                  // point, line
	PointOnCircle                // point, circle | arc
	Horizontal                   // line | point, point
	Vertical                     // line | point, point
	Parallel                     // line, line
	Perpendicular                // line, line
	Tangent                      // line, circle|arc  or  circle|arc, circle|arc
	Equal                        // line, line  or  circle|arc, circle|arc
	Concentric                   // circle|arc, circle|arc
	Midpoint                     // point, line
	Fix                          // point (Value, Value2 = x, y)
	Distance                     // point, point  or  point, line  (Value)
	HDistance                    // point, point (Value, signed b.x − a.x)
	VDistance                    // point, point
	Angle                        // line, line (Value radians, signed)
	Radius                       // circle | arc
	Diameter                     // circle | arc
	ArcRadius                    // arc (internal: end on the start's radius)
)

var conNames = [...]string{"Coincident", "On line", "On circle", "Horizontal", "Vertical", "Parallel",
	"Perpendicular", "Tangent", "Equal", "Concentric", "Midpoint", "Fix", "Distance", "Horizontal distance",
	"Vertical distance", "Angle", "Radius", "Diameter", "Arc radius"}

func (k ConKind) String() string { return conNames[k] }

// Dimensional reports a constraint that carries a value (a dimension).
func (k ConKind) Dimensional() bool {
	switch k {
	case Distance, HDistance, VDistance, Angle, Radius, Diameter:
		return true
	}
	return false
}

// Constraint ties entities E together.
type Constraint struct {
	ID       int
	Kind     ConKind
	E        []int
	Value    float64 // mm or radians
	Value2   float64
	Expr     string // the expression the value came from, if typed
	Internal bool   // made by the sketch itself (hidden)
	// Flip picks the variant of a two-way constraint (internal tangency,
	// which side of a line for a distance) fixed when it was added.
	Flip bool
	// Label placement offset for dimensions (sketch units, from the anchor).
	Offset geom.Vec2
}

func (s *Sketch) xy(id int) (geom.Vec2, int) {
	e := s.Ent(id)
	return geom.Vec2{X: s.Params[e.Par], Y: s.Params[e.Par+1]}, e.Par
}

// lineVec is a line's endpoints.
func (s *Sketch) lineEnds(id int) (a, b geom.Vec2) {
	e := s.Ent(id)
	return s.P(e.Pts[0]), s.P(e.Pts[1])
}

// params lists the parameter indices a constraint reads.
func (s *Sketch) conParams(c *Constraint) []int {
	var out []int
	seen := map[int]bool{}
	addPoint := func(id int) {
		e := s.Ent(id)
		if e != nil && e.Kind == Point && !seen[e.Par] {
			seen[e.Par] = true
			out = append(out, e.Par, e.Par+1)
		}
	}
	for _, id := range c.E {
		e := s.Ent(id)
		if e == nil {
			continue
		}
		switch e.Kind {
		case Point:
			addPoint(id)
		case Line, Arc:
			for _, p := range e.Pts {
				addPoint(p)
			}
		case Circle:
			addPoint(e.Pts[0])
			if !seen[e.Par] {
				seen[e.Par] = true
				out = append(out, e.Par)
			}
		}
	}
	return out
}

func (s *Sketch) center(id int) geom.Vec2 { return s.P(s.Ent(id).Pts[0]) }

// ends are the end points of a line or arc.
func ends(e *Entity) []int {
	switch e.Kind {
	case Line:
		return e.Pts
	case Arc:
		return e.Pts[1:]
	}
	return nil
}

// sharedEnd finds an end point two curves share (the same point, or two
// points held coincident).
func (s *Sketch) sharedEnd(a, b *Entity) (int, bool) {
	for _, p := range ends(a) {
		for _, q := range ends(b) {
			if p == q {
				return p, true
			}
		}
	}
	for _, c := range s.Cons {
		if c.Kind != Coincident {
			continue
		}
		for _, p := range ends(a) {
			for _, q := range ends(b) {
				if (c.E[0] == p && c.E[1] == q) || (c.E[0] == q && c.E[1] == p) {
					return p, true
				}
			}
		}
	}
	return 0, false
}

// residuals appends a constraint's equations (zero when satisfied).
func (s *Sketch) residuals(c *Constraint, out []float64) []float64 {
	ents := make([]*Entity, len(c.E))
	for i, id := range c.E {
		ents[i] = s.Ent(id)
		if ents[i] == nil {
			return out
		}
	}
	switch c.Kind {
	case Coincident:
		a, b := s.P(c.E[0]), s.P(c.E[1])
		return append(out, a.X-b.X, a.Y-b.Y)
	case PointOnLine:
		p := s.P(c.E[0])
		a, b := s.lineEnds(c.E[1])
		d := b.Sub(a)
		l := math.Max(d.Len(), 1e-9)
		return append(out, d.Cross(p.Sub(a))/l)
	case PointOnCircle:
		p := s.P(c.E[0])
		return append(out, p.Dist(s.center(c.E[1]))-s.Radius(ents[1]))
	case Horizontal, Vertical:
		var a, b geom.Vec2
		if ents[0].Kind == Line {
			a, b = s.lineEnds(c.E[0])
		} else {
			a, b = s.P(c.E[0]), s.P(c.E[1])
		}
		if c.Kind == Horizontal {
			return append(out, b.Y-a.Y)
		}
		return append(out, b.X-a.X)
	case Parallel, Perpendicular, Angle:
		a0, a1 := s.lineEnds(c.E[0])
		b0, b1 := s.lineEnds(c.E[1])
		u, v := a1.Sub(a0), b1.Sub(b0)
		lu, lv := math.Max(u.Len(), 1e-9), math.Max(v.Len(), 1e-9)
		switch c.Kind {
		case Parallel:
			return append(out, u.Cross(v)/(lu*lv))
		case Perpendicular:
			return append(out, u.Dot(v)/(lu*lv))
		}
		// angle from u to v
		ang := math.Atan2(u.Cross(v), u.Dot(v))
		d := ang - c.Value
		d = math.Remainder(d, 2*math.Pi)
		return append(out, d)
	case Tangent:
		// sharing an end (the usual case: a line running into an arc), the
		// distance form has no gradient at tangency; ask for the line (or the
		// other arc's radius) to be square to the radius at that end instead
		if p, ok := s.sharedEnd(ents[0], ents[1]); ok {
			pp := s.P(p)
			r1 := pp.Sub(s.center(c.E[1]))
			l1 := math.Max(r1.Len(), 1e-9)
			if ents[0].Kind == Line {
				a, b := s.lineEnds(c.E[0])
				d := b.Sub(a)
				return append(out, d.Dot(r1)/(math.Max(d.Len(), 1e-9)*l1))
			}
			r0 := pp.Sub(s.center(c.E[0]))
			return append(out, r0.Cross(r1)/(math.Max(r0.Len(), 1e-9)*l1))
		}
		if ents[0].Kind == Line {
			a, b := s.lineEnds(c.E[0])
			d := b.Sub(a)
			l := math.Max(d.Len(), 1e-9)
			cc := s.center(c.E[1])
			return append(out, math.Abs(d.Cross(cc.Sub(a))/l)-s.Radius(ents[1]))
		}
		c1, c2 := s.center(c.E[0]), s.center(c.E[1])
		r1, r2 := s.Radius(ents[0]), s.Radius(ents[1])
		if c.Flip { // inside
			return append(out, c1.Dist(c2)-math.Abs(r1-r2))
		}
		return append(out, c1.Dist(c2)-(r1+r2))
	case Equal:
		if ents[0].Kind == Line {
			a0, a1 := s.lineEnds(c.E[0])
			b0, b1 := s.lineEnds(c.E[1])
			return append(out, a1.Dist(a0)-b1.Dist(b0))
		}
		return append(out, s.Radius(ents[0])-s.Radius(ents[1]))
	case Concentric:
		a, b := s.center(c.E[0]), s.center(c.E[1])
		return append(out, a.X-b.X, a.Y-b.Y)
	case Midpoint:
		p := s.P(c.E[0])
		a, b := s.lineEnds(c.E[1])
		m := a.Add(b).Scale(0.5)
		return append(out, p.X-m.X, p.Y-m.Y)
	case Fix:
		p := s.P(c.E[0])
		return append(out, p.X-c.Value, p.Y-c.Value2)
	case Distance:
		p := s.P(c.E[0])
		if ents[1].Kind == Line {
			a, b := s.lineEnds(c.E[1])
			d := b.Sub(a)
			l := math.Max(d.Len(), 1e-9)
			return append(out, math.Abs(d.Cross(p.Sub(a))/l)-c.Value)
		}
		return append(out, p.Dist(s.P(c.E[1]))-c.Value)
	case HDistance:
		return append(out, s.P(c.E[1]).X-s.P(c.E[0]).X-c.Value)
	case VDistance:
		return append(out, s.P(c.E[1]).Y-s.P(c.E[0]).Y-c.Value)
	case Radius:
		return append(out, s.Radius(ents[0])-c.Value)
	case Diameter:
		return append(out, 2*s.Radius(ents[0])-c.Value)
	case ArcRadius:
		e := ents[0]
		cc := s.P(e.Pts[0])
		return append(out, s.P(e.Pts[2]).Dist(cc)-s.P(e.Pts[1]).Dist(cc))
	}
	return out
}

// Measure is what a dimensional constraint's value is now (mm or radians),
// for a dimension being added at the current geometry.
func (s *Sketch) Measure(c *Constraint) float64 {
	saved := c.Value
	c.Value = 0
	r := s.residuals(c, nil)
	c.Value = saved
	if len(r) == 0 {
		return 0
	}
	switch c.Kind {
	case Angle:
		return r[0] // residual with value 0 is the angle itself
	}
	return r[0]
}

// kinds of entities a constraint takes, for validation
func (s *Sketch) checkArgs(k ConKind, ids []int) error {
	kinds := make([]Kind, len(ids))
	for i, id := range ids {
		e := s.Ent(id)
		if e == nil {
			return fmt.Errorf("no entity %d", id)
		}
		kinds[i] = e.Kind
	}
	is := func(want ...Kind) bool {
		if len(want) != len(kinds) {
			return false
		}
		for i := range want {
			if want[i] != kinds[i] {
				return false
			}
		}
		return true
	}
	round := func(k Kind) bool { return k == Circle || k == Arc }
	ok := false
	switch k {
	case Coincident, HDistance, VDistance:
		ok = is(Point, Point)
	case PointOnLine, Midpoint:
		ok = is(Point, Line)
	case PointOnCircle:
		ok = len(kinds) == 2 && kinds[0] == Point && round(kinds[1])
	case Horizontal, Vertical:
		ok = is(Line) || is(Point, Point)
	case Parallel, Perpendicular, Angle:
		ok = is(Line, Line)
	case Tangent:
		ok = len(kinds) == 2 && ((kinds[0] == Line && round(kinds[1])) || (round(kinds[0]) && round(kinds[1])))
	case Equal:
		ok = is(Line, Line) || (len(kinds) == 2 && round(kinds[0]) && round(kinds[1]))
	case Concentric:
		ok = len(kinds) == 2 && round(kinds[0]) && round(kinds[1])
	case Fix:
		ok = is(Point)
	case Distance:
		ok = is(Point, Point) || is(Point, Line)
	case Radius, Diameter:
		ok = len(kinds) == 1 && round(kinds[0])
	}
	if !ok {
		return fmt.Errorf("%s doesn't take %v", k, kinds)
	}
	return nil
}
