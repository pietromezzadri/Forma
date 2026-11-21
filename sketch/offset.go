package sketch

// Offsetting loops of segments: each piece moved sideways by a distance,
// the gaps at corners rounded where the pieces part and joined by a chord
// where they overlap. The loops that come out may cross themselves
// (a corner's overlap, a counter closing up); the region they enclose by
// the non-zero winding rule is the true offset (Chen & McMains, "Polygon
// offsetting by computing winding numbers"), which is how a text's ink is
// found anyway.

import (
	"math"

	"forma/geom"
)

const offsetTol = 1e-3 // how far an offset Bézier may stray from the true offset

// OffsetContours grows a text's ink by d (shrinks it when d < 0): every
// loop moved d away from the ink, whichever way the font winds its loops.
func OffsetContours(loops [][]Seg, d float64) [][]Seg {
	if d == 0 {
		return loops
	}
	area := 0.0
	for _, l := range loops {
		area += LoopArea(l)
	}
	// the ink is on the loops' left when they wind counter-clockwise
	left := -d
	if area < 0 {
		left = d
	}
	var out [][]Seg
	for _, l := range loops {
		if o := OffsetLoop(l, left); len(o) > 0 {
			out = append(out, o)
		}
	}
	return out
}

// segTangents are a segment's unit directions leaving A and arriving at B.
func segTangents(s Seg) (geom.Vec2, geom.Vec2) {
	switch {
	case s.Ctrl != nil:
		return bezEndDir(s.Ctrl, false), bezEndDir(s.Ctrl, true)
	case s.Arc:
		ra, rb := s.A.Sub(s.C), s.B.Sub(s.C)
		ta, tb := geom.Vec2{X: -ra.Y, Y: ra.X}.Norm(), geom.Vec2{X: -rb.Y, Y: rb.X}.Norm()
		if !s.CCW {
			ta, tb = ta.Scale(-1), tb.Scale(-1)
		}
		return ta, tb
	}
	t := s.B.Sub(s.A).Norm()
	return t, t
}

// bezEndDir is a Bézier's direction at its start (or end), looking past
// handles lying on the end point.
func bezEndDir(c []geom.Vec2, end bool) geom.Vec2 {
	n := len(c) - 1
	for k := 1; k <= n; k++ {
		var d geom.Vec2
		if end {
			d = c[n].Sub(c[n-k])
		} else {
			d = c[k].Sub(c[0])
		}
		if d.Len() > 1e-12 {
			return d.Norm()
		}
	}
	return geom.Vec2{X: 1}
}

func leftOf(t geom.Vec2) geom.Vec2 { return geom.Vec2{X: -t.Y, Y: t.X} }

// OffsetSeg is a segment moved d to its left (right for d < 0): a line, an
// arc of another radius (none if it shrinks to nothing), or Béziers within
// offsetTol of the true offset curve.
func OffsetSeg(s Seg, d float64) []Seg {
	switch {
	case s.Ctrl != nil:
		c := s.Ctrl
		if len(c) == 3 { // raise a quadratic to a cubic
			c = []geom.Vec2{c[0], c[0].Add(c[1].Sub(c[0]).Scale(2.0 / 3)), c[2].Add(c[1].Sub(c[2]).Scale(2.0 / 3)), c[2]}
		}
		if len(c) == 2 {
			return OffsetSeg(Seg{A: c[0], B: c[1], Src: s.Src}, d)
		}
		var out []Seg
		for _, b := range offsetBez(c, d, 0) {
			out = append(out, Seg{A: b[0], B: b[len(b)-1], Ctrl: b, Src: s.Src})
		}
		return out
	case s.Arc:
		// the left of a counter-clockwise arc is its centre
		r := s.R - d
		if !s.CCW {
			r = s.R + d
		}
		if r <= 1e-9 {
			return nil
		}
		k := r / s.R
		return []Seg{{Arc: true, A: s.C.Add(s.A.Sub(s.C).Scale(k)), B: s.C.Add(s.B.Sub(s.C).Scale(k)), C: s.C, R: r, CCW: s.CCW, Src: s.Src}}
	}
	n := leftOf(s.B.Sub(s.A).Norm()).Scale(d)
	return []Seg{{A: s.A.Add(n), B: s.B.Add(n), Src: s.Src}}
}

