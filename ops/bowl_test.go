package ops

import (
	"math"
	"testing"

	"forma/brep"
	"forma/geom"
	"forma/sketch"
)

// The dog bowl: a ring (Ø120 / Ø135, 50 high) with a foot revolved onto its
// outside whose profile is a Bézier leaving the floor flat (tangent to the
// ring's bottom face along the circle where they meet).
func TestBowlFootJoins(t *testing.T) {
	ring := sketch.New("ring", geom.World)
	c := ring.AddPoint(geom.V2(0, 0)).ID
	ring.AddCircle(c, 60)
	ring.AddCircle(c, 67.5)
	var reg sketch.Region
	for _, r := range ring.Regions() {
		if len(r.Holes) == 1 {
			reg = r
		}
	}
	body, err := Extrude("ring", reg, geom.World, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	xz := geom.Frame{X: geom.XAxis, Y: geom.ZAxis, Z: geom.YAxis.Neg()}
	s := sketch.New("foot", xz)
	a := s.AddPoint(geom.V2(67.5, 0)).ID
	h1 := s.AddPoint(geom.V2(75, 0)).ID
	h2 := s.AddPoint(geom.V2(75, 15)).ID
	e := s.AddPoint(geom.V2(72.5, 27.5)).ID
	top := s.AddPoint(geom.V2(67.5, 50)).ID
	s.AddSpline(a, h1, h2, e)
	s.AddLine(e, top)
	s.AddLine(top, a)
	rs := s.Regions()
	if len(rs) != 1 {
		t.Fatalf("%d regions", len(rs))
	}
	foot, err := Revolve("foot", rs[0], xz, geom.V2(0, 0), geom.V2(0, 1), 2*math.Pi)
	if err != nil {
		t.Fatal(err)
	}
	vr, vf := checkSolid(t, body), checkSolid(t, foot)
	u, err := brep.Boolean(body, foot, brep.Union)
	if err != nil {
		t.Fatal(err)
	}
	if v := checkSolid(t, u); math.Abs(v-(vr+vf)) > 2e-3*(vr+vf) {
		t.Fatalf("volume %.2f, want %.2f", v, vr+vf)
	}
}
