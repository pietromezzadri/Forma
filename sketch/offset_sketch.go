package sketch

// The offset tool: a chain of curves (or one circle) copied a distance to
// one side. The copy is driven, not solved: an Offset constraint holds the
// source curves and the distance (a dimension, so it takes an expression),
// and after every solve the copy is worked out again from where the
// sources are and its points moved there (they are locked, as projected
// geometry is). Deleting a curve of the copy deletes the offset; deleting
// a source leaves the copy as ordinary curves.

import (
	"fmt"
	"math"

	"forma/geom"
)

// AddOffset copies the curves ids (a chain or a closed loop of lines, arcs
// and splines, or a circle) d to the side: outward from a closed loop,
// left of an open chain as it runs from its first curve (right for d < 0).
func (s *Sketch) AddOffset(ids []int, d float64) (*Constraint, error) {
	if d == 0 || math.IsNaN(d) {
		return nil, fmt.Errorf("the offset distance can't be 0")
	}
	order, _, err := s.chainOf(ids)
	if err != nil {
		return nil, err
	}
	c := &Constraint{ID: s.id(), Kind: Offset, E: order, Value: d}
	s.Cons = append(s.Cons, c)
	if err := s.refreshOffset(c); err != nil {
		s.Cons = s.Cons[:len(s.Cons)-1]
		return nil, err
	}
	return c, nil
}

// OffsetOf is the offset constraint curve id is part of the copy of (nil if
// none).
func (s *Sketch) OffsetOf(id int) *Constraint {
	e := s.Ent(id)
	if e == nil || e.Off == 0 {
		return nil
	}
	if c := s.Con(e.Off); c != nil && c.Kind == Offset {
		return c
	}
	return nil
}

// DeleteOffset removes an offset and its copy.
func (s *Sketch) DeleteOffset(c *Constraint) {
	s.dropCon(c.ID)
	s.clearOffset(c.ID)
}

func (s *Sketch) dropCon(id int) {
	var cons []*Constraint
	for _, c := range s.Cons {
		if c.ID != id {
			cons = append(cons, c)
		}
	}
	s.Cons = cons
}

// offsetEnts are the curves an offset made, in the order made.
func (s *Sketch) offsetEnts(id int) []*Entity {
	var out []*Entity
	for _, e := range s.Ents {
		if e.Off == id && e.Kind != Point {
			out = append(out, e)
		}
	}
	return out
}

// chainOf puts curves in order end to end: their IDs from one end of the
// chain to the other (or round the loop), and whether it is closed.
func (s *Sketch) chainOf(ids []int) ([]int, bool, error) {
	if len(ids) == 0 {
		return nil, false, fmt.Errorf("pick the curves to offset")
	}
	for _, id := range ids {
		e := s.Ent(id)
		if e == nil {
			return nil, false, fmt.Errorf("no curve %d", id)
		}
		switch e.Kind {
		case Line, Arc, Spline:
		case Circle:
			if len(ids) > 1 {
				return nil, false, fmt.Errorf("a circle is offset on its own")
			}
			return []int{id}, true, nil
		default:
			return nil, false, fmt.Errorf("%s %d can't be offset (lines, arcs, splines and circles can)", e.Kind, id)
		}
	}
	segs := map[int]Seg{}
	for _, id := range ids {
		segs[id] = s.entSeg(s.Ent(id), false)
	}
	tol := 1e-6 * (s.Bounds().Diagonal() + 1)
	meets := func(p geom.Vec2, id int) bool {
		return segs[id].A.Dist(p) < tol || segs[id].B.Dist(p) < tol
	}
	// start from a curve with an end nothing else meets (an open chain's
	// end), else anywhere (a loop)
	ends := func(id int) int {
		n := 0
		for _, p := range []geom.Vec2{segs[id].A, segs[id].B} {
			for _, o := range ids {
				if o != id && meets(p, o) {
					n++
					break
				}
			}
		}
		return n
	}
	start := ids[0]
	for _, id := range ids {
		if ends(id) < 2 {
			start = id
			break
		}
	}
	order := []int{start}
	used := map[int]bool{start: true}
	meetsAny := func(p geom.Vec2) bool {
		for _, o := range ids {
			if o != start && meets(p, o) {
				return true
			}
		}
		return false
	}
	// leave the start by the end that meets the next curve
	at := segs[start].B
	if !meetsAny(at) && meetsAny(segs[start].A) {
		at = segs[start].A
	}
	for len(order) < len(ids) {
		next := -1
		for _, o := range ids {
			if !used[o] && meets(at, o) {
				next = o
				break
			}
		}
		if next < 0 {
			return nil, false, fmt.Errorf("the curves don't make one chain")
		}
		used[next] = true
		order = append(order, next)
		if segs[next].A.Dist(at) < tol {
			at = segs[next].B
		} else {
			at = segs[next].A
		}
	}
	first := segs[order[0]]
	closed := len(order) > 1 && (first.A.Dist(at) < tol || first.B.Dist(at) < tol)
	if len(order) == 1 {
		closed = first.A.Dist(first.B) < tol
	}
	return order, closed, nil
}

