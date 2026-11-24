package doc

import (
	"math"
	"strings"
	"testing"

	"forma/brep"
	"forma/geom"
)

// the chamfer face's reach back along the top face (z = 20) and the front
// face (y = 0) of a 40 × 30 × 20 box, chamfered on the edge between them
func chamferReach(t *testing.T, b *brep.Body) (top, front float64) {
	t.Helper()
	for _, f := range b.Faces() {
		if !strings.Contains(f.Name, "chamfer:") {
			continue
		}
		top, front = -1, -1
		for _, l := range f.Loops {
			for _, h := range l.HalfEdges() {
				for _, v := range h.Edge.Verts {
					p := v.P
					if math.Abs(p.Z-20) < 1e-6 {
						top = math.Max(top, p.Y)
					}
					if math.Abs(p.Y) < 1e-6 {
						front = math.Max(front, 20-p.Z)
					}
				}
			}
		}
		return top, front
	}
	t.Fatal("no chamfer face")
	return 0, 0
}

// M19: chamfers by two distances, or a distance and an angle; Swap moves
// the distances to the other faces; a chamfer from before keeps its equal
// distances.
func TestChamferTwoDistances(t *testing.T) {
	run := func(set func(f *Feature), keep func(e *brep.Edge) bool) (*Document, *Feature) {
		d := New()
		bx := d.Insert(NewPrim(KBox))
		bx.Inputs["Width"], bx.Inputs["Depth"], bx.Inputs["Height"] = "40", "30", "20"
		d.Regenerate()
		f := d.Insert(NewFillet(edgeRefs(d, keep), true))
		set(f)
		d.Regenerate()
		if f.Err != "" {
			t.Fatal(f.Err)
		}
		if err := brep.Check(d.Bodies[0].Body); err != nil {
			t.Fatal(err)
		}
		return d, f
	}
	// the edge along X at the top front (y = 0, z = 20)
	topFront := func(e *brep.Edge) bool {
		a, b := e.Verts[0].P, e.Verts[1].P
		return math.Abs(a.Y) < 1e-9 && math.Abs(b.Y) < 1e-9 && math.Abs(a.Z-20) < 1e-9 && math.Abs(b.Z-20) < 1e-9
	}
	for _, c := range []struct {
		name       string
		set        func(f *Feature)
		top, front float64 // reach along each face, either way round
	}{
		{"equal", func(f *Feature) { f.Inputs["Distance"] = "2" }, 2, 2},
		{"before types", func(f *Feature) { f.Inputs = map[string]string{"Distance": "3"} }, 3, 3},
		{"two distances", func(f *Feature) {
			f.Inputs["Type"], f.Inputs["Distance"], f.Inputs["Distance 2"] = "Two distances", "2", "5"
		}, 0, 0},
		{"distance and angle", func(f *Feature) {
			f.Inputs["Type"], f.Inputs["Distance"], f.Inputs["Angle"] = "Distance and angle", "2", "30"
		}, 0, 0},
	} {
		d, f := run(c.set, topFront)
		b := d.Bodies[0].Body
		top, front := chamferReach(t, b)
		cut := 24000 - volume(t, b)
		t.Logf("%s: back %.4f along the top, %.4f down the front; %.3f mm³ off", c.name, top, front, cut)
		switch c.name {
		case "equal", "before types":
			if math.Abs(top-c.top) > 1e-6 || math.Abs(front-c.front) > 1e-6 {
				t.Fatalf("%s: %.4f, %.4f", c.name, top, front)
			}
		case "two distances":
			lo, hi := math.Min(top, front), math.Max(top, front)
			if math.Abs(lo-2) > 1e-6 || math.Abs(hi-5) > 1e-6 || math.Abs(cut-0.5*2*5*40) > 1e-6 {
				t.Fatalf("two distances: %.4f, %.4f, cut %.4f", top, front, cut)
			}
			// swapped: the other way round
			f.Inputs["Swap"] = "1"
			d.Regenerate()
			if f.Err != "" {
				t.Fatal(f.Err)
			}
			t2, f2 := chamferReach(t, d.Bodies[0].Body)
			if math.Abs(t2-front) > 1e-6 || math.Abs(f2-top) > 1e-6 {
				t.Fatalf("swap: %.4f, %.4f after %.4f, %.4f", t2, f2, top, front)
			}
		case "distance and angle":
			// 2 along one face at 30° from it: 2·tan 30° along the other (square faces)
			lo, hi := math.Min(top, front), math.Max(top, front)
			if math.Abs(hi-2) > 1e-6 || math.Abs(lo-2*math.Tan(math.Pi/6)) > 1e-6 {
				t.Fatalf("distance and angle: %.4f, %.4f", top, front)
			}
		}
	}
	// every edge of the box, two distances: mitred corners, a valid solid
	run(func(f *Feature) {
		f.Inputs["Type"], f.Inputs["Distance"], f.Inputs["Distance 2"] = "Two distances", "2", "3"
	}, func(*brep.Edge) bool { return true })
	// a cylinder's top edge, two distances: a cone band
	d := New()
	cy := d.Insert(NewPrim(KCylinder))
	cy.Inputs["Radius"], cy.Inputs["Height"] = "10", "20"
	d.Regenerate()
	f := d.Insert(NewFillet(edgeRefs(d, func(e *brep.Edge) bool {
		_, ok := e.Curve.(geom.Circle)
		return ok && e.Verts[0].P.Z > 10
	}), true))
	f.Inputs["Type"], f.Inputs["Distance"], f.Inputs["Distance 2"] = "Two distances", "1", "3"
	d.Regenerate()
	if f.Err != "" {
		t.Fatal(f.Err)
	}
	if err := brep.Check(d.Bodies[0].Body); err != nil {
		t.Fatal(err)
	}
	// an angle the faces can't take
	f.Inputs["Type"], f.Inputs["Angle"] = "Distance and angle", "95"
	d.Regenerate()
	if f.Err == "" {
		t.Fatal("95° from a face at 90° to the other should fail")
	}
	t.Logf("95°: %s", f.Err)
}