// offsetBez offsets a cubic by d to its left: ends moved along their
// normals, handles kept in direction and scaled by how the radius of
// curvature changes, halved until the result is within offsetTol.
func offsetBez(c []geom.Vec2, d float64, depth int) [][]geom.Vec2 {
	t0, t1 := bezEndDir(c, false), bezEndDir(c, true)
	k0, k1 := bezCurvature(c, 0), bezCurvature(c, 1)
	p0 := c[0].Add(leftOf(t0).Scale(d))
	p3 := c[3].Add(leftOf(t1).Scale(d))
	f0, f1 := 1-d*k0, 1-d*k1
	q := []geom.Vec2{p0, p0.Add(c[1].Sub(c[0]).Scale(f0)), p3.Add(c[2].Sub(c[3]).Scale(f1)), p3}
	// where the curve bends tighter than d toward the offset its offset folds
	// back on itself (a swallowtail the winding rule leaves out): no point
	// halving it finer
	if depth >= 7 || depth >= 2 && (f0 <= 0 || f1 <= 0) {
		return [][]geom.Vec2{q}
	}
	tol := offsetTol
	ok := f0 > 0 && f1 > 0
	for i := 1; i <= 7 && ok; i++ {
		t := float64(i) / 8
		dv := bezDeriv(c, t)
		if dv.Len() < 1e-12 {
			continue
		}
		want := bezAt(c, t).Add(leftOf(dv.Norm()).Scale(d))
		if bezAt(q, bezProject(q, want)).Dist(want) > tol {
			ok = false
		}
	}
	if ok {
		return [][]geom.Vec2{q}
	}
	l, r := bezSplit(c, 0.5)
	return append(offsetBez(l, d, depth+1), offsetBez(r, d, depth+1)...)
}

// OffsetLoop is a closed loop with every segment moved d to its left, the
// corners joined: an arc (as Béziers) round the corner where the moved
// pieces part, a chord where they overlap.
func OffsetLoop(l []Seg, d float64) []Seg {
	if len(l) == 0 {
		return nil
	}
	pieces := make([][]Seg, len(l))
	for i, s := range l {
		pieces[i] = OffsetSeg(s, d)
	}
	var out []Seg
	src := l[0].Src
	for i, s := range l {
		out = append(out, pieces[i]...)
		next := l[(i+1)%len(l)]
		if len(pieces[i]) == 0 || len(pieces[(i+1)%len(l)]) == 0 {
			continue
		}
		a := pieces[i][len(pieces[i])-1].B
		b := pieces[(i+1)%len(l)][0].A
		if a.Dist(b) < 1e-9 {
			continue
		}
		_, tin := segTangents(s)
		tout, _ := segTangents(next)
		v := s.B
		turn := tin.Cross(tout) // > 0 turning left
		// a slight turn inward: the moved pieces overlap a hair; the next starts
		// where this one ends (a chord there would double back on them)
		inward := turn*d > 0 && tin.Dot(tout) > 0 && math.Abs(turn) < 0.15
		if inward || tin.Dot(tout) > 0 && (math.Abs(turn) < 0.03 || a.Dist(b) < 0.02*math.Abs(d)) {
			// a smooth join (the ends only miss by the offsets' error): the
			// next piece starts where this one ends
			if j := (i + 1) % len(l); j == 0 {
				out[0] = startAt(out[0], a)
			} else {
				pieces[j][0] = startAt(pieces[j][0], a)
			}
			continue
		}
		if turn*d > 0 {
			// the offset side is inside the turn: the moved pieces overlap;
			// a chord joins them (the little loop it closes winds twice, so
			// it is ink; a bridge out to the corner and back made a spike of
			// two lines on top of each other at a slight turn)
			out = append(out, Seg{A: a, B: b, Src: src})
			continue
		}
		out = append(out, arcBeziers(v, a, b, d < 0, src)...)
	}
	return out
}

// startAt is s starting at p instead (a Bézier's first handle moved with
// its end, so its direction stays).
func startAt(s Seg, p geom.Vec2) Seg {
	if s.Ctrl != nil {
		c := append([]geom.Vec2(nil), s.Ctrl...)
		c[1] = c[1].Add(p.Sub(c[0]))
		c[0] = p
		s.Ctrl = c
	}
	s.A = p
	return s
}

// arcBeziers is the arc round c from a to b (counter-clockwise, or
// clockwise when ccw is false… by the shorter way the corner turns) as
// cubic Béziers of at most a quarter turn each.
func arcBeziers(c, a, b geom.Vec2, ccw bool, src int) []Seg {
	r := a.Dist(c)
	a0 := a.Sub(c).Angle()
	sw := geom.WrapAngle(b.Sub(c).Angle() - a0)
	if !ccw {
		sw -= 2 * math.Pi
	}
	if math.Abs(sw) > 2*math.Pi-1e-9 {
		sw = 0
	}
	n := int(math.Ceil(math.Abs(sw) / (math.Pi / 2)))
	if n < 1 {
		n = 1
	}
	step := sw / float64(n)
	k := 4.0 / 3 * math.Tan(step/4) * r
	var out []Seg
	at := func(ang float64) geom.Vec2 { return c.Add(geom.Vec2{X: math.Cos(ang), Y: math.Sin(ang)}.Scale(r)) }
	for i := 0; i < n; i++ {
		u0, u1 := a0+step*float64(i), a0+step*float64(i+1)
		p0, p3 := at(u0), at(u1)
		if i == 0 {
			p0 = a
		}
		if i == n-1 {
			p3 = b
		}
		t0 := geom.Vec2{X: -math.Sin(u0), Y: math.Cos(u0)}
		t1 := geom.Vec2{X: -math.Sin(u1), Y: math.Cos(u1)}
		ctrl := []geom.Vec2{p0, p0.Add(t0.Scale(k)), p3.Sub(t1.Scale(k)), p3}
		out = append(out, Seg{A: p0, B: p3, Ctrl: ctrl, Src: src})
	}
	return out
}
