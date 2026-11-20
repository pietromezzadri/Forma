package geom

import "math"

// Ellipse lies in F's XY plane round F.O: semi-axis A along F.X, B along
// F.Y; t + P is the eccentric angle from F.X toward F.Y (P lets an arc start
// at t = 0, as circle arcs do by their frame). A plane meets a cylinder in one
// when it cuts the axis at a slant.
type Ellipse struct {
	F       Frame
	A, B, P float64
}

func (e Ellipse) Eval(t float64) Vec3 {
	t += e.P
	return e.F.O.Add(e.F.X.Scale(e.A * math.Cos(t))).Add(e.F.Y.Scale(e.B * math.Sin(t)))
}
func (e Ellipse) Deriv(t float64) Vec3 {
	t += e.P
	return e.F.X.Scale(-e.A * math.Sin(t)).Add(e.F.Y.Scale(e.B * math.Cos(t)))
}
func (e Ellipse) Project(p Vec3) float64 {
	l := e.F.ToLocal(p)
	t := math.Atan2(l.Y*e.A, l.X*e.B)
	// Newton on (E(t) − p)·E'(t) = 0 in the ellipse's plane
	for i := 0; i < 12; i++ {
		c, s := math.Cos(t), math.Sin(t)
		dx, dy := e.A*c-l.X, e.B*s-l.Y
		ex, ey := -e.A*s, e.B*c
		f := dx*ex + dy*ey
		df := ex*ex + ey*ey + dx*(-e.A*c) + dy*(-e.B*s)
		if math.Abs(df) < 1e-300 {
			break
		}
		step := f / df
		t -= step
		if math.Abs(step) < 1e-15 {
			break
		}
	}
	return WrapAngle(t - e.P)
}
func (Ellipse) Kind() string { return "ellipse" }

// CurvePeriod is 2π for closed curves parametrised by angle (circles,
// ellipses), else 0: their parameters wrap and a range may cross 0.
func CurvePeriod(c Curve) float64 {
	switch c.(type) {
	case Circle, Ellipse:
		return 2 * math.Pi
	}
	return 0
}

// An axis of revolution, for recognising circles.
type axis struct{ o, d Vec3 }

func revAxis(s Surface) (axis, bool) {
	switch k := s.(type) {
	case Cylinder:
		return axis{k.F.O, k.F.Z}, true
	case Cone:
		return axis{k.F.O, k.F.Z}, true
	case Torus:
		return axis{k.F.O, k.F.Z}, true
	}
	return axis{}, false
}

// ExactCurve recognises the intersection of surfaces a and b through pts
// (points lying on both) as a line, a circle or an ellipse when every point
// lies on it within tol: two planes and lines on cylinders and cones
// (straight runs), a plane square to a cylinder, cone or torus, a plane
// through a sphere, two surfaces of revolution on one axis (circles about
// it), a plane cutting a cylinder at a slant (an ellipse). Otherwise the
// curve has no closed form here and the caller keeps its polyline.
//
// The circle's and ellipse's frames are right-handed with Z along the axis
// or the plane's normal; which way an edge runs along them is the caller's.
func ExactCurve(a, b Surface, pts []Vec3, closed bool, tol float64) (Curve, bool) {
	if len(pts) < 2 {
		return nil, false
	}
	if !closed {
		first, last := pts[0], pts[len(pts)-1]
		if l, n := LineThrough(first, last); n > 10*tol {
			ok := true
			for _, p := range pts {
				t := l.Project(p)
				if l.Eval(t).Dist(p) > tol || t < -tol || t > n+tol {
					ok = false
					break
				}
			}
			if ok {
				return l, true
			}
		}
	}
	var axes []axis
	for _, s := range []Surface{a, b} {
		if ax, ok := revAxis(s); ok {
			axes = append(axes, ax)
		}
	}
	pl, plOK := a.(Plane)
	other := b
	if !plOK {
		pl, plOK = b.(Plane)
		other = a
	}
	if plOK {
		if sp, ok := other.(Sphere); ok {
			axes = append(axes, axis{sp.F.O, pl.F.Z})
		}
	}
	sa, okA := a.(Sphere)
	sb, okB := b.(Sphere)
	if okA && okB {
		if d := sb.F.O.Sub(sa.F.O); d.Len() > tol {
			axes = append(axes, axis{sa.F.O, d.Norm()})
		}
	}
	for _, ax := range axes {
		if c, ok := circleAbout(ax, pts, tol); ok {
			return snapCircle(c, tol, a, b), true
		}
	}
	if plOK {
		if cy, ok := other.(Cylinder); ok {
			if e, ok := slantEllipse(pl, cy); ok && onEllipse(e, pts, tol) {
				return e, true
			}
		}
	}
	return nil, false
}

