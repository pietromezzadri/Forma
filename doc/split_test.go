package doc

import (
	"math"
	"testing"
)

// Split: a box cut by the XY plane moved up, keeping both halves (the
// body keeps the top, the bottom is a new body), then one side only; a
// flat face as the plane; and the step survives a save.
func TestSplit(t *testing.T) {
	for _, c := range []struct {
		keep string
		n    int
		want []float64
	}{{"Both", 2, []float64{40 * 30 * 15, 40 * 30 * 5}}, {"Front", 1, []float64{40 * 30 * 15}}, {"Back", 1, []float64{40 * 30 * 5}}} {
		d := New()
		bx := d.Insert(NewPrim(KBox))
		bx.Inputs["Width"], bx.Inputs["Depth"], bx.Inputs["Height"] = "40", "30", "20"
		d.Regenerate()
		sp := d.Insert(NewSplit(BodyKey(bx.ID, 0), PlaneRef{Std: "xy"}))
		sp.Inputs["Offset"], sp.Inputs["Keep"] = "5", c.keep
		d.Regenerate()
		if sp.Err != "" {
			t.Fatalf("%s: %s", c.keep, sp.Err)
		}
		if len(d.Bodies) != c.n {
			t.Fatalf("%s: %d bodies, want %d", c.keep, len(d.Bodies), c.n)
		}
		for k, w := range c.want {
			if v := volume(t, d.Bodies[k].Body); math.Abs(v-w) > 1e-6*w {
				t.Fatalf("%s: body %d volume %.1f, want %.0f", c.keep, k, v, w)
			}
		}
		if d.Bodies[0].Key != BodyKey(bx.ID, 0) {
			t.Fatalf("%s: the split body lost its key", c.keep)
		}
		d2, err := Unmarshal(d.Marshal())
		if err != nil {
			t.Fatal(err)
		}
		if len(d2.Bodies) != c.n {
			t.Fatalf("%s reloaded: %d bodies", c.keep, len(d2.Bodies))
		}
	}
	// by a face of another body: a box's side face cuts a long box in two
	d := New()
	long := d.Insert(NewPrim(KBox))
	long.Inputs["Width"], long.Inputs["Depth"], long.Inputs["Height"] = "100", "10", "10"
	wall := d.Insert(NewPrim(KBox))
	wall.Inputs["Width"], wall.Inputs["Depth"], wall.Inputs["Height"], wall.Inputs["X"], wall.Inputs["Y"] = "5", "40", "40", "30", "50"
	d.Regenerate()
	var side *FaceRef
	for _, fc := range d.Bodies[1].Body.Faces() {
		if pl, ok := PlaneOf(fc); ok && math.Abs(pl.Z.X) > 0.99 && math.Abs(pl.O.X-30) < 1e-6 {
			side = &FaceRef{Feature: wall.ID, Body: BodyKey(wall.ID, 0), Face: fc.Name}
		}
	}
	if side == nil {
		t.Fatal("no face at x 30")
	}
	sp := d.Insert(NewSplit(BodyKey(long.ID, 0), PlaneRef{Feature: side.Feature, Body: side.Body, Face: side.Face}))
	d.Regenerate()
	if sp.Err != "" {
		t.Fatal(sp.Err)
	}
	var vs []float64
	for _, r := range d.Bodies {
		vs = append(vs, volume(t, r.Body))
	}
	t.Logf("by a face: %v", vs)
	if len(vs) != 3 || math.Abs(vs[0]+vs[2]-100*10*10) > 1e-3 || math.Min(vs[0], vs[2]) != 30*10*10 {
		t.Fatalf("by a face: volumes %v, want 3000 and 7000 and the wall", vs)
	}
	// a plane that misses says so
	sp.Inputs["Offset"] = "500"
	d.Regenerate()
	if sp.Err == "" {
		t.Fatal("a plane past the body should fail")
	}
	// a colour survives a save
	long.Color = "#c0392b"
	d2, err := Unmarshal(d.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := ParseColor(d2.Feature(long.ID).Color); !ok || FormatColor(c) != "#c0392b" {
		t.Fatalf("colour after a save: %q", d2.Feature(long.ID).Color)
	}
}
