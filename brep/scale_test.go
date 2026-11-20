package brep_test

import (
	"math"
	"testing"

	"forma/brep"
	"forma/geom"
)

// Scaling a drilled block by 2 about a point: a valid solid of 8× the
// volume, its hole's rim a circle of twice the radius.
func TestScaleBody(t *testing.T) {
	r := boolCase(t, "hole", box(t, 0, 0, 0, 40, 30, 20), cyl(t, 20, 15, -5, 3, 30), brep.Subtract, 24000-math.Pi*9*20)
	s, err := brep.Scale(r, geom.V3(5, 5, 5), 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := brep.Check(s); err != nil {
		t.Fatal(err)
	}
	if v := vol(t, s); math.Abs(v-8*(24000-math.Pi*9*20)) > 1e-3*v {
		t.Fatalf("volume %.3f", v)
	}
	for _, e := range s.Edges() {
		if c, ok := e.Curve.(geom.Circle); ok && math.Abs(c.R-6) > 1e-12 {
			t.Fatalf("rim radius %g", c.R)
		}
	}
	onSurfaces(t, s, 1e-9)
}
