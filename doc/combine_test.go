package doc

import (
	"math"
	"testing"
)

// Combine: two overlapping boxes joined, cut and intersected; the tool is
// used up unless kept, and the step survives a save.
func TestCombine(t *testing.T) {
	cases := []struct {
		op   string
		want float64
	}{{"Join", 8000 + 8000 - 4000}, {"Cut", 8000 - 4000}, {"Intersect", 4000}}
	for _, c := range cases {
		for _, keep := range []string{"0", "1"} {
			d := New()
			a := d.Insert(NewPrim(KBox))
			a.Inputs["Width"], a.Inputs["Depth"], a.Inputs["Height"] = "20", "20", "20"
			b := d.Insert(NewPrim(KBox))
			b.Inputs["Width"], b.Inputs["Depth"], b.Inputs["Height"], b.Inputs["X"] = "20", "20", "20", "10"
			d.Regenerate()
			cb := d.Insert(NewCombine(BodyKey(a.ID, 0), []int{BodyKey(b.ID, 0)}))
			cb.Inputs["Operation"], cb.Inputs["Keep tools"] = c.op, keep
			d.Regenerate()
			if cb.Err != "" {
				t.Fatalf("%s: %s", c.op, cb.Err)
			}
			wantN := 1
			if keep == "1" {
				wantN = 2
			}
			if len(d.Bodies) != wantN {
				t.Fatalf("%s keep=%s: %d bodies, want %d", c.op, keep, len(d.Bodies), wantN)
			}
			if v := volume(t, d.Bodies[0].Body); math.Abs(v-c.want) > 1e-6*c.want {
				t.Fatalf("%s: volume %.3f, want %.0f", c.op, v, c.want)
			}
			d2, err := Unmarshal(d.Marshal())
			if err != nil {
				t.Fatal(err)
			}
			if len(d2.Bodies) != wantN {
				t.Fatalf("%s reloaded: %d bodies", c.op, len(d2.Bodies))
			}
		}
	}
	// apart: nothing in common says so
	d := New()
	a := d.Insert(NewPrim(KBox))
	b := d.Insert(NewPrim(KBox))
	b.Inputs["X"] = "500"
	d.Regenerate()
	cb := d.Insert(NewCombine(BodyKey(a.ID, 0), []int{BodyKey(b.ID, 0)}))
	cb.Inputs["Operation"] = "Intersect"
	d.Regenerate()
	if cb.Err == "" {
		t.Fatal("intersecting bodies apart should fail")
	}
}
