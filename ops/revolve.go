package ops

import (
	"fmt"
	"math"

	"forma/brep"
	"forma/geom"
	"forma/sketch"
)

// Revolve turns a sketch region about an axis lying in its plane (through
// axisP along axisD, both in sketch coordinates) by angle radians (2π for a
// full turn). The region must keep to one side of the axis; it may touch it.
// Lines become planes, cylinders or cones and arcs spheres or tori; faces are
// named "start", "end" (the caps of a part turn) and "rev:<entity>:<n>".
func Revolve(name string, reg sketch.Region, plane geom.Frame, axisP, axisD geom.Vec2, angle float64) (*brep.Body, error) {
	if angle <= 1e-9 || angle > 2*math.Pi+1e-9 {
		return nil, fmt.Errorf("revolve: angle must be in (0°, 360°]")
	}
	if axisD.Len() < 1e-12 {
		return nil, fmt.Errorf("revolve: no axis")
	}
	full := angle > 2*math.Pi-1e-9
	if full {
		angle = 2 * math.Pi
	}
	N := plane.Z
	P0 := plane.At2(axisP)
	A := plane.Dir(geom.Vec3{X: axisD.X, Y: axisD.Y}).Norm()
	U := N.Cross(A).Norm()

	loops := append([][]sketch.Seg{reg.Outer}, reg.Holes...)
	// which side of the axis the profile is on
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, l := range loops {
		for _, s := range l {
			for _, p := range sketch.SegPoints(s, 0.01) {
				r := plane.At2(p).Sub(P0).Dot(U)
				lo, hi = math.Min(lo, r), math.Max(hi, r)
			}
		}
	}
	tol := 1e-6 * (1 + hi - lo)
	switch {
	case lo >= -tol:
	case hi <= tol:
		U = U.Neg()
	default:
		return nil, fmt.Errorf("revolve: the profile crosses the axis")
	}
	if math.Max(math.Abs(lo), math.Abs(hi)) < tol {
		return nil, fmt.Errorf("revolve: the profile lies on the axis")
	}
	F := geom.Frame{O: P0, X: U, Y: A.Cross(U), Z: A}
	rh := func(p3 geom.Vec3) (float64, float64) {
		d := p3.Sub(P0)
		return d.Dot(U), d.Dot(A)
	}
	cs, sn := math.Cos(angle), math.Sin(angle)
	rotDir := func(v geom.Vec3) geom.Vec3 {
		l := geom.Vec3{X: v.Dot(F.X), Y: v.Dot(F.Y), Z: v.Dot(F.Z)}
		return F.Dir(geom.Vec3{X: l.X*cs - l.Y*sn, Y: l.X*sn + l.Y*cs, Z: l.Z})
	}
	rot := func(p geom.Vec3) geom.Vec3 { return P0.Add(rotDir(p.Sub(P0))) }
	ring := func(h float64) geom.Frame { return geom.Frame{O: P0.Add(A.Scale(h)), X: F.X, Y: F.Y, Z: F.Z} }

	b := brep.NewBuilder(name)
	pieces := map[int]int{}
	type vtx struct {
		v0, v1 *brep.Vertex
		circ   *brep.Edge // nil on the axis
		axis   bool
	}
	type lp struct {
		segs   []sketch.Seg
		vs     []vtx
		e0, e1 []*brep.Edge // nil for axis segments of a full turn
		onAxis []bool
	}
	var lps []*lp
	for _, segs := range loops {
		l := &lp{segs: segs}
		for _, s := range segs {
			p := plane.At2(s.A)
			r, h := rh(p)
			v := vtx{axis: r < tol}
			if v.axis {
				// snap onto the axis exactly
				p = P0.Add(A.Scale(h))
				v.v0 = b.Vertex(p)
				v.v1 = v.v0
			} else {
				v.v0 = b.Vertex(p)
				if full {
					v.v1 = v.v0
				} else {
					v.v1 = b.Vertex(rot(p))
				}
				v.circ = b.Edge(v.v0, v.v1, geom.Circle{F: ring(h), R: r}, 0, angle)
			}
			l.vs = append(l.vs, v)
		}
		n := len(segs)
		for i, s := range segs {
			va, vb := l.vs[i], l.vs[(i+1)%n]
			axisSeg := va.axis && vb.axis && !s.Arc
			l.onAxis = append(l.onAxis, axisSeg)
			if axisSeg && full {
				l.e0, l.e1 = append(l.e0, nil), append(l.e1, nil)
				continue
			}
			e0 := revSegEdge(b, plane, s, va.v0, vb.v0, nil)
			e1 := e0
			if !full && !axisSeg {
				e1 = revSegEdge(b, plane, s, va.v1, vb.v1, rotDir)
			}
			l.e0, l.e1 = append(l.e0, e0), append(l.e1, e1)
		}
		lps = append(lps, l)
	}

	// the turned faces
	for _, l := range lps {
		n := len(l.segs)
		for i, s := range l.segs {
			if l.onAxis[i] {
				continue
			}
			va, vb := l.vs[i], l.vs[(i+1)%n]
			r0, h0 := rh(plane.At2(s.A))
			r1, h1 := rh(plane.At2(s.B))
			if va.axis {
				r0 = 0
			}
			if vb.axis {
				r1 = 0
			}
			var surf geom.Surface
			if s.Ctrl != nil {
				// the profile in the axis frame: X its distance out, Z its height
				loc := make([]geom.Vec3, len(s.Ctrl))
				for k, c := range s.Ctrl {
					r, h := rh(plane.At2(c))
					loc[k] = geom.Vec3{X: r, Z: h}
				}
				if va.axis {
					loc[0].X = 0
				}
				if vb.axis {
					loc[len(loc)-1].X = 0
				}
				surf = geom.Revolved{F: F, C: geom.Bezier(loc...)}
			} else if s.Arc {
				rc, hc := rh(plane.At2(s.C))
				if rc < tol {
					surf = geom.Sphere{F: ring(hc), R: s.R}
				} else {
					surf = geom.Torus{F: ring(hc), R: rc, Rm: s.R}
				}
			} else {
				switch {
				case math.Abs(h1-h0) < tol:
					surf = geom.Plane{F: ring(h0)}
				case math.Abs(r1-r0) < tol:
					surf = geom.Cylinder{F: F, R: r0}
				default:
					// framed at the wider end: its radius there sizes the mesh
					if r0 >= r1 {
						surf = geom.Cone{F: ring(h0), R: r0, A: math.Atan((r1 - r0) / (h1 - h0))}
					} else {
						surf = geom.Cone{F: ring(h1), R: r1, A: math.Atan((r1 - r0) / (h1 - h0))}
					}
				}
			}
			// outward: right of the profile's travel, in the sketch plane
			mid, dir := segMid(s)
			m3 := plane.At2(mid)
			d3 := plane.Dir(geom.Vec3{X: dir.X, Y: dir.Y})
			out := d3.Cross(N)
			u, v := surf.Project(m3)
			rev := surf.Normal(u, v).Dot(out) < 0

			loop := []brep.Use{brep.F(l.e0[i])}
			if vb.circ != nil {
				loop = append(loop, brep.F(vb.circ))
			}
			loop = append(loop, brep.R(l.e1[i]))
			if va.circ != nil {
				loop = append(loop, brep.R(va.circ))
			}
			// that loop runs clockwise in (angle, along-profile); flip it if
			// its normal −(∂angle × ∂profile) points inward
			du := A.Cross(m3.Sub(P0))
			if du.Cross(d3).Neg().Dot(out) < 0 {
				loop = reverseLoop(loop)
			}
			k := pieces[s.Src]
			pieces[s.Src]++
			b.Face(surf, rev, fmt.Sprintf("rev:%d:%d", s.Src, k), loop)
		}
	}

	// caps of a part turn: the profile at the start and turned to the end
	if !full {
		startOut := F.Y.Neg()
		startRev := F.Y.Dot(N) > 0
		var sl, el [][]brep.Use
		for _, l := range lps {
			var s0, s1 []brep.Use
			for i := range l.segs {
				s0 = append(s0, brep.F(l.e0[i]))
				s1 = append(s1, brep.F(l.e1[i]))
			}
			if startRev {
				s0 = reverseLoop(s0)
			} else {
				s1 = reverseLoop(s1)
			}
			sl, el = append(sl, s0), append(el, s1)
		}
		b.Face(geom.Plane{F: geom.FrameZ(P0, startOut, A)}, false, "start", sl...)
		b.Face(geom.Plane{F: geom.FrameZ(P0, rotDir(F.Y), A)}, false, "end", el...)
	}
	return b.Finish()
}

