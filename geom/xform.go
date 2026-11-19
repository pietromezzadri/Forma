package geom

import "math"

// Xform is a rigid motion or a reflection: p ↦ R·p + T, R orthonormal (its
// columns are where the axes go).
type Xform struct {
	R [3]Vec3 // columns
	T Vec3
}

// Identity is the transform that leaves everything where it is.
var Identity = Xform{R: [3]Vec3{XAxis, YAxis, ZAxis}}

// Dir maps a direction.
func (x Xform) Dir(d Vec3) Vec3 {
	return x.R[0].Scale(d.X).Add(x.R[1].Scale(d.Y)).Add(x.R[2].Scale(d.Z))
}

// Pt maps a point.
func (x Xform) Pt(p Vec3) Vec3 { return x.Dir(p).Add(x.T) }

// Det is +1 for a rigid motion, −1 for a reflection.
func (x Xform) Det() float64 { return x.R[0].Dot(x.R[1].Cross(x.R[2])) }

// Then is x followed by y.
func (x Xform) Then(y Xform) Xform {
	return Xform{R: [3]Vec3{y.Dir(x.R[0]), y.Dir(x.R[1]), y.Dir(x.R[2])}, T: y.Pt(x.T)}
}

// Inverse undoes x.
func (x Xform) Inverse() Xform {
	// Rᵀ: rows become columns
	r := [3]Vec3{
		{x.R[0].X, x.R[1].X, x.R[2].X},
		{x.R[0].Y, x.R[1].Y, x.R[2].Y},
		{x.R[0].Z, x.R[1].Z, x.R[2].Z},
	}
	inv := Xform{R: r}
	inv.T = inv.Dir(x.T).Neg()
	return inv
}

// Translate moves by d.
func Translate(d Vec3) Xform { x := Identity; x.T = d; return x }

// Rotate turns by angle (radians, right hand) about the axis through p along d.
func Rotate(p, d Vec3, angle float64) Xform {
	d = d.Norm()
	c, s := math.Cos(angle), math.Sin(angle)
	rot := func(v Vec3) Vec3 {
		// Rodrigues
		return v.Scale(c).Add(d.Cross(v).Scale(s)).Add(d.Scale(d.Dot(v) * (1 - c)))
	}
	x := Xform{R: [3]Vec3{rot(XAxis), rot(YAxis), rot(ZAxis)}}
	x.T = p.Sub(x.Dir(p))
	return x
}

// Mirror reflects in the plane through p with normal n.
func Mirror(p, n Vec3) Xform {
	n = n.Norm()
	ref := func(v Vec3) Vec3 { return v.Sub(n.Scale(2 * v.Dot(n))) }
	x := Xform{R: [3]Vec3{ref(XAxis), ref(YAxis), ref(ZAxis)}}
	x.T = p.Sub(x.Dir(p))
	return x
}

// Frame maps a frame keeping it right-handed: a reflection turns Y over, so
// a surface's parameters change sign (see Surface mapping in brep).
func (x Xform) Frame(f Frame) Frame {
	g := Frame{O: x.Pt(f.O), X: x.Dir(f.X), Y: x.Dir(f.Y), Z: x.Dir(f.Z)}
	if x.Det() < 0 {
		g.Y = g.Y.Neg()
	}
	return g
}

// Curve maps a curve so its parameters stay as they were (t ↦ x(c(t))).
func (x Xform) Curve(c Curve) Curve {
	switch k := c.(type) {
	case Line:
		return Line{P: x.Pt(k.P), D: x.Dir(k.D)}
	case Circle:
		f := Frame{O: x.Pt(k.F.O), X: x.Dir(k.F.X), Y: x.Dir(k.F.Y)}
		f.Z = f.X.Cross(f.Y)
		return Circle{F: f, R: k.R}
	case *Polyline:
		pts := make([]Vec3, len(k.Pts))
		for i, p := range k.Pts {
			pts[i] = x.Pt(p)
		}
		return NewPolyline(pts, k.Tol)
	}
	return c
}

// Surface maps a surface. Its natural normal maps with it (n ↦ R·n), so a
// face keeps its Reversed flag; under a reflection the parameters change
// (planes v ↦ −v, the surfaces of revolution u ↦ −u), which nothing relies
// on: faces are meshed from their edges by projection.
func (x Xform) Surface(s Surface) Surface {
	switch k := s.(type) {
	case Plane:
		return Plane{F: x.Frame(k.F)}
	case Cylinder:
		return Cylinder{F: x.Frame(k.F), R: k.R}
	case Cone:
		return Cone{F: x.Frame(k.F), R: k.R, A: k.A}
	case Sphere:
		return Sphere{F: x.Frame(k.F), R: k.R}
	case Torus:
		return Torus{F: x.Frame(k.F), R: k.R, Rm: k.Rm}
	}
	return s
}
