package ops

import (
	"math"
	"testing"

	"forma/brep"
	"forma/geom"
	"forma/sketch"
)

// Feet of every kind joined to the ring: a Bézier leaving the floor flat
// (tangent to the ring's bottom along the rim) or sloped, or a straight
// line; sitting on the ring's outside face or reaching into its wall. The
// overlapping ones cross the ring's split outer cylinder where the foot's
// seam crosses the split line (a cut ending on two edges at once).
func TestBowlFootCases(t *testing.T) {
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
	vr := checkSolid(t, body)
	xz := geom.Frame{X: geom.XAxis, Y: geom.ZAxis, Z: geom.YAxis.Neg()}
	try := func(name string, h1, h2 geom.Vec2, lineFoot bool, inner float64, height ...float64) {
		s := sketch.New("foot", xz)
		a := s.AddPoint(geom.V2(inner, 0)).ID
		e := s.AddPoint(geom.V2(72.5, 27.5)).ID
		h := 50.0
		if len(height) > 0 {
			h = height[0]
		}
		top := s.AddPoint(geom.V2(inner, h)).ID
		if lineFoot {
			s.AddLine(a, e)
		} else {
			s.AddSpline(a, s.AddPoint(h1).ID, s.AddPoint(h2).ID, e)
		}
		s.AddLine(e, top)
		s.AddLine(top, a)
		foot, err := Revolve("foot", s.Regions()[0], xz, geom.V2(0, 0), geom.V2(0, 1), 2*math.Pi)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		vf := checkSolid(t, foot)
		u, err := brep.Boolean(body, foot, brep.Union)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		in, err := brep.Boolean(body, foot, brep.Intersect)
		vi := 0.0
		if err == nil {
			vi = checkSolid(t, in)
		} else if err != brep.ErrEmpty {
			t.Fatalf("%s: intersection: %v", name, err)
		}
		if v := checkSolid(t, u); math.Abs(v-(vr+vf-vi)) > 2e-3*v {
			t.Fatalf("%s: union %.2f, want %.2f", name, v, vr+vf-vi)
		}
	}
	try("flat spline", geom.V2(75, 0), geom.V2(75, 15), false, 67.5)
	try("sloped spline", geom.V2(75, 3), geom.V2(75, 15), false, 67.5)
	try("line foot", geom.Vec2{}, geom.Vec2{}, true, 67.5)
	try("flat spline, overlap", geom.V2(75, 0), geom.V2(75, 15), false, 65)
	try("sloped spline, overlap", geom.V2(75, 3), geom.V2(75, 15), false, 65)
	try("line foot, overlap", geom.Vec2{}, geom.Vec2{}, true, 65)
	// flush with the ring's outside and a little taller: the ring's top rim
	// splits the foot's inner face half a millimetre below its top
	try("line foot, taller", geom.Vec2{}, geom.Vec2{}, true, 67.5, 50.5)
	try("flat spline, taller", geom.V2(75, 0), geom.V2(75, 15), false, 67.5, 50.5)
}
