package doc

import (
	"math"
	"strings"
	"testing"

	"forma/brep"
	"forma/geom"
	"forma/sketch"
)

// A dog's name parameter inlaid in a bowl's side: the bowl pocketed and a
// body per letter; renaming the dog rebuilds pockets and letters.
func TestEmbossNameFollows(t *testing.T) {
	sketch.FontDirs = append(sketch.FontDirs, "../assets/fonts")
	d := New()
	d.AddParam("name", `"Luna"`)
	bowl := d.Insert(NewPrim(KCylinder))
	bowl.Inputs["Radius"], bowl.Inputs["Height"] = "40", "30"
	sk := d.Insert(NewSketch(PlaneRef{Std: "xz"}))
	a := sk.Sketch.AddPoint(geom.V2(0, 12))
	e := sk.Sketch.AddText(a.ID, "=name", "Inter-Medium", 8)
	e.Align = sketch.AlignCenter
	d.Regenerate()
	var side string
	for _, fc := range bowl.Bodies[0].Faces() {
		if strings.HasSuffix(fc.Name, "side") {
			side = fc.Name
		}
	}
	em := d.Insert(NewEmboss([]ProfileRef{{Sketch: sk.ID, Outer: []int{e.ID}, Text: e.ID}}, BodyKey(bowl.ID, 0),
		FaceRef{Feature: bowl.ID, Face: side}))
	em.Inputs["Depth"] = "1.5"
	d.Regenerate()
	if em.Err != "" {
		t.Fatal(em.Err)
	}
	count := func() int {
		n := 0
		for _, b := range d.Bodies {
			if b.Owner == em.ID {
				n++
			}
		}
		return n
	}
	if n := count(); n != 4 {
		t.Fatalf("%d inlays for Luna, want 4", n)
	}
	// the letters land on the front (the sketch's normal, −Y): readable there
	for _, b := range d.Bodies {
		if b.Owner == em.ID && b.Body.Bounds().Max.Y > 0 {
			t.Fatalf("%s lies behind the bowl", b.Name)
		}
	}
	d.Params[0].Expr = `"Lilly"`
	d.Regenerate()
	if em.Err != "" {
		t.Fatal(em.Err)
	}
	// L, i (stem and dot), l, l, y
	if n := count(); n != 6 {
		t.Fatalf("%d inlays for Lilly, want 6", n)
	}
	// flipped: on the back
	em.Inputs["Flip"] = "1"
	d.Regenerate()
	if em.Err != "" {
		t.Fatal(em.Err)
	}
	for _, b := range d.Bodies {
		if b.Owner == em.ID && b.Body.Bounds().Min.Y < 0 {
			t.Fatalf("flipped, %s is still in front", b.Name)
		}
	}
}

// The name in Cooper Black (heavy, its letters touching once grown) inlaid,
// engraved and raised. Skipped where the font isn't installed.
func TestEmbossCooperBlack(t *testing.T) {
	have := false
	for _, n := range sketch.FontNames() {
		have = have || strings.EqualFold(n, "COOPBL")
	}
	if !have {
		t.Skip("Cooper Black isn't installed")
	}
	d := New()
	d.AddParam("name", `"Luna"`)
	bowl := d.Insert(NewPrim(KCylinder))
	bowl.Inputs["Radius"], bowl.Inputs["Height"] = "40", "30"
	sk := d.Insert(NewSketch(PlaneRef{Std: "xz"}))
	a := sk.Sketch.AddPoint(geom.V2(-16, 11))
	e := sk.Sketch.AddText(a.ID, "=name", "COOPBL", 10)
	d.Regenerate()
	var side string
	for _, fc := range bowl.Bodies[0].Faces() {
		if strings.HasSuffix(fc.Name, "side") {
			side = fc.Name
		}
	}
	em := d.Insert(NewEmboss([]ProfileRef{{Sketch: sk.ID, Outer: []int{e.ID}, Text: e.ID}}, BodyKey(bowl.ID, 0), FaceRef{Feature: bowl.ID, Face: side}))
	for _, ty := range EmbossTypes {
		em.Inputs["Type"] = ty
		d.Regenerate()
		if em.Err != "" {
			t.Fatalf("%s: %s", ty, em.Err)
		}
	}
}

// Any profile takes a clearance now: a plate with a round hole inlaid in
// the side of a cylinder, the pocket its outline grown and its hole shrunk.
func TestEmbossInlayShape(t *testing.T) {
	d := New()
	bowl := d.Insert(NewPrim(KCylinder))
	bowl.Inputs["Radius"], bowl.Inputs["Height"] = "40", "30"
	sk := d.Insert(NewSketch(PlaneRef{Std: "xz"}))
	s := sk.Sketch
	var ids []int
	var pts []int
	for _, p := range []geom.Vec2{{X: -10, Y: 11}, {X: 10, Y: 11}, {X: 10, Y: 19}, {X: -10, Y: 19}} {
		pts = append(pts, s.AddPoint(p).ID)
	}
	for i := range pts {
		ids = append(ids, s.AddLine(pts[i], pts[(i+1)%4]).ID)
	}
	hole := s.AddCircle(s.AddPoint(geom.V2(0, 15)).ID, 2)
	d.Regenerate()
	var side string
	for _, fc := range bowl.Bodies[0].Faces() {
		if strings.HasSuffix(fc.Name, "side") {
			side = fc.Name
		}
	}
	orig := bowl.Bodies[0]
	em := d.Insert(NewEmboss([]ProfileRef{{Sketch: sk.ID, Outer: ids, Holes: [][]int{{hole.ID}}}}, BodyKey(bowl.ID, 0),
		FaceRef{Feature: bowl.ID, Face: side}))
	em.Inputs["Depth"], em.Inputs["Clearance"] = "2", "0.3"
	d.Regenerate()
	if em.Err != "" {
		t.Fatal(em.Err)
	}
	var piece, body *brep.Body
	for _, b := range d.Bodies {
		if b.Owner == em.ID {
			piece = b.Body
		} else {
			body = b.Body
		}
	}
	if piece == nil || body == nil {
		t.Fatal("no inlay")
	}
	cut, err := brep.Boolean(orig, body, brep.Subtract)
	if err != nil {
		t.Fatal(err)
	}
	pocket := brep.Mass(cut, 1e-5).Volume
	grown := 20.6*8.6 - math.Pi*1.7*1.7
	area := 160 - math.Pi*4
	vin := brep.Mass(piece, 1e-5).Volume
	// projected onto a cylinder of radius 40 the areas are a little bigger
	if pocket < grown*2 || pocket > grown*2*1.06 {
		t.Fatalf("pocket %.1f mm³, want about %.1f", pocket, grown*2)
	}
	if vin < area*1.7 || vin > area*1.7*1.06 {
		t.Fatalf("inlay %.1f mm³, want about %.1f", vin, area*1.7)
	}
	if x, err := brep.Boolean(body, piece, brep.Intersect); err == nil && x != nil && brep.Mass(x, 1e-4).Volume > 1e-3 {
		t.Fatalf("the inlay overlaps the body by %.4f mm³", brep.Mass(x, 1e-4).Volume)
	}
}
