package ops

import (
	"math"
	"testing"

	"forma/brep"
	"forma/geom"
	"forma/sketch"
)

// a bowl's half section: up the axis, out along the rim, and back down a
// cubic Bézier to the foot on the axis
func bowlSketch() (*sketch.Sketch, []geom.Vec2) {
	s := sketch.New("s", geom.World)
	o := s.AddPoint(geom.V2(0, 0)).ID
	top := s.AddPoint(geom.V2(0, 30)).ID
	rim := s.AddPoint(geom.V2(25, 30)).ID
	c1 := s.AddPoint(geom.V2(32, 8)).ID
	c2 := s.AddPoint(geom.V2(12, 0)).ID
	s.AddLine(o, top)
	s.AddLine(top, rim)
	s.AddSpline(rim, c1, c2, o)
	ctrl := []geom.Vec2{{X: 25, Y: 30}, {X: 32, Y: 8}, {X: 12, Y: 0}, {X: 0, Y: 0}}
	return s, ctrl
}

// polygon of the bowl section, finely sampled (for reference integrals)
func bowlPoly(ctrl []geom.Vec2) []geom.Vec2 {
	pts := make([]geom.Vec3, len(ctrl))
	for i, c := range ctrl {
		pts[i] = geom.Vec3{X: c.X, Y: c.Y}
	}
	b := geom.Bezier(pts...)
	poly := []geom.Vec2{{X: 0, Y: 30}}
	for i := 0; i <= 20000; i++ {
		p := b.Eval(float64(i) / 20000)
		poly = append(poly, geom.Vec2{X: p.X, Y: p.Y})
	}
	return poly
}

func TestExtrudeSpline(t *testing.T) {
	s, ctrl := bowlSketch()
	rs := s.Regions()
	if len(rs) != 1 {
		t.Fatalf("%d regions", len(rs))
	}
	b, err := Extrude("e", rs[0], geom.World, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	area := 0.0
	poly := bowlPoly(ctrl)
	for i := range poly {
		area += poly[i].Cross(poly[(i+1)%len(poly)]) / 2
	}
	v := checkSolid(t, b)
	if want := math.Abs(area) * 10; math.Abs(v-want)/want > 1e-3 {
		t.Fatalf("volume %.3f, want %.3f", v, want)
	}
	kinds := map[string]int{}
	for _, f := range b.Faces() {
		kinds[f.Surf.Kind()]++
	}
	if kinds["extruded"] != 1 {
		t.Fatalf("faces %v", kinds)
	}
	// cut a hole through the curved side
	f := geom.World
	f.O = geom.V3(20, 10, 5)
	f.Z, f.X, f.Y = geom.XAxis, geom.YAxis, geom.ZAxis
	f.O = geom.V3(0, 12, 5)
	cy, err := Cylinder(f, 2.5, 40)
	if err != nil {
		t.Fatal(err)
	}
	cut, err := brep.Boolean(b, cy, brep.Subtract)
	if err != nil {
		t.Fatal(err)
	}
	vc := checkSolid(t, cut)
	inter, err := brep.Boolean(b, cy, brep.Intersect)
	if err != nil {
		t.Fatal(err)
	}
	vi := checkSolid(t, inter)
	if math.Abs(vc+vi-v) > 2e-3*v {
		t.Fatalf("cut %.3f + intersection %.3f ≠ %.3f", vc, vi, v)
	}
}

// Revolved about Y: a bowl whose side is a surface of revolution; its
// volume by Pappus, 2π ∫∫ x dA.
func TestRevolveSpline(t *testing.T) {
	s, ctrl := bowlSketch()
	poly := bowlPoly(ctrl)
	mx := 0.0 // ∫∫ x dA over the section
	for i := range poly {
		p, q := poly[i], poly[(i+1)%len(poly)]
		mx += (p.X + q.X) * p.Cross(q) / 6
	}
	want := 2 * math.Pi * math.Abs(mx)
	b := revolveCase(t, s, 2*math.Pi, want, geom.World)
	kinds := map[string]int{}
	for _, f := range b.Faces() {
		kinds[f.Surf.Kind()]++
	}
	if kinds["revolved"] != 1 {
		t.Fatalf("faces %v", kinds)
	}
	revolveCase(t, s, math.Pi/2, want/4, geom.World)
	// a plane square to the axis cuts it in a circle
	cut, err := brep.Boolean(b, boxBody(t, -50, 15, -50, 50, 60, 50), brep.Subtract)
	if err != nil {
		t.Fatal(err)
	}
	checkSolid(t, cut)
	circ := 0
	for _, e := range cut.Edges() {
		if c, ok := e.Curve.(geom.Circle); ok && math.Abs(c.F.O.Y-15) < 1e-9 {
			circ++
		}
	}
	if circ == 0 {
		t.Fatal("no exact circle where the plane cut the revolved side")
	}
}

func boxBody(t *testing.T, x0, y0, z0, x1, y1, z1 float64) *brep.Body {
	t.Helper()
	b, err := Box(geom.V3(x0, y0, z0), geom.V3(x1, y1, z1))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
