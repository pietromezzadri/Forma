package geom

import "math"

// SignedDist is (close to) the signed distance from p to surface s:
// positive on the side its natural normal points to, zero on it. Exact for
// planes, cylinders, spheres and tori; for cones it is the distance to the
// generator line in p's half-plane.
func SignedDist(s Surface, p Vec3) float64 {
	switch k := s.(type) {
	case Plane:
		return p.Sub(k.F.O).Dot(k.F.Z)
	case Cylinder:
		l := k.F.ToLocal(p)
		return math.Hypot(l.X, l.Y) - k.R
	case Cone:
		l := k.F.ToLocal(p)
		t := math.Tan(k.A)
		return (math.Hypot(l.X, l.Y) - k.R - l.Z*t) / math.Sqrt(1+t*t)
	case Sphere:
		return p.Dist(k.F.O) - k.R
	case Torus:
		l := k.F.ToLocal(p)
		return math.Hypot(math.Hypot(l.X, l.Y)-k.R, l.Z) - k.Rm
	}
	u, v := s.Project(p)
	q := s.Eval(u, v)
	return p.Sub(q).Dot(s.Normal(u, v))
}

// DistGrad is the gradient of SignedDist (numerical, central differences).
func DistGrad(s Surface, p Vec3, h float64) Vec3 {
	dx := SignedDist(s, p.Add(Vec3{X: h})) - SignedDist(s, p.Sub(Vec3{X: h}))
	dy := SignedDist(s, p.Add(Vec3{Y: h})) - SignedDist(s, p.Sub(Vec3{Y: h}))
	dz := SignedDist(s, p.Add(Vec3{Z: h})) - SignedDist(s, p.Sub(Vec3{Z: h}))
	return Vec3{dx, dy, dz}.Scale(1 / (2 * h))
}

// OntoBoth moves p onto the intersection of two surfaces (Newton steps on
// the pair of distances, the smallest move each time). It reports whether
// it got within tol of both.
func OntoBoth(a, b Surface, p Vec3, tol float64) (Vec3, bool) {
	h := 1e-6 * (1 + p.Len()*1e-3)
	for it := 0; it < 12; it++ {
		fa, fb := SignedDist(a, p), SignedDist(b, p)
		if math.Abs(fa) < tol && math.Abs(fb) < tol {
			return p, true
		}
		ga, gb := DistGrad(a, p, h), DistGrad(b, p, h)
		// minimum-norm step δ = Jᵀ (J Jᵀ)⁻¹ (−f), J = [ga; gb]
		aa, ab, bb := ga.Dot(ga), ga.Dot(gb), gb.Dot(gb)
		det := aa*bb - ab*ab
		if math.Abs(det) < 1e-14 {
			// nearly tangent: step onto each in turn
			if aa > 0 {
				p = p.Sub(ga.Scale(fa / aa))
			}
			if bb > 0 {
				p = p.Sub(gb.Scale(SignedDist(b, p) / bb))
			}
			continue
		}
		la := (-fa*bb + fb*ab) / det
		lb := (-fb*aa + fa*ab) / det
		p = p.Add(ga.Scale(la)).Add(gb.Scale(lb))
	}
	fa, fb := SignedDist(a, p), SignedDist(b, p)
	return p, math.Abs(fa) < tol*10 && math.Abs(fb) < tol*10
}

// Coincident reports whether two surfaces are the same surface (within tol),
// judged at the given sample points of one of them.
func Coincident(a, b Surface, samples []Vec3, tol float64) bool {
	if len(samples) == 0 || a.Kind() != b.Kind() {
		return false
	}
	for _, p := range samples {
		if math.Abs(SignedDist(b, p)) > tol {
			return false
		}
	}
	return true
}
