package ops

import (
	"math"
	"testing"

	"forma/geom"
)

// A box and a cylinder split straight and slanting: two valid solids whose
// volumes add up to the whole, each on its side of the plane.
func TestSplitBody(t *testing.T) {
	box, err := Box(geom.V3(0, 0, 0), geom.V3(40, 30, 20))
	if err != nil {
		t.Fatal(err)
	}
	cyl, err := Cylinder(geom.World, 12, 30)
	if err != nil {
		t.Fatal(err)
	}
	slant := geom.Frame{O: geom.V3(0, 0, 15), X: geom.V3(1, 0, 0), Y: geom.V3(0, 1, 1).Norm(), Z: geom.V3(0, -1, 1).Norm()}
	for _, c := range []struct {
		name  string
		v     float64
		pl    geom.Frame
		front float64 // the front part's share
	}{
		{"box at z 5", 0, geom.Frame{O: geom.V3(0, 0, 5), X: geom.XAxis, Y: geom.YAxis, Z: geom.ZAxis}, 0.75},
		{"box at x 10", 0, geom.Frame{O: geom.V3(10, 0, 0), X: geom.YAxis, Y: geom.ZAxis, Z: geom.XAxis}, 0.75},
		{"cylinder slanting", 1, slant, 0.5},
	} {
		b := box
		if c.v == 1 {
			b = cyl
		}
		v0 := checkSolid(t, b)
		f, k, err := SplitBody(b, c.pl, "split")
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		vf, vk := checkSolid(t, f), checkSolid(t, k)
		t.Logf("%s: %.1f + %.1f = %.1f of %.1f", c.name, vf, vk, vf+vk, v0)
		if math.Abs(vf+vk-v0) > 1e-3*v0 || math.Abs(vf/v0-c.front) > 0.01 {
			t.Fatalf("%s: front %.2f, back %.2f of %.2f (front should be %.0f %%)", c.name, vf, vk, v0, 100*c.front)
		}
		named := 0
		for _, fc := range f.Faces() {
			if fc.Name == "split/front" {
				named++
			}
		}
		if named != 1 {
			t.Fatalf("%s: %d faces named split/front", c.name, named)
		}
	}
	// a plane on a face or past the body cuts nothing
	for _, z := range []float64{0, 25} {
		if _, _, err := SplitBody(box, geom.Frame{O: geom.V3(0, 0, z), X: geom.XAxis, Y: geom.YAxis, Z: geom.ZAxis}, "s"); err != ErrSplitMiss {
			t.Fatalf("plane at z %g: %v, want ErrSplitMiss", z, err)
		}
	}
}
