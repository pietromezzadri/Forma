package doc

import (
	"math"
	"testing"

	"forma/brep"
)

// Every part model builds one valid solid of about its module's size, and
// a shaft's rim is where the model says.
func TestPartModels(t *testing.T) {
	want := map[string][3]float64{ // overall size (mm), within 15 %
		"uno": {74.8, 53.3, 12.1}, "sg90": {32.2, 12.2, 27.5}, "tt-motor": {22.5, 73, 28.4},
		"battery-4aa": {62.5, 57.5, 16}, "tb6612": {20.3, 20.3, 8.6},
		"button": {6, 6, 8.4}, "pot": {16, 18, 29}, "led": {5.8, 5.8, 35.6},
	}
	for _, id := range PartModelIDs() {
		d, err := PartDesign(id)
		if err != nil {
			t.Errorf("%s: %v", id, err)
			continue
		}
		mustOK(t, d.Features...)
		if len(d.Bodies) != 1 {
			t.Errorf("%s: %d bodies", id, len(d.Bodies))
			continue
		}
		b := d.Bodies[0].Body
		if err := brep.Check(b); err != nil {
			t.Errorf("%s: %v", id, err)
		}
		bb := b.Bounds()
		size := bb.Max.Sub(bb.Min)
		if w, ok := want[id]; ok {
			for k, got := range []float64{size.X, size.Y, size.Z} {
				if math.Abs(got-w[k]) > 0.15*w[k] {
					t.Errorf("%s: size %v, want about %v", id, size, w)
					break
				}
			}
		}
		if s := PartModels[id].Shaft; s != nil {
			circleEdge(t, d, d.Bodies[0].Key, s[0], s[1], s[2], s[3])
		}
		if _, err := NewPartComponent(id); err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
}

// Only the LED's lens lights up: its top and side, not the flange or legs.
func TestLEDLitFaces(t *testing.T) {
	d := New()
	f, err := NewPartComponent("led")
	if err != nil {
		t.Fatal(err)
	}
	d.Insert(f)
	d.Regenerate()
	m := PartModels["led"]
	var lit []string
	for _, b := range d.Bodies {
		for _, fc := range b.Body.Faces() {
			if m.LitFace(fc.Name) {
				lit = append(lit, fc.Name)
			}
		}
	}
	if len(lit) != 2 {
		t.Fatalf("lit faces %v, want the lens's top and side", lit)
	}
	if PartModels["button"].LitFace(lit[0]) {
		t.Fatal("a button has nothing lit")
	}
}
