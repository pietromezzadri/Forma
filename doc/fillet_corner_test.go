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

// Edge sets: a box's every edge rounded, the four upright edges with a
// radius of their own. Two sets at the same size still make balls; a
// different size makes the corners mitres (the rounds cross), valid, and
// the upright rounds take their own radius.
func TestFilletEdgeSets(t *testing.T) {
	d := New()
	bx := d.Insert(NewPrim(KBox))
	bx.Inputs["Width"], bx.Inputs["Depth"], bx.Inputs["Height"] = "40", "30", "20"
	d.Regenerate()
	f := d.Insert(NewFillet(edgeRefs(d, func(*brep.Edge) bool { return true }), false))
	f.Inputs["Radius"] = "4"
	var upright []int
	for i, r := range f.Edges {
		// an upright edge's middle is at half height
		if math.Abs(r.P.Z-10) < 1e-9 {
			upright = append(upright, i)
		}
	}
	if len(upright) != 4 {
		t.Fatalf("%d upright edges", len(upright))
	}
	set := SetEdgeSize(f, upright, nil, "4")
	if set != 1 || f.Inputs["Radius 2"] != "4" || len(EdgeSets(f)) != 1 {
		t.Fatalf("set %d, input %q, sets %v", set, f.Inputs["Radius 2"], EdgeSets(f))
	}
	d.Regenerate()
	if f.Err != "" {
		t.Fatal(f.Err)
	}
	if n := ballFaces(d.Bodies[0].Body); n != 8 {
		t.Fatalf("two sets at 4 mm: %d balls, want 8", n)
	}
	// the upright edges at 8 mm: mitres where they meet the 4 mm rounds
	f.Inputs["Radius 2"] = "8"
	d.Regenerate()
	if f.Err != "" {
		t.Fatal(f.Err)
	}
	b := d.Bodies[0].Body
	if err := brep.Check(b); err != nil {
		t.Fatal(err)
	}
	if n := ballFaces(b); n != 0 {
		t.Fatalf("mixed radii: %d balls, want 0", n)
	}
	// an upright round's radius: its cylinder face's
	var radii []float64
	for _, fc := range b.Faces() {
		if c, ok := fc.Surf.(geom.Cylinder); ok && math.Abs(math.Abs(c.F.Z.Z)-1) < 1e-9 {
			radii = append(radii, c.R)
		}
	}
	if len(radii) != 4 || math.Abs(radii[0]-8) > 1e-9 {
		t.Fatalf("upright rounds' radii %v, want four of 8", radii)
	}
	// exact: the box with its uprights rounded at 8 (a rounded rectangle
	// prism), less the 4 mm rims round top and bottom (Pappus round the
	// arcs: the cut-off region's centroid lies r(10−3π)/(3(4−π)) in)
	R, r := 8.0, 4.0
	prism := (1200 - (4-math.Pi)*R*R) * 20
	cbar := r * (10 - 3*math.Pi) / (3 * (4 - math.Pi))
	path := 2*(40-2*R) + 2*(30-2*R) + 2*math.Pi*(R-cbar)
	want := prism - 2*r*r*(1-math.Pi/4)*path
	v := volume(t, b)
	t.Logf("mixed: %d faces, volume %.1f, exact %.1f", len(b.Faces()), v, want)
	if len(b.Faces()) != 26 || math.Abs(v-want) > 0.003*want {
		t.Fatalf("mixed: %d faces (want 26), volume %.1f (exact %.1f)", len(b.Faces()), v, want)
	}
	// and back on the step's own radius: balls again, saved and read back
	ClearEdgeSet(f, set)
	d.Regenerate()
	if f.Err != "" || ballFaces(d.Bodies[0].Body) != 8 || len(EdgeSets(f)) != 0 {
		t.Fatalf("cleared: %q, %d balls, sets %v", f.Err, ballFaces(d.Bodies[0].Body), EdgeSets(f))
	}
	SetEdgeSize(f, upright[:2], nil, "6")
	d2, err := Unmarshal(d.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	g := d2.Feature(f.ID)
	if g.Err != "" || len(EdgeSets(g)) != 1 || g.Inputs["Radius 2"] != "6" { // a cleared set's number is free again
		t.Fatalf("reloaded: %q, sets %v, inputs %v", g.Err, EdgeSets(g), g.Inputs)
	}
}

// Mixed radii either way round: uprights larger or smaller than the rims,
// or nearly the same, every combination a valid solid of 26 faces.
func TestFilletMixedRadii(t *testing.T) {
	for _, c := range [][2]string{{"8", "2"}, {"4", "3"}, {"3", "4"}} {
		d := New()
		bx := d.Insert(NewPrim(KBox))
		bx.Inputs["Width"], bx.Inputs["Depth"], bx.Inputs["Height"] = "40", "30", "20"
		d.Regenerate()
		f := d.Insert(NewFillet(edgeRefs(d, func(*brep.Edge) bool { return true }), false))
		f.Inputs["Radius"] = c[0]
		var up []int
		for i, r := range f.Edges {
			if math.Abs(r.P.Z-10) < 1e-9 {
				up = append(up, i)
			}
		}
		SetEdgeSize(f, up, nil, c[1])
		d.Regenerate()
		if f.Err != "" {
			t.Fatalf("rims %s, uprights %s: %s", c[0], c[1], f.Err)
		}
		b := d.Bodies[0].Body
		if err := brep.Check(b); err != nil || len(b.Faces()) != 26 {
			t.Fatalf("rims %s, uprights %s: %d faces, %v", c[0], c[1], len(b.Faces()), err)
		}
	}
}
