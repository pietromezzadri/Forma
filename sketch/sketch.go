// Package sketch is Forma's 2D constraint sketcher: points, lines, circles
// and arcs on a plane, geometric and dimensional constraints solved by
// Levenberg–Marquardt, degrees of freedom from the Jacobian's rank, and the
// closed regions the curves bound (the profiles features use).
package sketch

import (
	"fmt"
	"math"
	"sort"

	"forma/geom"
)

type Kind int

const (
	Point Kind = iota
	Line
	Circle
	Arc
)

func (k Kind) String() string {
	return [...]string{"Point", "Line", "Circle", "Arc"}[k]
}

// Entity is a sketch element. Points own two parameters (x, y); a line
// joins two points; a circle has a centre point and a radius parameter; an
// arc has centre, start and end points and runs counter-clockwise from
// start to end.
type Entity struct {
	ID           int
	Kind         Kind
	Pts          []int // point entity IDs: line [a b], circle [c], arc [c start end]
	Par          int   // point: index of x (y is Par+1); circle: radius index
	Construction bool
}

// Sketch is a set of entities and constraints on a plane.
type Sketch struct {
	Name   string
	Plane  geom.Frame
	Params []float64
	Ents   []*Entity
	Cons   []*Constraint
	nextID int
	idx    map[int]*Entity

	// Status of the last solve.
	Status SolveStatus
}

// SolveStatus reports how the last solve went.
type SolveStatus struct {
	Converged bool
	Residual  float64
	DOF       int
	Iter      int
	// Fixed[entityID] is true when nothing about the entity can move.
	Fixed map[int]bool
}

func New(name string, plane geom.Frame) *Sketch {
	return &Sketch{Name: name, Plane: plane}
}

func (s *Sketch) id() int { s.nextID++; return s.nextID }

// Ent finds an entity by ID.
func (s *Sketch) Ent(id int) *Entity {
	if s.idx == nil || len(s.idx) != len(s.Ents) {
		s.idx = make(map[int]*Entity, len(s.Ents))
		for _, e := range s.Ents {
			s.idx[e.ID] = e
		}
	}
	return s.idx[id]
}

