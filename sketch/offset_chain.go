package sketch

// Offsetting a chain of curves for the sketch's offset tool: every piece
// moved sideways, and the corners joined the way a drawing wants them —
// lines meeting at a mitre, the gap round any other outside corner closed
// by an arc about the corner, the pieces of an inside corner trimmed back
// to where they cross.

import (
	"fmt"
	"math"

	"forma/geom"
)

const miterLimit = 4 // a mitre longer than this many distances becomes an arc

// OffsetChain moves a chain of segments (each starting where the last
// ends; closed: the last ends where the first starts) d to its left (right
// for d < 0) and joins the corners.
func OffsetChain(chain []Seg, d float64, closed bool) []Seg {
	if len(chain) == 0 || d == 0 {
		return append([]Seg(nil), chain...)
	}
	type part struct {
		src    Seg
		pieces []Seg
	}
	var parts []part
	for _, s := range chain {
		if p := OffsetSeg(s, d); len(p) > 0 {
			parts = append(parts, part{s, p})
		}
	}
	if len(parts) == 0 {
		return nil
	}
	var box geom.Box
	for _, s := range chain {
		box.Add(geom.V3(s.A.X, s.A.Y, 0))
		box.Add(geom.V3(s.B.X, s.B.Y, 0))
	}
	tol := 1e-7 * (box.Diagonal() + math.Abs(d) + 1)
	n := len(parts)
	joins := n - 1
	if closed {
		joins = n
	}
	// what goes between part i and the next
	extra := make([][]Seg, n)
	for i := 0; i < joins; i++ {
		j := (i + 1) % n
		P := &parts[i].pieces[len(parts[i].pieces)-1]
		Q := &parts[j].pieces[0]
		a, b := P.B, Q.A
		if a.Dist(b) < tol {
			*Q = startAt(*Q, a)
			continue
		}
		_, tin := segTangents(parts[i].src)
		tout, _ := segTangents(parts[j].src)
		v := parts[i].src.B
		turn := tin.Cross(tout) // > 0 turning left
		if math.Abs(turn) < 1e-9 && tin.Dot(tout) > 0 {
			*Q = startAt(*Q, a) // smooth: the offsets meet but for rounding
			continue
		}
		if turn*d < 0 {
			// the outside of the corner: the moved pieces part
			if P.Ctrl == nil && !P.Arc && Q.Ctrl == nil && !Q.Arc {
				if m, ok := lineMeet(P.A, P.B, Q.A, Q.B); ok && m.Dist(v) <= miterLimit*math.Abs(d) {
					P.B, Q.A = m, m
					continue
				}
			}
			extra[i] = []Seg{arcSeg(v, a, b, d < 0, parts[i].src.Src)}
			continue
		}
		// the inside: they overlap; cut both back to where they cross
		if x, tp, tq, ok := segCross(*P, *Q, v, tol); ok {
			*P = trimEnd(*P, tp, x)
			*Q = trimStart(*Q, tq, x)
			continue
		}
		if P.Ctrl == nil && !P.Arc && Q.Ctrl == nil && !Q.Arc {
			if m, ok := lineMeet(P.A, P.B, Q.A, Q.B); ok {
				P.B, Q.A = m, m
				continue
			}
		}
		// pieces too short to cross (the offset is wider than them): a chord
		extra[i] = []Seg{{A: a, B: b, Src: parts[i].src.Src}}
	}
	var out []Seg
	for i := range parts {
		out = append(out, parts[i].pieces...)
		out = append(out, extra[i]...)
	}
	if closed && len(out) > 0 {
		// the last piece's end and the first's start are one point
		out[len(out)-1].B = out[0].A
		if c := out[len(out)-1].Ctrl; c != nil {
			c[len(c)-1] = out[0].A
		}
	}
	return out
}

// lineMeet is where the lines through a–b and c–d cross.
func lineMeet(a, b, c, d geom.Vec2) (geom.Vec2, bool) {
	r, s := b.Sub(a), d.Sub(c)
	den := r.Cross(s)
	if math.Abs(den) < 1e-12*r.Len()*s.Len() {
		return geom.Vec2{}, false
	}
	t := c.Sub(a).Cross(s) / den
	return a.Add(r.Scale(t)), true
}

