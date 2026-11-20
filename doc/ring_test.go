package doc

import (
	"math"
	"os"
	"testing"
)

// A ring sketched as two circles round one centre, extruded, with a flange
// revolved onto its outside and joined. Changing the inner diameter must
// leave the centre (and so the outer circle) where it is, and the flange
// must still join: it used to move the shared centre by 2.5 mm, and the
// join failed.
func TestRingInnerDiameter(t *testing.T) {
	b, err := os.ReadFile("testdata/ring_flange.forma")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"120", "110", "125", "90"} {
		d, err := Unmarshal(b)
		if err != nil {
			t.Fatal(err)
		}
		d.Regenerate()
		s := d.Features[0].Sketch
		for _, c := range s.Cons {
			if c.ID == 3 {
				c.Expr = v
			}
		}
		d.Regenerate()
		for _, f := range d.Features {
			if f.Err != "" {
				t.Fatalf("inner %s: %s: %s", v, f.Name, f.Err)
			}
		}
		if c := s.P(1); c.Len() > 1e-6 {
			t.Fatalf("inner %s: the centre moved to %v", v, c)
		}
		if len(d.Bodies) != 1 {
			t.Fatalf("inner %s: %d bodies", v, len(d.Bodies))
		}
		bb := d.Bodies[0].Body.Bounds()
		if math.Abs(bb.Max.X-81.7302494707577) > 1e-6 || math.Abs(bb.Min.X+81.7302494707577) > 1e-6 {
			t.Fatalf("inner %s: bounds %v .. %v", v, bb.Min, bb.Max)
		}
	}
}
