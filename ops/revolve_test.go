package ops

import (
	"math"
	"testing"

	"forma/brep"
	"forma/geom"
	"forma/sketch"
)

var yAxisP, yAxisD = geom.V2(0, 0), geom.V2(0, 1)

func revolveCase(t *testing.T, s *sketch.Sketch, angle, want float64, plane geom.Frame) *brep.Body {
	t.Helper()
	rs := s.Regions()
	if len(rs) == 0 {
		t.Fatal("no region")
	}
	b, err := Revolve("r", rs[0], plane, yAxisP, yAxisD, angle)
	if err != nil {
		t.Fatal(err)
	}
	v := checkSolid(t, b)
	if math.Abs(v-want)/want > 2e-3 {
		t.Fatalf("volume %.3f, want %.3f (%.3f%%)", v, want, 100*(v-want)/want)
	}
	return b
}

func TestRevolveTube(t *testing.T) {
	s := sketch.New("s", geom.World)
	rectSketch(s, 10, 0, 20, 30)
	revolveCase(t, s, 2*math.Pi, math.Pi*(400-100)*30, geom.World)
	// and a quarter of it
	b := revolveCase(t, s, math.Pi/2, math.Pi*(400-100)*30/4, geom.World)
	names := map[string]bool{}
	for _, f := range b.Faces() {
		names[f.Name] = true
	}
	if !names["start"] || !names["end"] {
		t.Fatalf("no caps: %v", names)
	}
}

// Touching the axis: a solid cylinder (discs at the ends, no hole).
func TestRevolveCylinder(t *testing.T) {
	s := sketch.New("s", geom.World)
	rectSketch(s, 0, 0, 15, 20)
	b := revolveCase(t, s, 2*math.Pi, math.Pi*225*20, geom.World)
	if g := brep.Genus(b); g != 0 {
		t.Fatalf("genus %d", g)
	}
	revolveCase(t, s, math.Pi*0.75, math.Pi*225*20*0.375, geom.World)
}

// A half disc round its diameter: a sphere; a triangle: a cone.
func TestRevolveSphereAndCone(t *testing.T) {
	s := sketch.New("s", geom.World)
	c := s.AddPoint(geom.V2(0, 0)).ID
	lo, hi := s.AddPoint(geom.V2(0, -10)).ID, s.AddPoint(geom.V2(0, 10)).ID
	s.AddArc(c, lo, hi) // the right half
	s.AddLine(hi, lo)
	revolveCase(t, s, 2*math.Pi, 4.0/3*math.Pi*1000, geom.World)
	revolveCase(t, s, math.Pi, 2.0/3*math.Pi*1000, geom.World)

	k := sketch.New("k", geom.World)
	p := func(x, y float64) int { return k.AddPoint(geom.V2(x, y)).ID }
	a, b, d := p(0, 0), p(10, 0), p(0, 20)
	k.AddLine(a, b)
	k.AddLine(b, d)
	k.AddLine(d, a)
	revolveCase(t, k, 2*math.Pi, math.Pi*100*20/3, geom.World)
}

// A circle away from the axis: a torus; on a tilted plane.
func TestRevolveTorus(t *testing.T) {
	plane := geom.FrameZ(geom.V3(5, 5, 5), geom.V3(1, 2, 3), geom.V3(1, 0, 0))
	s := sketch.New("s", plane)
	s.AddCircle(s.AddPoint(geom.V2(30, 4)).ID, 8)
	b := revolveCase(t, s, 2*math.Pi, 2*math.Pi*math.Pi*30*64, plane)
	if g := brep.Genus(b); g != 1 {
		t.Fatalf("genus %d, want 1", g)
	}
	revolveCase(t, s, math.Pi/3, 2*math.Pi*math.Pi*30*64/6, plane)
}

// On the other side of the axis, and crossing it.
func TestRevolveSides(t *testing.T) {
	s := sketch.New("s", geom.World)
	rectSketch(s, -20, 0, -10, 30)
	revolveCase(t, s, 2*math.Pi, math.Pi*(400-100)*30, geom.World)

	x := sketch.New("x", geom.World)
	rectSketch(x, -5, 0, 10, 30)
	if _, err := Revolve("r", x.Regions()[0], geom.World, yAxisP, yAxisD, math.Pi); err == nil {
		t.Fatal("a profile crossing the axis was revolved")
	}
}
