package ops

import (
	"math"
	"strings"
	"testing"

	"forma/brep"
	"forma/geom"
	"forma/sketch"
)

// checkSolid validates a body and its mesh and returns the mesh volume.
func checkSolid(t *testing.T, b *brep.Body) float64 {
	t.Helper()
	if err := brep.Check(b); err != nil {
		t.Fatal(err)
	}
	opt := brep.DefaultMeshOptions(b)
	opt.ChordTol = b.Bounds().Diagonal() * 2e-5
	m := brep.Tessellate(b, opt)
	if err := m.Watertight(1e-6); err != nil {
		t.Fatalf("mesh: %v", err)
	}
	if v := m.Volume(); v <= 0 {
		t.Fatalf("volume %.3f: inside out", v)
	}
	return m.Volume()
}

func rectSketch(s *sketch.Sketch, x0, y0, x1, y1 float64) {
	a, b := s.AddPoint(geom.V2(x0, y0)).ID, s.AddPoint(geom.V2(x1, y0)).ID
	c, d := s.AddPoint(geom.V2(x1, y1)).ID, s.AddPoint(geom.V2(x0, y1)).ID
	s.AddLine(a, b)
	s.AddLine(b, c)
	s.AddLine(c, d)
	s.AddLine(d, a)
}

func TestExtrudeRectangle(t *testing.T) {
	s := sketch.New("s", geom.World)
	rectSketch(s, 0, 0, 40, 30)
	rs := s.Regions()
	b, err := Extrude("e", rs[0], geom.World, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if v := checkSolid(t, b); math.Abs(v-24000) > 1e-6 {
		t.Fatalf("volume %.6f", v)
	}
	if v, e, f, _, _ := b.Counts(); v != 8 || e != 12 || f != 6 {
		t.Fatalf("V%d E%d F%d", v, e, f)
	}
	names := map[string]bool{}
	for _, f := range b.Faces() {
		names[f.Name] = true
	}
	if !names["top"] || !names["bottom"] {
		t.Fatalf("faces %v", names)
	}
}

// A plate with a round hole and a slot, on a tilted plane, extruded both ways.
func TestExtrudePlateWithHoles(t *testing.T) {
	plane := geom.FrameZ(geom.V3(10, -5, 3), geom.V3(0.3, -0.2, 1), geom.V3(1, 0.4, 0))
	s := sketch.New("s", plane)
	rectSketch(s, 0, 0, 80, 50)
	s.AddCircle(s.AddPoint(geom.V2(20, 25)).ID, 9)
	c1, c2 := s.AddPoint(geom.V2(50, 25)).ID, s.AddPoint(geom.V2(64, 25)).ID
	s1, e1 := s.AddPoint(geom.V2(50, 31)).ID, s.AddPoint(geom.V2(50, 19)).ID
	s2, e2 := s.AddPoint(geom.V2(64, 19)).ID, s.AddPoint(geom.V2(64, 31)).ID
	s.AddArc(c1, s1, e1)
	s.AddArc(c2, s2, e2)
	s.AddLine(e2, s1)
	s.AddLine(e1, s2)
	rs := s.Regions()
	plate := rs[0]
	if len(plate.Holes) != 2 {
		t.Fatalf("%d holes", len(plate.Holes))
	}
	b, err := Extrude("e", plate, plane, -4, 8)
	if err != nil {
		t.Fatal(err)
	}
	want := plate.Area * 12
	if v := checkSolid(t, b); math.Abs(v-want)/want > 1e-3 {
		t.Fatalf("volume %.3f, want %.3f", v, want)
	}
	if g := brep.Genus(b); g != 2 {
		t.Fatalf("genus %d, want 2 (two through holes)", g)
	}
	arcs := 0
	for _, f := range b.Faces() {
		if strings.HasPrefix(f.Name, "side:") && f.Surf.Kind() == "cylinder" {
			arcs++
		}
	}
	if arcs != 4 { // the hole's two halves and the slot's two ends
		t.Fatalf("%d cylindrical sides, want 4", arcs)
	}
	// the disc inside the hole extrudes on its own too
	disc, err := Extrude("d", rs[len(rs)-1], plane, 0, 5)
	if err != nil {
		t.Fatal(err)
	}
	checkSolid(t, disc)
}

// A notch: an arc bulging into the profile (clockwise in the outer loop)
// must face inward.
func TestExtrudeConcaveArc(t *testing.T) {
	s := sketch.New("s", geom.World)
	p := func(x, y float64) int { return s.AddPoint(geom.V2(x, y)).ID }
	a, b, c, d := p(0, 0), p(40, 0), p(40, 30), p(0, 30)
	e, f := p(25, 30), p(15, 30)
	cc := p(20, 30)
	s.AddLine(a, b)
	s.AddLine(b, c)
	s.AddLine(c, e)
	s.AddArc(cc, f, e) // CCW from f to e: the lower half, a bite into the top
	s.AddLine(f, d)
	s.AddLine(d, a)
	rs := s.Regions()
	if len(rs) != 1 {
		t.Fatalf("%d regions", len(rs))
	}
	want := 1200 - math.Pi*25/2
	if math.Abs(rs[0].Area-want) > 1e-6 {
		t.Fatalf("area %.4f, want %.4f", rs[0].Area, want)
	}
	body, err := Extrude("e", rs[0], geom.World, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if v := checkSolid(t, body); math.Abs(v-want*10)/(want*10) > 1e-3 {
		t.Fatalf("volume %.3f, want %.3f", v, want*10)
	}
}
