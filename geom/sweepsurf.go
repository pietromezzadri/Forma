package geom

import "math"

// Revolved is a profile curve turned about an axis: the axis is F.Z through
// F.O, and the profile C lies in F's XZ half-plane in F's own coordinates
// (X the distance from the axis, Z the height, Y = 0). u is the angle from
// F.X toward F.Y, v the profile's parameter. Its natural normal is
// ∂S/∂u × ∂S/∂v, which is (h′ cos u, h′ sin u, −r′) in F: to the right of
// the profile's travel in the (r, h) plane. (STEP's surface of revolution,
// with the same parameters and normal.)
type Revolved struct {
	F Frame
	C *BSpline
}

func (s Revolved) rh(v float64) (float64, float64) {
	q := s.C.Eval(v)
	return q.X, q.Z
}

func (s Revolved) Eval(u, v float64) Vec3 {
	r, h := s.rh(v)
	return s.F.O.Add(s.F.X.Scale(r * math.Cos(u))).Add(s.F.Y.Scale(r * math.Sin(u))).Add(s.F.Z.Scale(h))
}

func (s Revolved) Normal(u, v float64) Vec3 {
	d := s.C.Deriv(v)
	n := Vec3{d.Z * math.Cos(u), d.Z * math.Sin(u), -d.X}
	if n.Len() < 1e-300 {
		return s.F.Z
	}
	return s.F.Dir(n.Norm())
}

func (s Revolved) Project(p Vec3) (float64, float64) {
	l := s.F.ToLocal(p)
	u := WrapAngle(math.Atan2(l.Y, l.X))
	v := s.C.Project(Vec3{X: math.Hypot(l.X, l.Y), Z: l.Z})
	return u, v
}

func (Revolved) PeriodU() float64 { return 2 * math.Pi }
func (Revolved) PeriodV() float64 { return 0 }

func (s Revolved) Singular(_, v float64) bool {
	r, _ := s.rh(v)
	return math.Abs(r) < 1e-9
}

func (s Revolved) Metric() (float64, float64) {
	rmax := 1e-3
	for _, c := range s.C.Ctrl {
		rmax = math.Max(rmax, math.Abs(c.X))
	}
	t0, t1 := s.C.Range()
	return rmax, s.C.Length() / math.Max(t1-t0, 1e-12)
}

func (Revolved) Kind() string { return "revolved" }

// Radius is the smallest radius of curvature worth meshing to: the
// profile's tightest bend or its largest distance from the axis.
func (s Revolved) BendRadius() float64 {
	r := s.C.MinBendRadius()
	rmax := 0.0
	for _, c := range s.C.Ctrl {
		rmax = math.Max(rmax, math.Abs(c.X))
	}
	return math.Min(r, math.Max(rmax, 1e-3))
}

// Extruded is a curve swept along a unit direction D: S(u, v) = C(u) + v·D,
// u the curve's parameter, v the distance. The curve lies in a plane square
// to D. Its natural normal is C′(u) × D. (STEP's surface of linear
// extrusion.)
type Extruded struct {
	C *BSpline
	D Vec3
}

func (s Extruded) Eval(u, v float64) Vec3 { return s.C.Eval(u).Add(s.D.Scale(v)) }

func (s Extruded) Normal(u, _ float64) Vec3 {
	n := s.C.Deriv(u).Cross(s.D)
	if n.Len() < 1e-300 {
		return s.D.AnyPerp()
	}
	return n.Norm()
}

func (s Extruded) Project(p Vec3) (float64, float64) {
	o := s.C.Ctrl[0]
	q := p.Sub(s.D.Scale(p.Sub(o).Dot(s.D)))
	u := s.C.Project(q)
	return u, p.Sub(s.C.Eval(u)).Dot(s.D)
}

func (Extruded) PeriodU() float64               { return 0 }
func (Extruded) PeriodV() float64               { return 0 }
func (Extruded) Singular(float64, float64) bool { return false }

func (s Extruded) Metric() (float64, float64) {
	t0, t1 := s.C.Range()
	return s.C.Length() / math.Max(t1-t0, 1e-12), 1
}

func (Extruded) Kind() string { return "extruded" }

// BendRadius is the curve's tightest radius of curvature.
func (s Extruded) BendRadius() float64 { return s.C.MinBendRadius() }
