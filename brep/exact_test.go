package brep_test

import (
	"math"
	"testing"

	"forma/brep"
	"forma/geom"
	"forma/ops"
)

// curves counts a body's edges by curve kind.
func curves(b *brep.Body) map[string]int {
	n := map[string]int{}
	for _, e := range b.Edges() {
		n[e.Curve.Kind()]++
	}
	return n
}

// onSurfaces: every edge's curve lies on both its faces' surfaces.
func onSurfaces(t *testing.T, b *brep.Body, tol float64) {
	t.Helper()
	for _, e := range b.Edges() {
		for i := 0; i <= 16; i++ {
			p := e.Curve.Eval(e.T0 + (e.T1-e.T0)*float64(i)/16)
			for _, h := range e.HE {
				if d := math.Abs(geom.SignedDist(h.Loop.Face.Surf, p)); d > tol {
					t.Fatalf("%s edge %d is %.3g mm off face %q", e.Curve.Kind(), e.ID, d, h.Loop.Face.Name)
				}
			}
		}
	}
}

// A drilled hole's rims are true circles of the drill's radius.
func TestExactHoleRims(t *testing.T) {
	r := boolCase(t, "hole", box(t, 0, 0, 0, 40, 30, 20), cyl(t, 20, 15, -5, 3, 30), brep.Subtract, 24000-math.Pi*9*20)
	if n := curves(r); n["circle"] != 2 || n["polyline"] != 0 {
		t.Fatalf("edges %v", n)
	}
	for _, e := range r.Edges() {
		if c, ok := e.Curve.(geom.Circle); ok {
			if math.Abs(c.R-3) > 1e-12 || math.Hypot(c.F.O.X-20, c.F.O.Y-15) > 1e-12 || math.Abs(c.F.O.Z-10) != 10 {
				t.Fatalf("rim r %v at %v", c.R, c.F.O)
			}
		}
	}
	onSurfaces(t, r, 1e-12)
	// the rim can be rounded: the fillet tool takes the boolean's circle
	var rim *brep.Edge
	for _, e := range r.Edges() {
		if c, ok := e.Curve.(geom.Circle); ok && c.F.O.Z > 19 {
			rim = e
		}
	}
	tool, op, err := ops.EdgeTool(rim, 1, false, 1, [2]bool{})
	if err != nil {
		t.Fatal(err)
	}
	rounded, err := brep.Boolean(r, tool, op)
	if err != nil {
		t.Fatal(err)
	}
	// a round of radius 1 on a hole of radius 3 takes the corner's ring:
	// (1 − π/4) r² about the circle through the corner's centroid
	ring := (1 - math.Pi/4) * 2 * math.Pi * (3 + (10-3*math.Pi)/(12-3*math.Pi))
	vol(t, rounded)
	if cut := brep.Mass(r, 1e-5).Volume - brep.Mass(rounded, 1e-5).Volume; math.Abs(cut-ring) > 0.01*ring {
		t.Fatalf("rounding the rim took %.4f, want %.4f", cut, ring)
	}
}

// A plane cutting a cylinder at a slant meets it in an ellipse.
func TestExactSlantEllipse(t *testing.T) {
	c := cyl(t, 0, 0, -20, 4, 40)
	// a block tilted 30° about X: its top face cuts the cylinder slanted
	blk := box(t, -50, -50, -100, 50, 50, 0)
	tilt, err := brep.Transform(blk, geom.Rotate(geom.Vec3{}, geom.XAxis, math.Pi/6), nil)
	if err != nil {
		t.Fatal(err)
	}
	r, err := brep.Boolean(c, tilt, brep.Subtract)
	if err != nil {
		t.Fatal(err)
	}
	n := curves(r)
	if n["ellipse"] == 0 || n["polyline"] != 0 {
		t.Fatalf("edges %v", n)
	}
	for _, e := range r.Edges() {
		if el, ok := e.Curve.(geom.Ellipse); ok {
			if math.Abs(el.B-4) > 1e-12 || math.Abs(el.A-4/math.Cos(math.Pi/6)) > 1e-12 {
				t.Fatalf("ellipse %g × %g", el.A, el.B)
			}
		}
	}
	onSurfaces(t, r, 1e-9)
	// the volume above the slanted plane: half the cylinder's (the plane goes
	// through the axis at the middle)
	if v := vol(t, r); math.Abs(v-math.Pi*16*20) > 0.05*math.Pi*16*20/100 {
		t.Fatalf("volume %.4f, want %.4f", v, math.Pi*16*20)
	}
}

// A plane through a sphere cuts it in a circle; two curved surfaces with
// no closed form keep a polyline.
func TestExactSphereSlice(t *testing.T) {
	s, err := ops.Sphere(geom.Vec3{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	r, err := brep.Boolean(s, box(t, -20, -20, 3, 20, 20, 20), brep.Subtract)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range r.Edges() {
		if c, ok := e.Curve.(geom.Circle); ok && math.Abs(c.F.O.Z-3) < 1e-12 {
			found = true
			if math.Abs(c.R-math.Sqrt(91)) > 1e-12 {
				t.Fatalf("slice r %v, want √91", c.R)
			}
		}
	}
	if !found {
		t.Fatalf("no circle at z 3: %v", curves(r))
	}
	onSurfaces(t, r, 1e-9)

	// crossing cylinders of two radii: a curve of degree four
	a := cyl(t, 0, 0, -20, 5, 40)
	f := geom.World
	f.O = geom.V3(-20, 0, 0)
	f.Z, f.X, f.Y = geom.XAxis, geom.YAxis, geom.ZAxis
	b, err := ops.Cylinder(f, 3, 40)
	if err != nil {
		t.Fatal(err)
	}
	u, err := brep.Boolean(a, b, brep.Union)
	if err != nil {
		t.Fatal(err)
	}
	if n := curves(u); n["polyline"] == 0 {
		t.Fatalf("crossing cylinders: edges %v", n)
	}
}
