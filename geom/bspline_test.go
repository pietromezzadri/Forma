package geom

import (
	"math"
	"testing"
)

func TestBezierEvalDeriv(t *testing.T) {
	p := []Vec3{{0, 0, 0}, {1, 2, 0}, {3, 3, 1}, {4, 0, 2}}
	b := Bezier(p...)
	bern := func(tt float64) Vec3 {
		s := 1 - tt
		return p[0].Scale(s * s * s).Add(p[1].Scale(3 * s * s * tt)).Add(p[2].Scale(3 * s * tt * tt)).Add(p[3].Scale(tt * tt * tt))
	}
	for _, tt := range []float64{0, 0.1, 0.37, 0.5, 0.9, 1} {
		if d := b.Eval(tt).Dist(bern(tt)); d > 1e-12 {
			t.Fatalf("eval at %g off by %g", tt, d)
		}
		h := 1e-6
		num := bern(math.Min(1, tt+h)).Sub(bern(math.Max(0, tt-h))).Scale(1 / (math.Min(1, tt+h) - math.Max(0, tt-h)))
		tol := 1e-5
		if tt == 0 || tt == 1 {
			tol = 1e-4 // one-sided difference at the ends
		}
		if d := b.Deriv(tt).Dist(num); d > tol {
			t.Fatalf("deriv at %g off by %g", tt, d)
		}
	}
	// projection round trip
	for _, tt := range []float64{0.05, 0.3, 0.71, 0.98} {
		q := b.Eval(tt)
		if got := b.Project(q.Add(b.Deriv(tt).Cross(Vec3{0, 0, 1}).Norm().Scale(1e-3))); math.Abs(got-tt) > 1e-3 {
			t.Fatalf("project %g got %g", tt, got)
		}
	}
	// splitting keeps the curve
	s := b.SubBezier(0.2, 0.7)
	for _, u := range []float64{0, 0.25, 0.5, 1} {
		if d := s.Eval(u).Dist(b.Eval(0.2 + 0.5*u)); d > 1e-12 {
			t.Fatalf("sub-Bézier at %g off by %g", u, d)
		}
	}
	// a straight one's length
	if l := Bezier(Vec3{}, Vec3{X: 1}, Vec3{X: 3}).Length(); math.Abs(l-3) > 1e-9 {
		t.Fatalf("length %g", l)
	}
	// reversed
	r := b.Reversed()
	if d := r.Eval(0.3).Dist(b.Eval(0.7)); d > 1e-12 {
		t.Fatalf("reversed off by %g", d)
	}
}

func TestBSplineMultiSpan(t *testing.T) {
	// a uniform clamped cubic with interior knots: evaluating at a knot
	// matches both neighbouring spans
	b := &BSpline{Deg: 3, Knots: []float64{0, 0, 0, 0, 1, 2, 3, 3, 3, 3},
		Ctrl: []Vec3{{0, 0, 0}, {1, 2, 0}, {2, -1, 0}, {3, 3, 0}, {4, 0, 0}, {5, 1, 0}}}
	for _, k := range []float64{1, 2} {
		a, c := b.Eval(k-1e-9), b.Eval(k+1e-9)
		if a.Dist(c) > 1e-7 || b.Eval(k).Dist(a) > 1e-7 {
			t.Fatalf("not continuous at knot %g", k)
		}
	}
	if b.Eval(0).Dist(b.Ctrl[0]) > 1e-12 || b.Eval(3).Dist(b.Ctrl[5]) > 1e-12 {
		t.Fatal("clamped ends don't interpolate")
	}
	ts := SampleParams(b, 0, 3, 1e-3, math.Inf(1))
	for i := 0; i+1 < len(ts); i++ {
		m := b.Eval((ts[i] + ts[i+1]) / 2)
		a, c := b.Eval(ts[i]), b.Eval(ts[i+1])
		l := c.Sub(a)
		d := m.Sub(a).Cross(l).Len() / l.Len()
		if d > 1e-3*1.01 {
			t.Fatalf("chord %d off by %g", i, d)
		}
	}
}