// Con finds a constraint by ID.
func (s *Sketch) Con(id int) *Constraint {
	for _, c := range s.Cons {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// AddPoint adds a free point.
func (s *Sketch) AddPoint(p geom.Vec2) *Entity {
	e := &Entity{ID: s.id(), Kind: Point, Par: len(s.Params)}
	s.Params = append(s.Params, p.X, p.Y)
	s.Ents = append(s.Ents, e)
	return e
}

// P is a point entity's position.
func (s *Sketch) P(id int) geom.Vec2 {
	e := s.Ent(id)
	return geom.Vec2{X: s.Params[e.Par], Y: s.Params[e.Par+1]}
}

// SetP moves a point (no solve).
func (s *Sketch) SetP(id int, p geom.Vec2) {
	e := s.Ent(id)
	s.Params[e.Par], s.Params[e.Par+1] = p.X, p.Y
}

// AddLine adds a line between two existing points.
func (s *Sketch) AddLine(a, b int) *Entity {
	e := &Entity{ID: s.id(), Kind: Line, Pts: []int{a, b}}
	s.Ents = append(s.Ents, e)
	return e
}

// AddLineXY adds a line with two new points.
func (s *Sketch) AddLineXY(a, b geom.Vec2) *Entity {
	return s.AddLine(s.AddPoint(a).ID, s.AddPoint(b).ID)
}

// AddCircle adds a circle round an existing centre point.
func (s *Sketch) AddCircle(center int, r float64) *Entity {
	e := &Entity{ID: s.id(), Kind: Circle, Pts: []int{center}, Par: len(s.Params)}
	s.Params = append(s.Params, r)
	s.Ents = append(s.Ents, e)
	return e
}

// AddArc adds an arc (counter-clockwise from start to end round center);
// end is kept on the start's radius by an internal constraint.
func (s *Sketch) AddArc(center, start, end int) *Entity {
	e := &Entity{ID: s.id(), Kind: Arc, Pts: []int{center, start, end}}
	s.Ents = append(s.Ents, e)
	s.Cons = append(s.Cons, &Constraint{ID: s.id(), Kind: ArcRadius, E: []int{e.ID}, Internal: true})
	return e
}

// Radius of a circle or arc.
func (s *Sketch) Radius(e *Entity) float64 {
	switch e.Kind {
	case Circle:
		return s.Params[e.Par]
	case Arc:
		return s.P(e.Pts[1]).Dist(s.P(e.Pts[0]))
	}
	return 0
}

// ArcAngles are an arc's start angle and its positive sweep.
func (s *Sketch) ArcAngles(e *Entity) (a0, sweep float64) {
	c := s.P(e.Pts[0])
	a0 = s.P(e.Pts[1]).Sub(c).Angle()
	a1 := s.P(e.Pts[2]).Sub(c).Angle()
	sweep = geom.WrapAngle(a1 - a0)
	if sweep < 1e-12 {
		sweep = 2 * math.Pi
	}
	return
}

// Delete removes an entity, what depends on it (lines on a deleted point,
// constraints on it) and points no longer used by anything.
func (s *Sketch) Delete(id int) {
	dead := map[int]bool{id: true}
	changed := true
	for changed {
		changed = false
		for _, e := range s.Ents {
			if dead[e.ID] {
				continue
			}
			for _, p := range e.Pts {
				if dead[p] {
					dead[e.ID], changed = true, true
					break
				}
			}
		}
	}
	// a deleted curve's own points go too when nothing else uses them
	for _, e := range s.Ents {
		if !dead[e.ID] || e.Kind == Point {
			continue
		}
		for _, p := range e.Pts {
			if !s.pointUsedOutside(p, dead) {
				dead[p] = true
			}
		}
	}
	var ents []*Entity
	for _, e := range s.Ents {
		if !dead[e.ID] {
			ents = append(ents, e)
		}
	}
	s.Ents = ents
	s.idx = nil
	var cons []*Constraint
	for _, c := range s.Cons {
		keep := true
		for _, x := range c.E {
			if dead[x] {
				keep = false
			}
		}
		if keep {
			cons = append(cons, c)
		}
	}
	s.Cons = cons
}

func (s *Sketch) pointUsedOutside(p int, dead map[int]bool) bool {
	for _, e := range s.Ents {
		if dead[e.ID] {
			continue
		}
		for _, q := range e.Pts {
			if q == p {
				return true
			}
		}
	}
	for _, c := range s.Cons {
		if c.Internal {
			continue
		}
		for _, x := range c.E {
			if x == p {
				// a constraint on it alone doesn't keep a point
				return false
			}
		}
	}
	return false
}

// DeleteConstraint removes one constraint.
func (s *Sketch) DeleteConstraint(id int) {
	for i, c := range s.Cons {
		if c.ID == id && !c.Internal {
			s.Cons = append(s.Cons[:i], s.Cons[i+1:]...)
			return
		}
	}
}

// Curves are the entities that aren't points.
func (s *Sketch) Curves() []*Entity {
	var out []*Entity
	for _, e := range s.Ents {
		if e.Kind != Point {
			out = append(out, e)
		}
	}
	return out
}

// Polyline samples a curve in sketch coordinates (chordTol in mm).
func (s *Sketch) Polyline(e *Entity, chordTol float64) []geom.Vec2 {
	switch e.Kind {
	case Point:
		return []geom.Vec2{s.P(e.ID)}
	case Line:
		return []geom.Vec2{s.P(e.Pts[0]), s.P(e.Pts[1])}
	case Circle, Arc:
		c := s.P(e.Pts[0])
		r := s.Radius(e)
		a0, sw := 0.0, 2*math.Pi
		if e.Kind == Arc {
			a0, sw = s.ArcAngles(e)
		}
		n := int(math.Ceil(sw / geom.ArcStep(math.Max(r, 1e-6), chordTol)))
		if n < 4 {
			n = 4
		}
		out := make([]geom.Vec2, n+1)
		for i := 0; i <= n; i++ {
			a := a0 + sw*float64(i)/float64(n)
			out[i] = c.Add(geom.Vec2{X: math.Cos(a), Y: math.Sin(a)}.Scale(r))
		}
		if e.Kind == Arc {
			out[0], out[n] = s.P(e.Pts[1]), s.P(e.Pts[2])
		}
		return out
	}
	return nil
}

// Bounds of the sketch's points and curves (sketch coordinates).
func (s *Sketch) Bounds() geom.Box {
	var b geom.Box
	for _, e := range s.Ents {
		for _, p := range s.Polyline(e, 0.1) {
			b.Add(geom.Vec3{X: p.X, Y: p.Y})
		}
	}
	return b
}

// Describe names an entity for the UI.
func (s *Sketch) Describe(id int) string {
	e := s.Ent(id)
	if e == nil {
		return fmt.Sprintf("#%d", id)
	}
	switch e.Kind {
	case Line:
		return fmt.Sprintf("Line %d (%.3f mm)", e.ID, s.P(e.Pts[0]).Dist(s.P(e.Pts[1])))
	case Circle:
		return fmt.Sprintf("Circle %d (Ø %.3f mm)", e.ID, 2*s.Radius(e))
	case Arc:
		return fmt.Sprintf("Arc %d (R %.3f mm)", e.ID, s.Radius(e))
	}
	p := s.P(e.ID)
	return fmt.Sprintf("Point %d (%.3f, %.3f)", e.ID, p.X, p.Y)
}

// PointsOf lists the point IDs an entity moves with (itself for a point).
func (s *Sketch) PointsOf(id int) []int {
	e := s.Ent(id)
	if e == nil {
		return nil
	}
	if e.Kind == Point {
		return []int{id}
	}
	return append([]int(nil), e.Pts...)
}

// sortedIDs returns the keys of a set in order.
func sortedIDs(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