// arcSeg is the arc round c from a to b (counter-clockwise when ccw), its
// radius a's distance and b put on it.
func arcSeg(c, a, b geom.Vec2, ccw bool, src int) Seg {
	r := a.Dist(c)
	if d := b.Sub(c); d.Len() > 1e-12 {
		b = c.Add(d.Norm().Scale(r))
	}
	return Seg{Arc: true, A: a, B: b, C: c, R: r, CCW: ccw, Src: src}
}

// segCurve is a segment as an intersectable curve; rev says the curve's
// parameter runs against the segment's (a clockwise arc).
func segCurve(s Seg) (*gcurve, bool) {
	switch {
	case s.Ctrl != nil:
		return &gcurve{bez: s.Ctrl, a: s.A, b: s.B}, false
	case s.Arc:
		a, b := s.A, s.B
		if !s.CCW {
			a, b = b, a
		}
		a0 := a.Sub(s.C).Angle()
		sw := geom.WrapAngle(b.Sub(s.C).Angle() - a0)
		if sw < 1e-12 {
			sw = 2 * math.Pi
		}
		return &gcurve{arc: true, c: s.C, r: s.R, a0: a0, sw: sw}, !s.CCW
	}
	return &gcurve{a: s.A, b: s.B}, false
}

// segCross is where two segments cross nearest v, with the parameters (in
// [0, 1] along each) there.
func segCross(p, q Seg, v geom.Vec2, tol float64) (geom.Vec2, float64, float64, bool) {
	gp, rp := segCurve(p)
	gq, rq := segCurve(q)
	best, bd := [2]float64{}, math.Inf(1)
	for _, x := range intersect(gp, gq, tol) {
		if d := gp.at(x[0]).Dist(v); d < bd {
			best, bd = x, d
		}
	}
	if math.IsInf(bd, 1) {
		return geom.Vec2{}, 0, 0, false
	}
	x := gp.at(best[0])
	tp, tq := best[0], best[1]
	if rp {
		tp = 1 - tp
	}
	if rq {
		tq = 1 - tq
	}
	// not a crossing at the far ends (the pieces only touching there)
	if tp < 1e-9 || tq > 1-1e-9 {
		return geom.Vec2{}, 0, 0, false
	}
	return x, tp, tq, true
}

// trimEnd is s ending at x (at parameter t along it).
func trimEnd(s Seg, t float64, x geom.Vec2) Seg {
	if s.Ctrl != nil {
		l, _ := bezSplit(s.Ctrl, t)
		l[len(l)-1] = x
		s.Ctrl = l
	}
	if s.Arc {
		x = s.C.Add(x.Sub(s.C).Norm().Scale(s.R))
	}
	s.B = x
	return s
}

// trimStart is s starting at x (at parameter t along it).
func trimStart(s Seg, t float64, x geom.Vec2) Seg {
	if s.Ctrl != nil {
		_, r := bezSplit(s.Ctrl, t)
		r[0] = x
		s.Ctrl = r
	}
	if s.Arc {
		x = s.C.Add(x.Sub(s.C).Norm().Scale(s.R))
	}
	s.A = x
	return s
}

// GrowRegion is a region grown by d (shrunk for d < 0): its outline moved
// d out and its holes d in, the corners joined as the offset tool joins
// them. A hole closed up by the growing is gone.
func GrowRegion(r Region, d float64) (Region, error) {
	if d == 0 {
		return r, nil
	}
	// the region lies left of every loop (outline counter-clockwise, holes
	// clockwise): away from it is right
	out := OffsetChain(r.Outer, -d, true)
	if len(out) == 0 || LoopArea(out) <= 0 {
		return Region{}, fmt.Errorf("shrunk by %g the region is gone", -d)
	}
	g := Region{Outer: out, Text: r.Text, Ink: r.Ink}
	g.Poly = loopPoly(out, 2e-5)
	g.Area = LoopArea(out)
	for _, h := range r.Holes {
		o := OffsetChain(h, -d, true)
		if len(o) == 0 || LoopArea(o) >= 0 {
			continue // closed up (or turned inside out)
		}
		g.Holes = append(g.Holes, o)
		g.HolePolys = append(g.HolePolys, loopPoly(o, 2e-5))
		g.Area += LoopArea(o)
	}
	return g, nil
}
