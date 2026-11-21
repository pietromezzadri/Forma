package doc

import (
	"strings"
	"testing"

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
