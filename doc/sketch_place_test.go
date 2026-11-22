package doc

import (
	"math"
	"strings"
	"testing"

	"forma/geom"
	"forma/sketch"
)

func coneSide(t *testing.T, d *Document, cone *Feature) string {
	t.Helper()
	for _, fc := range cone.Bodies[0].Faces() {
		if strings.HasSuffix(fc.Name, "side") {
			return fc.Name
		}
	}
	t.Fatal("no side face")
	return ""
}

// A sketch on a cone's side, touching it where it was picked: upright (X
// round the axis, Y up it, facing out), the word on it embossed in all
// three ways; the sketch moved by its placement inputs, and kept with the
// design.
func TestSketchOnConeFace(t *testing.T) {
	sketch.FontDirs = append(sketch.FontDirs, "../assets/fonts")
	d := New()
	cone := d.Insert(NewPrim(KCone))
	cone.Inputs["Base radius"], cone.Inputs["Top radius"], cone.Inputs["Height"] = "30", "15", "40"
	d.Regenerate()
	side := coneSide(t, d, cone)
	at := geom.V3(0, -22.5, 20) // the front of the side, half way up
	sk := d.Insert(NewSketch(PlaneRef{Feature: cone.ID, Face: side, At: &at}))
	a := sk.Sketch.AddPoint(geom.V2(0, 0))
	e := sk.Sketch.AddText(a.ID, "FORMA", sketch.DefaultFont, 6)
	e.Align = sketch.AlignCenter
	d.Regenerate()
	if sk.Err != "" {
		t.Fatal(sk.Err)
	}
	pl := sk.Sketch.Plane
	if pl.O.Dist(at) > 0.05 || pl.Z.Dot(geom.V3(0, -1, 0)) < 0.9 || pl.X.Dist(geom.XAxis) > 1e-9 || pl.Y.Z < 0.9 {
		t.Fatalf("plane on the cone: %+v", pl)
	}
	for _, op := range EmbossTypes {
		em := d.Insert(NewEmboss([]ProfileRef{{Sketch: sk.ID, Outer: []int{e.ID}, Text: e.ID}}, BodyKey(cone.ID, 0), FaceRef{Feature: cone.ID, Face: side}))
		em.Inputs["Type"] = op
		em.Inputs["Depth"] = "1"
		d.Regenerate()
		if em.Err != "" {
			t.Fatalf("%s: %s", op, em.Err)
		}
		d.Remove(em.ID)
		d.Regenerate()
	}
	// moved: 5 out along the normal, turned 90°, 3 along its (turned) X
	sk.Inputs["Offset"], sk.Inputs["Turn"], sk.Inputs["Move X"] = "5", "90", "3"
	d.Regenerate()
	if sk.Err != "" {
		t.Fatal(sk.Err)
	}
	q := sk.Sketch.Plane
	want := at.Add(pl.Z.Scale(5)).Add(pl.Y.Scale(3))
	if q.O.Dist(want) > 0.05 || q.X.Dist(pl.Y) > 1e-9 || math.Abs(q.Z.Dot(pl.Z)-1) > 1e-9 {
		t.Fatalf("moved plane %+v, want origin %v and X along the old Y", q, want)
	}
	d2, err := Unmarshal(d.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	if s2 := d2.Feature(sk.ID); s2 == nil || s2.Plane.At == nil || s2.Sketch.Plane.O.Dist(q.O) > 1e-9 {
		t.Fatal("the sketch's place wasn't kept")
	}
}

// Text running off a face's side, where it turns away from the sketch,
// says so plainly.
func TestEmbossOffTheSide(t *testing.T) {
	sketch.FontDirs = append(sketch.FontDirs, "../assets/fonts")
	d := New()
	cone := d.Insert(NewPrim(KCone))
	cone.Inputs["Base radius"], cone.Inputs["Top radius"], cone.Inputs["Height"] = "30", "15", "40"
	sk := d.Insert(NewSketch(PlaneRef{Std: "xz"}))
	a := sk.Sketch.AddPoint(geom.V2(-12, 16))
	e := sk.Sketch.AddText(a.ID, "FORMA", sketch.DefaultFont, 7)
	d.Regenerate()
	side := coneSide(t, d, cone)
	em := d.Insert(NewEmboss([]ProfileRef{{Sketch: sk.ID, Outer: []int{e.ID}, Text: e.ID}}, BodyKey(cone.ID, 0), FaceRef{Feature: cone.ID, Face: side}))
	d.Regenerate()
	if em.Err == "" || !strings.Contains(em.Err, "side of the face") {
		t.Fatalf("error %q, want one about the side of the face", em.Err)
	}
}

// Text wrapped round a cylinder: a line longer than half the way round,
// which projected straight on runs off the sides, wraps round to the back.
func TestEmbossWrapsRound(t *testing.T) {
	sketch.FontDirs = append(sketch.FontDirs, "../assets/fonts")
	d := New()
	cyl := d.Insert(NewPrim(KCylinder))
	cyl.Inputs["Radius"], cyl.Inputs["Height"] = "12", "30"
	d.Regenerate()
	var side string
	for _, fc := range cyl.Bodies[0].Faces() {
		if strings.HasSuffix(fc.Name, "side") {
			side = fc.Name
		}
	}
	at := geom.V3(0, -12, 15)
	sk := d.Insert(NewSketch(PlaneRef{Feature: cyl.ID, Face: side, At: &at}))
	a := sk.Sketch.AddPoint(geom.V2(0, -2))
	e := sk.Sketch.AddText(a.ID, "WRAPPED ROUND", sketch.DefaultFont, 5)
	e.Align = sketch.AlignCenter
	em := d.Insert(NewEmboss([]ProfileRef{{Sketch: sk.ID, Outer: []int{e.ID}, Text: e.ID}}, BodyKey(cyl.ID, 0), FaceRef{Feature: cyl.ID, Face: side}))
	em.Inputs["Type"] = "Inlay"
	em.Inputs["Depth"] = "0.8"
	em.Inputs["Clearance"] = "0.1"
	d.Regenerate()
	if em.Err == "" {
		t.Fatal("projected straight on, the ends should run off the sides")
	}
	em.Inputs["Wrap"] = "1"
	d.Regenerate()
	if em.Err != "" {
		t.Fatal(em.Err)
	}
	back, n := 0, 0
	for _, b := range d.Bodies {
		if b.Owner != em.ID {
			continue
		}
		n++
		c := b.Body.Bounds().Center()
		if c.Y > 0 {
			back++
		}
		// every letter sits on the surface: its middle about the radius out
		if r := math.Hypot(c.X, c.Y); r < 10.5 || r > 12.5 {
			t.Fatalf("%s's middle %.2f from the axis", b.Name, r)
		}
	}
	if n < 10 || back < 2 {
		t.Fatalf("%d letters, %d of them round the back", n, back)
	}
}
