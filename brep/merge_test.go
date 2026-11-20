package brep_test

import (
	"math"
	"testing"

	"forma/brep"
)

func counts(b *brep.Body) [5]int {
	v, e, f, r, s := b.Counts()
	return [5]int{v, e, f, r, s}
}

// Two blocks side by side join into one block: six faces, no seam on top.
func TestMergeSideBySide(t *testing.T) {
	r := boolCase(t, "two blocks", box(t, 0, 0, 0, 10, 10, 10), box(t, 10, 0, 0, 20, 10, 10), brep.Union, 2000)
	if c := counts(r); c != [5]int{8, 12, 6, 0, 1} {
		t.Fatalf("V E F rings shells %v, want a plain block", c)
	}
	for _, e := range r.Edges() {
		if e.Curve.Kind() != "line" {
			t.Fatalf("a %s edge", e.Curve.Kind())
		}
	}
}

// An L: the top is one six-cornered face; the faces flush along the long
// side merge and their edges run on through.
func TestMergeL(t *testing.T) {
	r := boolCase(t, "L", box(t, 0, 0, 0, 20, 10, 5), box(t, 0, 10, 0, 10, 20, 5), brep.Union, 1500)
	// an L prism: 8 faces, 12 vertices, 18 edges
	if c := counts(r); c != [5]int{12, 18, 8, 0, 1} {
		t.Fatalf("V E F rings shells %v, want an L prism", c)
	}
}

// Four blocks round a square: the top is a ring with a hole.
func TestMergeRing(t *testing.T) {
	r := box(t, 0, 0, 0, 30, 10, 4)
	for _, b := range [][6]float64{{20, 10, 0, 30, 30, 4}, {0, 20, 0, 20, 30, 4}, {0, 10, 0, 10, 20, 4}} {
		var err error
		if r, err = brep.Boolean(r, box(t, b[0], b[1], b[2], b[3], b[4], b[5]), brep.Union); err != nil {
			t.Fatal(err)
		}
	}
	if v := vol(t, r); math.Abs(v-(900-100)*4) > 1e-6 {
		t.Fatalf("volume %.4f", v)
	}
	// a square frame: top and bottom with a hole, 4 + 4 sides
	if c := counts(r); c != [5]int{16, 24, 10, 2, 1} {
		t.Fatalf("V E F rings shells %v, want a square frame", c)
	}
}

// A hole bored across where two blocks were joined is one plain hole: the
// merged top has no seam left to split its rim.
func TestMergeThenDrill(t *testing.T) {
	two, err := brep.Boolean(box(t, 0, 0, 0, 10, 10, 10), box(t, 10, 0, 0, 20, 10, 10), brep.Union)
	if err != nil {
		t.Fatal(err)
	}
	r := boolCase(t, "hole across the join", two, cyl(t, 10, 5, -2, 3, 14), brep.Subtract, 2000-math.Pi*9*10)
	n := curves(r)
	if n["circle"] != 2 || n["polyline"] != 0 {
		t.Fatalf("edges %v: want one whole circle round each rim", n)
	}
	if c := counts(r); c[2] != 7 || c[3] != 2 {
		t.Fatalf("V E F rings shells %v", c)
	}
}