// circleAbout: the points lie on one circle about the axis.
func circleAbout(ax axis, pts []Vec3, tol float64) (Circle, bool) {
	d := ax.d.Norm()
	hs, rs := 0.0, 0.0
	for _, p := range pts {
		q := p.Sub(ax.o)
		h := q.Dot(d)
		hs += h
		rs += q.Sub(d.Scale(h)).Len()
	}
	n := float64(len(pts))
	h, r := hs/n, rs/n
	if r < 10*tol {
		return Circle{}, false
	}
	for _, p := range pts {
		q := p.Sub(ax.o)
		ph := q.Dot(d)
		if math.Abs(ph-h) > tol || math.Abs(q.Sub(d.Scale(ph)).Len()-r) > tol {
			return Circle{}, false
		}
	}
	o := ax.o.Add(d.Scale(h))
	x := pts[0].Sub(o)
	x = x.Sub(d.Scale(x.Dot(d))).Norm()
	return Circle{F: Frame{O: o, X: x, Y: d.Cross(x), Z: d}, R: r}, true
}

// slantEllipse is where plane p cuts cylinder c at a slant.
func slantEllipse(p Plane, c Cylinder) (Ellipse, bool) {
	n, d := p.F.Z.Norm(), c.F.Z.Norm()
	cos := n.Dot(d)
	if math.Abs(cos) < 1e-9 || math.Abs(cos) > 1-1e-12 {
		return Ellipse{}, false // along the axis (lines) or square to it (a circle)
	}
	o := c.F.O.Add(d.Scale(p.F.O.Sub(c.F.O).Dot(n) / cos))
	minor := n.Cross(d).Norm()
	major := minor.Cross(n).Norm()
	return Ellipse{F: Frame{O: o, X: major, Y: minor, Z: major.Cross(minor)}, A: c.R / math.Abs(cos), B: c.R}, true
}

func onEllipse(e Ellipse, pts []Vec3, tol float64) bool {
	for _, p := range pts {
		if e.Eval(e.Project(p)).Dist(p) > tol {
			return false
		}
	}
	return true
}

// snapCircle puts a circle found from points exactly where its surfaces say:
// its centre on a plane square to its axis, its radius a cylinder's, a
// cone's at that height or a sphere's slice (each only if within tol).
func snapCircle(c Circle, tol float64, ss ...Surface) Circle {
	d := c.F.Z
	for _, s := range ss {
		if p, ok := s.(Plane); ok && math.Abs(math.Abs(p.F.Z.Dot(d))-1) < 1e-12 {
			o := c.F.O.Add(d.Scale(p.F.O.Sub(c.F.O).Dot(p.F.Z) / p.F.Z.Dot(d)))
			if o.Dist(c.F.O) <= tol {
				c.F.O = o
			}
		}
	}
	onAxis := func(f Frame) bool {
		if math.Abs(math.Abs(f.Z.Dot(d))-1) > 1e-12 {
			return false
		}
		q := f.O.Sub(c.F.O)
		return q.Sub(d.Scale(q.Dot(d))).Len() <= tol
	}
	for _, s := range ss {
		r := -1.0
		switch k := s.(type) {
		case Cylinder:
			if onAxis(k.F) {
				r = k.R
			}
		case Cone:
			if onAxis(k.F) {
				r = math.Abs(k.radius(c.F.O.Sub(k.F.O).Dot(k.F.Z)))
			}
		case Sphere:
			q := k.F.O.Sub(c.F.O)
			if q.Sub(d.Scale(q.Dot(d))).Len() <= tol {
				if r2 := k.R*k.R - q.Dot(q); r2 > 0 {
					r = math.Sqrt(r2)
				}
			}
		}
		if r > 0 && math.Abs(r-c.R) <= tol {
			c.R = r
		}
	}
	return c
}

// StartAt re-frames a circle or an ellipse so that t = 0 is at p (a point on
// it) and t grows the way it did, or the other way with flip: an arc then
// runs over [0, t1] with t1 ≤ 2π, as the kernel's circle edges do. Other
// curves come back as they are.
func StartAt(c Curve, p Vec3, flip bool) Curve {
	switch k := c.(type) {
	case Circle:
		f := k.F
		if flip {
			f.Y, f.Z = f.Y.Neg(), f.Z.Neg()
		}
		x := p.Sub(f.O)
		x = x.Sub(f.Z.Scale(x.Dot(f.Z)))
		if x.Len() < 1e-300 {
			return c
		}
		f.X = x.Norm()
		f.Y = f.Z.Cross(f.X)
		return Circle{F: f, R: k.R}
	case Ellipse:
		e := k
		if flip {
			e.F.Y, e.F.Z = e.F.Y.Neg(), e.F.Z.Neg()
		}
		e.P = 0
		e.P = e.Project(p)
		return e
	}
	return c
}

// ArcEnd is the parameter of q on a curve re-framed by StartAt: in (0, 2π],
// 2π when full (q is the start again).
func ArcEnd(c Curve, q Vec3, full bool) float64 {
	if full {
		return 2 * math.Pi
	}
	t := c.Project(q)
	if t < 1e-9 {
		t = 2 * math.Pi
	}
	return t
}