// entSeg is a curve as a segment, run backwards if rev.
func (s *Sketch) entSeg(e *Entity, rev bool) Seg {
	var g Seg
	switch e.Kind {
	case Line:
		g = Seg{A: s.P(e.Pts[0]), B: s.P(e.Pts[1]), Src: e.ID}
	case Arc:
		g = Seg{Arc: true, A: s.P(e.Pts[1]), B: s.P(e.Pts[2]), C: s.P(e.Pts[0]), R: s.Radius(e), CCW: true, Src: e.ID}
	case Spline:
		c := s.Ctrl(e)
		g = Seg{A: c[0], B: c[len(c)-1], Ctrl: c, Src: e.ID}
	}
	if rev {
		g.A, g.B = g.B, g.A
		g.CCW = !g.CCW
		if g.Ctrl != nil {
			c := make([]geom.Vec2, len(g.Ctrl))
			for i, p := range g.Ctrl {
				c[len(c)-1-i] = p
			}
			g.Ctrl = c
		}
	}
	return g
}

// chainSegs are an offset's sources as segments running one way round.
func (s *Sketch) chainSegs(ids []int) ([]Seg, bool) {
	tol := 1e-6 * (s.Bounds().Diagonal() + 1)
	var out []Seg
	for i, id := range ids {
		e := s.Ent(id)
		g := s.entSeg(e, false)
		switch {
		case i > 0:
			if g.A.Dist(out[i-1].B) > tol && g.B.Dist(out[i-1].B) <= tol {
				g = s.entSeg(e, true)
			}
		case len(ids) > 1:
			nx := s.entSeg(s.Ent(ids[1]), false)
			if g.A.Dist(nx.A) <= tol || g.A.Dist(nx.B) <= tol {
				g = s.entSeg(e, true) // its start meets the next: run it the other way
			}
		}
		out = append(out, g)
	}
	closed := len(out) > 1 && out[len(out)-1].B.Dist(out[0].A) <= tol
	return out, closed
}

// offsetShape works out an offset's copy from where its sources are now.
func (s *Sketch) offsetShape(c *Constraint) ([]Seg, *Entity, float64, error) {
	if len(c.E) == 1 {
		if e := s.Ent(c.E[0]); e != nil && e.Kind == Circle {
			r := s.Radius(e) + c.Value
			if r <= 1e-9 {
				return nil, e, 0, fmt.Errorf("the offset is wider than the circle")
			}
			return nil, e, r, nil
		}
	}
	for _, id := range c.E {
		if s.Ent(id) == nil {
			return nil, nil, 0, fmt.Errorf("its curves are gone")
		}
	}
	segs, closed := s.chainSegs(c.E)
	d := c.Value
	if closed && LoopArea(segs) > 0 {
		d = -d // counter-clockwise: outward is to the right
	}
	out := OffsetChain(segs, d, closed)
	if len(out) == 0 {
		return nil, nil, 0, fmt.Errorf("the offset swallows the curves")
	}
	return out, nil, 0, nil
}

// refreshOffset puts an offset's copy where its sources say: its points
// moved if the copy has the same make-up, else made anew.
func (s *Sketch) refreshOffset(c *Constraint) error {
	segs, circle, r, err := s.offsetShape(c)
	if err != nil {
		return err
	}
	have := s.offsetEnts(c.ID)
	if circle != nil {
		if len(have) == 1 && have[0].Kind == Circle {
			s.MoveLocked(have[0].Pts[0], s.center(circle.ID))
			s.SetLockedRadius(have[0].ID, r)
			return nil
		}
		s.clearOffset(c.ID)
		ce := s.AddPoint(s.center(circle.ID))
		ce.Off = c.ID
		s.Lock(ce.ID)
		o := s.AddCircle(ce.ID, r)
		o.Off = c.ID
		s.LockRadius(o.ID)
		return nil
	}
	if sameMakeup(have, segs) {
		for i, g := range segs {
			e := have[i]
			switch e.Kind {
			case Line:
				s.MoveLocked(e.Pts[0], g.A)
				s.MoveLocked(e.Pts[1], g.B)
			case Arc:
				a, b := g.A, g.B
				if !g.CCW {
					a, b = b, a
				}
				s.MoveLocked(e.Pts[0], g.C)
				s.MoveLocked(e.Pts[1], a)
				s.MoveLocked(e.Pts[2], b)
			case Spline:
				for k, p := range g.Ctrl {
					s.MoveLocked(e.Pts[k], p)
				}
			}
		}
		return nil
	}
	s.clearOffset(c.ID)
	point := func(p geom.Vec2) int {
		pt := s.AddPoint(p)
		pt.Off = c.ID
		s.Lock(pt.ID)
		return pt.ID
	}
	// joints shared between neighbours
	n := len(segs)
	_, closed := s.chainSegs(c.E)
	joint := make([]int, n+1)
	for i := range segs {
		joint[i] = point(segs[i].A)
	}
	if closed {
		joint[n] = joint[0]
	} else {
		joint[n] = point(segs[n-1].B)
	}
	for i, g := range segs {
		var e *Entity
		a, b := joint[i], joint[i+1]
		switch {
		case g.Ctrl != nil:
			pts := []int{a}
			for _, p := range g.Ctrl[1 : len(g.Ctrl)-1] {
				pts = append(pts, point(p))
			}
			e = s.AddSpline(append(pts, b)...)
		case g.Arc:
			if !g.CCW {
				a, b = b, a
			}
			e = &Entity{ID: s.id(), Kind: Arc, Pts: []int{point(g.C), a, b}}
			s.Ents = append(s.Ents, e)
		default:
			e = s.AddLine(a, b)
		}
		e.Off = c.ID
	}
	s.idx = nil
	return nil
}

