package geom

import "math"

// Curve is a parametric 3D curve. Edges use a [t0, t1] range of it.
type Curve interface {
	Eval(t float64) Vec3
	Deriv(t float64) Vec3
	// Project returns the parameter of the curve point nearest p.
	Project(p Vec3) float64
	Kind() string
}

// Line is P + t·D with D a unit vector: t is the distance from P.
type Line struct{ P, D Vec3 }

// LineThrough is the line from a to b, parametrised so t = 0 at a, t = |ab| at b.
func LineThrough(a, b Vec3) (Line, float64) {
	d := b.Sub(a)
	return Line{P: a, D: d.Norm()}, d.Len()
}

func (l Line) Eval(t float64) Vec3    { return l.P.Add(l.D.Scale(t)) }
func (l Line) Deriv(float64) Vec3     { return l.D }
func (l Line) Project(p Vec3) float64 { return p.Sub(l.P).Dot(l.D) }
func (Line) Kind() string             { return "line" }

// Circle lies in F's XY plane round F.O, t the angle from F.X toward F.Y.
type Circle struct {
	F Frame
	R float64
}

func (c Circle) Eval(t float64) Vec3 {
	return c.F.O.Add(c.F.X.Scale(c.R * math.Cos(t))).Add(c.F.Y.Scale(c.R * math.Sin(t)))
}
func (c Circle) Deriv(t float64) Vec3 {
	return c.F.X.Scale(-c.R * math.Sin(t)).Add(c.F.Y.Scale(c.R * math.Cos(t)))
}
func (c Circle) Project(p Vec3) float64 {
	l := c.F.ToLocal(p)
	return WrapAngle(math.Atan2(l.Y, l.X))
}
func (Circle) Kind() string { return "circle" }

// SampleParams splits [t0, t1] of c into steps whose chord stays within
// chordTol of the curve and no longer than maxLen. Both ends are included.
func SampleParams(c Curve, t0, t1, chordTol, maxLen float64) []float64 {
	n := 1
	switch k := c.(type) {
	case Line:
		n = int(math.Ceil(math.Abs(t1-t0) / maxLen))
	case Circle:
		step := ArcStep(k.R, chordTol)
		n = int(math.Ceil(math.Abs(t1-t0) / step))
		if m := int(math.Ceil(math.Abs(t1-t0) * k.R / maxLen)); m > n {
			n = m
		}
		if full := math.Abs(t1-t0) > math.Pi*1.999; full && n < 8 {
			n = 8
		}
	case Ellipse:
		// the tightest bend has radius B²/A (A ≥ B) or A²/B
		rmin := math.Min(k.B*k.B/math.Max(k.A, 1e-300), k.A*k.A/math.Max(k.B, 1e-300))
		n = int(math.Ceil(math.Abs(t1-t0) / ArcStep(rmin, chordTol)))
		if m := int(math.Ceil(math.Abs(t1-t0) * math.Max(k.A, k.B) / maxLen)); m > n {
			n = m
		}
		if full := math.Abs(t1-t0) > math.Pi*1.999; full && n < 8 {
			n = 8
		}
	case *Polyline:
		lo, hi := math.Min(t0, t1), math.Max(t0, t1)
		ts := []float64{lo}
		for _, t := range k.cum {
			if t > lo+1e-12 && t < hi-1e-12 {
				ts = append(ts, t)
			}
		}
		ts = append(ts, hi)
		if t1 < t0 {
			for i, j := 0, len(ts)-1; i < j; i, j = i+1, j-1 {
				ts[i], ts[j] = ts[j], ts[i]
			}
		}
		return ts
	default:
		n = 32
	}
	if n < 1 {
		n = 1
	}
	ts := make([]float64, n+1)
	for i := range ts {
		ts[i] = t0 + (t1-t0)*float64(i)/float64(n)
	}
	ts[n] = t1
	return ts
}

// ArcStep is the largest angle step on a circle of radius r whose chord
// stays within tol of the arc.
func ArcStep(r, tol float64) float64 {
	if r <= tol {
		return math.Pi / 2
	}
	s := 2 * math.Acos(1-tol/r)
	if s > math.Pi/8 {
		s = math.Pi / 8
	}
	if s < 2*math.Pi/720 {
		s = 2 * math.Pi / 720
	}
	return s
}