func reverseLoop(l []brep.Use) []brep.Use {
	out := make([]brep.Use, len(l))
	for i, u := range l {
		out[len(l)-1-i] = brep.Use{E: u.E, Fwd: !u.Fwd}
	}
	return out
}

// segMid is a segment's middle point and its direction of travel there.
func segMid(s sketch.Seg) (geom.Vec2, geom.Vec2) {
	if s.Ctrl != nil {
		p, d := bezMid(s.Ctrl)
		return p, d
	}
	if !s.Arc {
		return s.A.Lerp(s.B, 0.5), s.B.Sub(s.A).Norm()
	}
	a0 := s.A.Sub(s.C).Angle()
	a1 := s.B.Sub(s.C).Angle()
	var m float64
	if s.CCW {
		sw := geom.WrapAngle(a1 - a0)
		if sw < 1e-12 {
			sw = 2 * math.Pi
		}
		m = a0 + sw/2
	} else {
		sw := geom.WrapAngle(a0 - a1)
		if sw < 1e-12 {
			sw = 2 * math.Pi
		}
		m = a0 - sw/2
	}
	u := geom.Vec2{X: math.Cos(m), Y: math.Sin(m)}
	t := u.Perp()
	if !s.CCW {
		t = t.Scale(-1)
	}
	return s.C.Add(u.Scale(s.R)), t
}

