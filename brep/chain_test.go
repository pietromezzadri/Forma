package brep_test

import (
	"math"
	"testing"

	"forma/brep"
	"forma/geom"
	"forma/ops"
	"forma/sketch"
)

// A plate whose outline is a rounded rectangle: its top rim is one chain
// of 4 lines and 4 arcs; a vertical tangent edge is not sharp, and a
// plain box's edge has no smooth neighbours.
func TestTangentChain(t *testing.T) {
	// 40 × 30 with corners of radius 6, as lines and arcs
	r, w, h := 6.0, 40.0, 30.0
	line := func(a, b geom.Vec2) sketch.Seg { return sketch.Seg{A: a, B: b} }
	arc := func(c geom.Vec2, a0, a1 float64) sketch.Seg {
		at := func(d float64) geom.Vec2 {
			return c.Add(geom.V2(math.Cos(d*math.Pi/180), math.Sin(d*math.Pi/180)).Scale(r))
		}
		return sketch.Seg{Arc: true, A: at(a0), B: at(a1), C: c, R: r, CCW: true}
	}
	segs := []sketch.Seg{
		line(geom.V2(r, 0), geom.V2(w-r, 0)),
		arc(geom.V2(w-r, r), -90, 0),
		line(geom.V2(w, r), geom.V2(w, h-r)),
		arc(geom.V2(w-r, h-r), 0, 90),
		line(geom.V2(w-r, h), geom.V2(r, h)),
		arc(geom.V2(r, h-r), 90, 180),
		line(geom.V2(0, h-r), geom.V2(0, r)),
		arc(geom.V2(r, r), 180, 270),
	}
	b, err := ops.Extrude("plate", sketch.Region{Outer: segs}, geom.World, 0, 8)
	if err != nil {
		t.Fatal(err)
	}
	var top *brep.Edge
	smooth := 0
	for _, e := range b.Edges() {
		p := e.Curve.Eval((e.T0 + e.T1) / 2)
		if !brep.Sharp(e) {
			smooth++
		}
		if p.Z > 7.9 && p.Y < 0.1 && top == nil {
			top = e
		}
	}
	if top == nil {
		t.Fatal("no top front edge")
	}
	if smooth != 8 {
		t.Errorf("%d smooth edges, want the 8 vertical ones where lines meet arcs", smooth)
	}
	chain := brep.TangentChain(b, top)
	if len(chain) != 8 {
		t.Fatalf("chain of %d edges, want the top rim's 8", len(chain))
	}
	for _, e := range chain {
		if e.Curve.Eval(e.T0).Z < 7.9 {
			t.Fatalf("edge %d isn't on the top", e.ID)
		}
	}
	box, _ := ops.Box(geom.V3(0, 0, 0), geom.V3(10, 10, 10))
	if c := brep.TangentChain(box, box.Edges()[0]); len(c) != 1 {
		t.Fatalf("a box edge chains to %d edges", len(c))
	}
}
