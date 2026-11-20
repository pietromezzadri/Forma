package doc

import (
	"math"
	"testing"

	"forma/geom"
	"forma/sketch"
)

// A box's edge and a cylinder's rim projected into a sketch follow them
// when they change.
func TestProjectedEdgesFollow(t *testing.T) {
	d := New()
	box := d.Insert(NewPrim(KBox)) // 40 × 30 × 20 at the origin
	cyl := d.Insert(NewPrim(KCylinder))
	cyl.Inputs["X"], cyl.Inputs["Y"], cyl.Inputs["Radius"], cyl.Inputs["Height"] = "80", "0", "8", "10"
	sk := d.Insert(NewSketch(PlaneRef{Std: "xy"}))
	d.Regenerate()
	// the box's front top edge (y 0, z 20) and the cylinder's top rim
	if err := ProjectEdge(sk, d.FindEdgeNow(edgeRef(t, d, geom.V3(20, 0, 20))), geom.V3(20, 0, 20)); err != nil {
		t.Fatal(err)
	}
	if err := ProjectEdge(sk, d.FindEdgeNow(edgeRef(t, d, geom.V3(88, 0, 10))), geom.V3(88, 0, 10)); err != nil {
		t.Fatal(err)
	}
	d.Regenerate()
	if sk.Err != "" {
		t.Fatal(sk.Err)
	}
	line, circle := (*sketch.Entity)(nil), (*sketch.Entity)(nil)
	for _, e := range sk.Sketch.Ents {
		switch {
		case e.Kind == sketch.Line && e.Proj != "":
			line = e
		case e.Kind == sketch.Circle && e.Proj != "":
			circle = e
		}
	}
	if line == nil || circle == nil {
		t.Fatalf("projected: line %v, circle %v", line != nil, circle != nil)
	}
	s := sk.Sketch
	ends := func() (geom.Vec2, geom.Vec2) {
		a, b := s.P(line.Pts[0]), s.P(line.Pts[1])
		if a.X > b.X {
			a, b = b, a
		}
		return a, b
	}
	if a, b := ends(); a.Dist(geom.V2(0, 0)) > 1e-9 || b.Dist(geom.V2(40, 0)) > 1e-9 {
		t.Fatalf("line %v → %v", a, b)
	}
	if c := s.P(circle.Pts[0]); c.Dist(geom.V2(80, 0)) > 1e-9 || math.Abs(s.Radius(circle)-8) > 1e-9 {
		t.Fatalf("circle at %v r %g", c, s.Radius(circle))
	}
	// the box widens, the cylinder grows: the projections follow
	box.Inputs["Width"] = "60"
	cyl.Inputs["Radius"] = "12"
	d.Regenerate()
	if sk.Err != "" {
		t.Fatal(sk.Err)
	}
	if _, b := ends(); b.Dist(geom.V2(60, 0)) > 1e-9 {
		t.Fatalf("line end at %v, want (60, 0)", b)
	}
	if r := s.Radius(circle); math.Abs(r-12) > 1e-9 {
		t.Fatalf("circle r %g, want 12", r)
	}
	if s.Status.Residual > 1e-9 {
		t.Fatalf("sketch left unsolved: %g", s.Status.Residual)
	}
}