// revSegEdge makes a profile segment's edge, turned by rotDir when given.
func revSegEdge(b *brep.Builder, plane geom.Frame, s sketch.Seg, va, vb *brep.Vertex, rotDir func(geom.Vec3) geom.Vec3) *brep.Edge {
	if s.Ctrl != nil {
		c := bezier3(plane, s.Ctrl, 0)
		if rotDir != nil {
			// turned round the axis like the vertices: through va's offset
			a0 := plane.At2(s.A)
			c = c.Map(func(p geom.Vec3) geom.Vec3 { return va.P.Add(rotDir(p.Sub(a0))) })
		}
		c.Ctrl[0], c.Ctrl[len(c.Ctrl)-1] = va.P, vb.P
		return b.Edge(va, vb, c, 0, 1)
	}
	if !s.Arc {
		return b.LineEdge(va, vb)
	}
	f := plane
	f.O = plane.At2(s.C)
	if rotDir != nil {
		o := f.O
		// turn the arc's frame round the axis: vertices are already turned,
		// so place the centre by the same rotation (through its offset from va)
		f.X, f.Y, f.Z = rotDir(plane.X), rotDir(plane.Y), rotDir(plane.Z)
		f.O = va.P.Sub(rotDir(plane.At2(s.A).Sub(o)))
	}
	a0 := s.A.Sub(s.C).Angle()
	a1 := s.B.Sub(s.C).Angle()
	var t1 float64
	if s.CCW {
		sw := geom.WrapAngle(a1 - a0)
		if sw < 1e-12 {
			sw = 2 * math.Pi
		}
		t1 = a0 + sw
	} else {
		sw := geom.WrapAngle(a0 - a1)
		if sw < 1e-12 {
			sw = 2 * math.Pi
		}
		t1 = a0 - sw
	}
	return b.Edge(va, vb, geom.Circle{F: f, R: s.R}, a0, t1)
}

// bezMid is a Bézier's middle point and its direction of travel there.
func bezMid(c []geom.Vec2) (geom.Vec2, geom.Vec2) {
	pts := make([]geom.Vec3, len(c))
	for i, p := range c {
		pts[i] = geom.Vec3{X: p.X, Y: p.Y}
	}
	b := geom.Bezier(pts...)
	m, d := b.Eval(0.5), b.Deriv(0.5)
	return geom.Vec2{X: m.X, Y: m.Y}, geom.Vec2{X: d.X, Y: d.Y}.Norm()
}
