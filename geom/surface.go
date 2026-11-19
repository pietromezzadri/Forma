package geom

import "math"

// Surface is a parametric surface. Its natural normal is ∂u × ∂v; a face
// may use it reversed.
type Surface interface {
	Eval(u, v float64) Vec3
	Normal(u, v float64) Vec3
	// Project returns the parameters of the surface point nearest p; a
	// periodic parameter comes back in [0, period).
	Project(p Vec3) (u, v float64)
	// PeriodU / PeriodV are 0 when the parameter isn't periodic.
	PeriodU() float64
	PeriodV() float64
	// Singular reports a parameter point where u is undefined (a pole or apex).
	Singular(u, v float64) bool
	// Metric is roughly how many mm one unit of u and of v spans, for
	// meshing in a parameter space of near-even proportions.
	Metric() (su, sv float64)
	Kind() string
}

// Plane: F.O + u·F.X + v·F.Y, normal F.Z.
type Plane struct{ F Frame }

func (p Plane) Eval(u, v float64) Vec3       { return p.F.O.Add(p.F.X.Scale(u)).Add(p.F.Y.Scale(v)) }
func (p Plane) Normal(float64, float64) Vec3 { return p.F.Z }
func (p Plane) Project(q Vec3) (float64, float64) {
	l := p.F.ToLocal(q)
	return l.X, l.Y
}
func (Plane) PeriodU() float64               { return 0 }
func (Plane) PeriodV() float64               { return 0 }
func (Plane) Singular(float64, float64) bool { return false }
func (Plane) Metric() (float64, float64)     { return 1, 1 }
func (Plane) Kind() string                   { return "plane" }

// Cylinder of radius R about F.Z: u the angle from F.X, v the height.
type Cylinder struct {
	F Frame
	R float64
}

func (c Cylinder) Eval(u, v float64) Vec3 {
	return c.F.ToWorld(Vec3{c.R * math.Cos(u), c.R * math.Sin(u), v})
}
func (c Cylinder) Normal(u, _ float64) Vec3 { return c.F.Dir(Vec3{math.Cos(u), math.Sin(u), 0}) }
func (c Cylinder) Project(p Vec3) (float64, float64) {
	l := c.F.ToLocal(p)
	return WrapAngle(math.Atan2(l.Y, l.X)), l.Z
}
func (Cylinder) PeriodU() float64               { return 2 * math.Pi }
func (Cylinder) PeriodV() float64               { return 0 }
func (Cylinder) Singular(float64, float64) bool { return false }
func (c Cylinder) Metric() (float64, float64)   { return c.R, 1 }
func (Cylinder) Kind() string                   { return "cylinder" }

// Cone about F.Z: radius R at v = 0 growing by tan(A) per unit of height v.
type Cone struct {
	F Frame
	R float64
	A float64 // half angle; negative narrows upward
}

func (c Cone) radius(v float64) float64 { return c.R + v*math.Tan(c.A) }
func (c Cone) Eval(u, v float64) Vec3 {
	r := c.radius(v)
	return c.F.ToWorld(Vec3{r * math.Cos(u), r * math.Sin(u), v})
}
func (c Cone) Normal(u, v float64) Vec3 {
	t := math.Tan(c.A)
	n := c.F.Dir(Vec3{math.Cos(u), math.Sin(u), -t}).Norm()
	if c.radius(v) < 0 {
		n = n.Neg()
	}
	return n
}
func (c Cone) Project(p Vec3) (float64, float64) {
	l := c.F.ToLocal(p)
	u := WrapAngle(math.Atan2(l.Y, l.X))
	// nearest point on the generator line in the (r, z) half-plane
	r := math.Hypot(l.X, l.Y)
	t := math.Tan(c.A)
	// generator: (R + v t, v); minimise (R + v t - r)^2 + (v - z)^2
	v := ((r-c.R)*t + l.Z) / (1 + t*t)
	return u, v
}
func (Cone) PeriodU() float64 { return 2 * math.Pi }
func (Cone) PeriodV() float64 { return 0 }
func (c Cone) Singular(_, v float64) bool {
	return math.Abs(c.radius(v)) < Tol
}
func (c Cone) Metric() (float64, float64) { return math.Max(math.Abs(c.R), 1e-3), 1 }
func (Cone) Kind() string                 { return "cone" }

// Sphere of radius R round F.O: u the longitude, v the latitude in [-π/2, π/2].
type Sphere struct {
	F Frame
	R float64
}

func (s Sphere) Eval(u, v float64) Vec3 {
	cv := math.Cos(v)
	return s.F.ToWorld(Vec3{s.R * cv * math.Cos(u), s.R * cv * math.Sin(u), s.R * math.Sin(v)})
}
func (s Sphere) Normal(u, v float64) Vec3 {
	cv := math.Cos(v)
	return s.F.Dir(Vec3{cv * math.Cos(u), cv * math.Sin(u), math.Sin(v)})
}
func (s Sphere) Project(p Vec3) (float64, float64) {
	l := s.F.ToLocal(p)
	r := l.Len()
	if r == 0 {
		return 0, 0
	}
	return WrapAngle(math.Atan2(l.Y, l.X)), math.Asin(math.Max(-1, math.Min(1, l.Z/r)))
}
func (Sphere) PeriodU() float64 { return 2 * math.Pi }
func (Sphere) PeriodV() float64 { return 0 }
func (s Sphere) Singular(_, v float64) bool {
	return math.Abs(math.Cos(v))*s.R < Tol
}
func (s Sphere) Metric() (float64, float64) { return s.R, s.R }
func (Sphere) Kind() string                 { return "sphere" }

// Torus round F.Z: major radius R, minor radius Rm; u round the axis, v round the tube.
type Torus struct {
	F     Frame
	R, Rm float64
}

func (t Torus) Eval(u, v float64) Vec3 {
	k := t.R + t.Rm*math.Cos(v)
	return t.F.ToWorld(Vec3{k * math.Cos(u), k * math.Sin(u), t.Rm * math.Sin(v)})
}
func (t Torus) Normal(u, v float64) Vec3 {
	cv := math.Cos(v)
	return t.F.Dir(Vec3{cv * math.Cos(u), cv * math.Sin(u), math.Sin(v)})
}
func (t Torus) Project(p Vec3) (float64, float64) {
	l := t.F.ToLocal(p)
	u := WrapAngle(math.Atan2(l.Y, l.X))
	r := math.Hypot(l.X, l.Y) - t.R
	return u, WrapAngle(math.Atan2(l.Z, r))
}
func (Torus) PeriodU() float64               { return 2 * math.Pi }
func (Torus) PeriodV() float64               { return 2 * math.Pi }
func (Torus) Singular(float64, float64) bool { return false }
func (t Torus) Metric() (float64, float64)   { return t.R + t.Rm, t.Rm }
func (Torus) Kind() string                   { return "torus" }

// Distance from p to surface s (through its projection).
func SurfaceDist(s Surface, p Vec3) float64 {
	u, v := s.Project(p)
	return s.Eval(u, v).Dist(p)
}