// A straight profile turned is a cylinder; a sloped one a cone.
func TestRevolvedMatchesCylinder(t *testing.T) {
	F := World
	rv := Revolved{F: F, C: Bezier(Vec3{X: 5}, Vec3{X: 5, Z: 4}, Vec3{X: 5, Z: 10})}
	cy := Cylinder{F: F, R: 5}
	for _, p := range []Vec3{{7, 1, 3}, {-2, 4, 6}, {0.5, -3, 2}} {
		if d := SignedDist(rv, p) - SignedDist(cy, p); math.Abs(d) > 1e-9 {
			t.Fatalf("signed distance at %v differs by %g", p, d)
		}
		u, v := rv.Project(p)
		cu, _ := cy.Project(p)
		if math.Abs(u-cu) > 1e-12 {
			t.Fatalf("u %g vs %g", u, cu)
		}
		if n := rv.Normal(u, v).Dot(cy.Normal(cu, 0)); n < 1-1e-12 {
			t.Fatalf("normals differ at %v", p)
		}
	}
}

func TestExtrudedMatchesPlane(t *testing.T) {
	// a straight curve along X swept along Z: the plane y = 0, normal
	// X × Z = −Y
	ex := Extruded{C: Bezier(Vec3{}, Vec3{X: 2}, Vec3{X: 6}), D: Vec3{Z: 1}}
	for _, p := range []Vec3{{3, 2, 5}, {1, -1, -2}} {
		if d := SignedDist(ex, p) + p.Y; math.Abs(d) > 1e-9 {
			t.Fatalf("signed distance at %v off by %g", p, d)
		}
		u, v := ex.Project(p)
		if q := ex.Eval(u, v); math.Abs(q.X-p.X) > 1e-9 || math.Abs(q.Z-p.Z) > 1e-9 || math.Abs(q.Y) > 1e-12 {
			t.Fatalf("project %v → %v", p, q)
		}
	}
	// a reflection keeps the natural normal mapped with the surface
	m := Mirror(Vec3{}, Vec3{X: 1})
	ey := m.Surface(ex).(Extruded)
	u, v := ex.Project(Vec3{3, 0, 1})
	u2, v2 := ey.Project(m.Pt(Vec3{3, 0, 1}))
	if d := ey.Normal(u2, v2).Dist(m.Dir(ex.Normal(u, v))); d > 1e-9 {
		t.Fatalf("mirrored normal off by %g", d)
	}
}

// Past its curve's ends an extruded or revolved spline goes on along the
// end tangents: a point there projects to a parameter beyond the curve's
// range that evaluates back to it, and the distance runs smoothly through
// the end (it used to jump in sign there, past the end, which a contour
// followed as if it were an intersection).
func TestSweptSurfacesCarryOnPastTheEnds(t *testing.T) {
	c := Bezier(Vec3{X: 60}, Vec3{X: 84, Z: 10}, Vec3{X: 82, Z: 42}, Vec3{X: 62, Z: 55})
	t0, t1 := c.Range()
	ex := Extruded{C: c, D: Vec3{Y: 1}}
	rv := Revolved{F: World, C: c}
	end, dir := c.Eval(t1), c.Deriv(t1).Norm()
	side := Vec3{X: dir.Z, Z: -dir.X} // the profile's normal, in its plane
	for _, k := range []float64{0.5, 3, 10} {
		// on the tangent past the end, and a little off it
		on := end.Add(dir.Scale(k))
		off := on.Add(side.Scale(0.7))
		for name, s := range map[string]Surface{"extruded": ex, "revolved": rv} {
			u, v := s.Project(on)
			if q := s.Eval(u, v); q.Dist(on) > 1e-9 {
				t.Fatalf("%s: %v past the end projects back to %v", name, on, q)
			}
			pu := u
			if name == "revolved" {
				pu = v
			}
			if pu <= t1 {
				t.Fatalf("%s: a point past the end has parameter %g in the range [%g, %g]", name, pu, t0, t1)
			}
			if d := SignedDist(s, on); math.Abs(d) > 1e-9 {
				t.Fatalf("%s: %g from the carried-on surface on it", name, d)
			}
			if d := math.Abs(SignedDist(s, off)); math.Abs(d-0.7) > 1e-6 {
				t.Fatalf("%s: %g from the carried-on surface 0.7 off it", name, d)
			}
		}
	}
	// within the range nothing changed
	mid := c.Eval((t0 + t1) / 2)
	if u, _ := ex.Project(mid); math.Abs(u-(t0+t1)/2) > 1e-6 {
		t.Fatalf("the middle projects to %g", u)
	}
}