// sameMakeup: the curves have the kinds (and splines the degrees) of segs.
func sameMakeup(have []*Entity, segs []Seg) bool {
	if len(have) != len(segs) {
		return false
	}
	for i, g := range segs {
		e := have[i]
		switch {
		case g.Ctrl != nil:
			if e.Kind != Spline || len(e.Pts) != len(g.Ctrl) {
				return false
			}
		case g.Arc:
			if e.Kind != Arc {
				return false
			}
		default:
			if e.Kind != Line {
				return false
			}
		}
	}
	return true
}

// clearOffset deletes an offset's copy (keeping the constraint).
func (s *Sketch) clearOffset(id int) {
	dead := map[int]bool{}
	for _, e := range s.Ents {
		if e.Off == id {
			dead[e.ID] = true
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

// updateOffsets reworks every offset's copy after a solve; a copy whose
// offset is gone (its sources deleted) is let go as ordinary curves.
func (s *Sketch) updateOffsets() {
	live := map[int]bool{}
	for _, c := range s.Cons {
		if c.Kind == Offset {
			live[c.ID] = true
			s.refreshOffset(c) // a copy that can't be made keeps its last shape
		}
	}
	for _, e := range s.Ents {
		if e.Off != 0 && !live[e.Off] {
			s.release(e)
		}
	}
}

// release lets a curve or point of a gone offset move: its locks dropped,
// an arc held to its radius again.
func (s *Sketch) release(e *Entity) {
	e.Off = 0
	var cons []*Constraint
	for _, c := range s.Cons {
		if c.Internal && (c.Kind == Fix || c.Kind == Radius) && len(c.E) == 1 && c.E[0] == e.ID {
			continue
		}
		cons = append(cons, c)
	}
	s.Cons = cons
	if e.Kind == Arc {
		s.Cons = append(s.Cons, &Constraint{ID: s.id(), Kind: ArcRadius, E: []int{e.ID}, Internal: true})
	}
}

// OffsetAnchor is where an offset's dimension is drawn: from the middle of
// its first source curve to the nearest point of the copy.
func (s *Sketch) OffsetAnchor(c *Constraint) (from, to geom.Vec2, ok bool) {
	if c.Kind != Offset || len(c.E) == 0 {
		return
	}
	src := s.Ent(c.E[0])
	if src == nil {
		return
	}
	pl := s.Polyline(src, 0.05)
	if src.Kind == Circle {
		from = pl[len(pl)/8]
	} else {
		from = SegPoint(s.entSeg(src, false), 0.5)
	}
	best := math.Inf(1)
	for _, e := range s.offsetEnts(c.ID) {
		p := s.Polyline(e, 0.01)
		for i := 0; i+1 < len(p); i++ {
			q := closestOnSeg(from, p[i], p[i+1])
			if d := q.Dist(from); d < best {
				best, to = d, q
			}
		}
	}
	return from, to, !math.IsInf(best, 1)
}

func closestOnSeg(p, a, b geom.Vec2) geom.Vec2 {
	d := b.Sub(a)
	l := d.Dot(d)
	if l < 1e-300 {
		return a
	}
	t := math.Max(0, math.Min(1, p.Sub(a).Dot(d)/l))
	return a.Add(d.Scale(t))
}

// SegPoint is the point at t (0..1) along a segment.
func SegPoint(g Seg, t float64) geom.Vec2 {
	switch {
	case g.Ctrl != nil:
		return bezAt(g.Ctrl, t)
	case g.Arc:
		a0 := g.A.Sub(g.C).Angle()
		sw := geom.WrapAngle(g.B.Sub(g.C).Angle() - a0)
		if !g.CCW {
			sw -= 2 * math.Pi
		}
		a := a0 + sw*t
		return g.C.Add(geom.Vec2{X: math.Cos(a), Y: math.Sin(a)}.Scale(g.R))
	}
	return g.A.Lerp(g.B, t)
}
