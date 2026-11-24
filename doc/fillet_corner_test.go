package doc

import (
	"math"
	"strings"
	"testing"

	"forma/brep"
	"forma/geom"
)

// pick the edges of body 0 that pass keep, as fillet references
func edgeRefs(d *Document, keep func(e *brep.Edge) bool) []EdgeRef {
	var refs []EdgeRef
	for _, e := range d.Bodies[0].Body.Edges() {
		if !keep(e) {
			continue
		}
		if r, ok := RefForEdge(e, e.Curve.Eval((e.T0+e.T1)/2)); ok {
			refs = append(refs, r)
		}
	}
	return refs
}

func ballFaces(b *brep.Body) int {
	n := 0
	for _, f := range b.Faces() {
		if strings.Contains(f.Name, "ball:") && strings.HasSuffix(f.Name, "/ball") {
			n++
		}
	}
	return n
}

// M19: where three rounded edges meet at a box's corner, the corner is a
// ball, not a mitre. Every edge of a box rounded is the rounded box of the
// textbook; one corner's three edges round that corner alone; a chamfer
// keeps its mitres.
func TestFilletBallCorners(t *testing.T) {
	const r = 4.0
	box := func() *Document {
		d := New()
		bx := d.Insert(NewPrim(KBox))
		bx.Inputs["Width"], bx.Inputs["Depth"], bx.Inputs["Height"] = "40", "30", "20"
		d.Regenerate()
		return d
	}
	// every edge: inner box, slabs, a cylinder's worth along each way, a ball
	d := box()
	f := d.Insert(NewFillet(edgeRefs(d, func(*brep.Edge) bool { return true }), false))
	f.Inputs["Radius"] = "4"
	d.Regenerate()
	if f.Err != "" {
		t.Fatal(f.Err)
	}
	b := d.Bodies[0].Body
	want := 32.0*22*12 + 2*(32*22+32*12+22*12)*r + math.Pi*r*r*(32+22+12) + 4.0/3*math.Pi*r*r*r
	v := volume(t, b)
	t.Logf("every edge: %d faces (%d balls), volume %.2f, exact %.2f", len(b.Faces()), ballFaces(b), v, want)
	if len(b.Faces()) != 26 || ballFaces(b) != 8 || math.Abs(v-want) > 0.002*want {
		t.Fatalf("every edge: %d faces, %d balls, volume %.2f (want 26, 8, %.2f)", len(b.Faces()), ballFaces(b), v, want)
	}
	if err := brep.Check(b); err != nil {
		t.Fatal(err)
	}
	// the three edges at one corner
	d = box()
	f = d.Insert(NewFillet(edgeRefs(d, func(e *brep.Edge) bool {
		return e.Verts[0].P.Len() < 1e-9 || e.Verts[1].P.Len() < 1e-9
	}), false))
	f.Inputs["Radius"] = "4"
	d.Regenerate()
	if f.Err != "" {
		t.Fatal(f.Err)
	}
	b = d.Bodies[0].Body
	want = 24000 - (r*r*r - math.Pi/6*r*r*r) - (r*r-math.Pi/4*r*r)*((40-r)+(30-r)+(20-r))
	if v := volume(t, b); ballFaces(b) != 1 || math.Abs(v-want) > 0.002*want {
		t.Fatalf("one corner: %d balls, volume %.2f, want %.2f", ballFaces(b), v, want)
	}
	// a chamfer's corners stay mitred
	d = box()
	f = d.Insert(NewFillet(edgeRefs(d, func(*brep.Edge) bool { return true }), true))
	f.Inputs["Distance"] = "2"
	d.Regenerate()
	if f.Err != "" || ballFaces(d.Bodies[0].Body) != 0 {
		t.Fatalf("chamfer: %q, %d balls", f.Err, ballFaces(d.Bodies[0].Body))
	}
	// a radius too big for the box's 20 mm edges keeps those corners mitred
	// or says so, but doesn't break
	d = box()
	f = d.Insert(NewFillet(edgeRefs(d, func(*brep.Edge) bool { return true }), false))
	f.Inputs["Radius"] = "9.5"
	d.Regenerate()
	t.Logf("radius 9.5: err %q", f.Err)
	if f.Err == "" {
		if err := brep.Check(d.Bodies[0].Body); err != nil {
			t.Fatal(err)
		}
	}
}

// Corners whose faces aren't square: a triangular prism (60° between its
// sides) with every edge rounded has six ball corners, is valid, and
// keeps less than the mitres did.
func TestFilletBallCornersSlanted(t *testing.T) {
	build := func(radius string) (*Document, *Feature) {
		d := New()
		sk := d.Insert(NewSketch(PlaneRef{Std: "xy"}))
		s := sk.Sketch
		p := []int{s.AddPoint(geomV2(0, 0)).ID, s.AddPoint(geomV2(40, 0)).ID, s.AddPoint(geomV2(20, 34.641)).ID}
		var ids []int
		for i := range p {
			ids = append(ids, s.AddLine(p[i], p[(i+1)%3]).ID)
		}
		e := d.Insert(NewExtrude([]ProfileRef{{Sketch: sk.ID, Outer: ids}}))
		e.Inputs["Distance"] = "15"
		d.Regenerate()
		if e.Err != "" {
			t.Fatal(e.Err)
		}
		f := d.Insert(NewFillet(edgeRefs(d, func(*brep.Edge) bool { return true }), false))
		f.Inputs["Radius"] = radius
		d.Regenerate()
		return d, f
	}
	d, f := build("3")
	if f.Err != "" {
		t.Fatal(f.Err)
	}
	b := d.Bodies[0].Body
	if err := brep.Check(b); err != nil {
		t.Fatal(err)
	}
	v := volume(t, b)
	t.Logf("prism: %d faces, %d balls, volume %.2f", len(b.Faces()), ballFaces(b), v)
	if ballFaces(b) != 6 || len(b.Faces()) != 5+9+6 {
		t.Fatalf("prism: %d faces, %d balls (want 20, 6)", len(b.Faces()), ballFaces(b))
	}
	// exact: the prism shrunk by r (side 40 − 2r√3, height 15 − 2r) grown by a
	// ball of r: V + S·r + M·r² + 4/3πr³, M half the sum of edge × outside angle
	sd, h := 40-2*3*math.Sqrt(3), 15.0-2*3
	area := math.Sqrt(3) / 4 * sd * sd
	want := area*h + (2*area+3*sd*h)*3 + 0.5*(3*h*2*math.Pi/3+6*sd*math.Pi/2)*9 + 4.0/3*math.Pi*27
	if math.Abs(v-want) > 0.003*want {
		t.Fatalf("prism: volume %.2f, exact %.2f", v, want)
	}
}
func geomV2(x, y float64) geom.Vec2 { return geom.V2(x, y) }
