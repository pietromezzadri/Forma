package doc

import (
	"math"
	"testing"

	"forma/brep"
	"forma/geom"
	"forma/sketch"
)

// A dog's name as a text parameter, sketched and extruded: changing the
// parameter rebuilds the letters and everything made from them.
func TestTextParameterCarriesThrough(t *testing.T) {
	sketch.FontDirs = append(sketch.FontDirs, "../assets/fonts")
	d := New()
	d.AddParam("name", `"Luna"`)
	sk := d.Insert(NewSketch(PlaneRef{Std: "xy"}))
	s := sk.Sketch
	a := s.AddPoint(geom.V2(0, 0))
	s.AddText(a.ID, "=name", "Inter-Medium", 12)
	d.Regenerate()
	if sk.Err != "" {
		t.Fatal(sk.Err)
	}
	var ref ProfileRef
	ink := 0
	area := 0.0
	for _, r := range sk.Regions {
		if r.Text != 0 && r.Ink {
			ref = RefFor(sk.ID, r)
			ink++
			area += r.Area
		}
	}
	if ink != 4 {
		t.Fatalf("%d letters of ink, want 4 (L u n a)", ink)
	}
	ex := d.Insert(NewExtrude([]ProfileRef{ref}))
	ex.Inputs["Distance"] = "3"
	d.Regenerate()
	if ex.Err != "" {
		t.Fatal(ex.Err)
	}
	vol := func() float64 {
		v := 0.0
		for _, b := range ex.Bodies {
			if err := brep.Check(b); err != nil {
				t.Fatal(err)
			}
			v += brep.Mass(b, 1e-4).Volume
		}
		return v
	}
	if v := vol(); math.Abs(v-3*area) > 1e-3*v {
		t.Fatalf("Luna: %.3f mm³, want %.3f", v, 3*area)
	}
	n := len(ex.Bodies)

	// renamed: the letters follow
	d.Params[0].Expr = `"Lilly"`
	d.Regenerate()
	if sk.Err != "" || ex.Err != "" {
		t.Fatalf("after renaming: %q %q", sk.Err, ex.Err)
	}
	area = 0
	for _, r := range sk.Regions {
		if r.Text != 0 && r.Ink {
			area += r.Area
		}
	}
	if len(ex.Bodies) == n {
		t.Fatalf("still %d bodies after the text changed", n)
	}
	if v := vol(); math.Abs(v-3*area) > 1e-3*v {
		t.Fatalf("Lilly: %.3f mm³, want %.3f", v, 3*area)
	}
}

// An o is a ring: its body is ink, its counter isn't.
func TestTextCounters(t *testing.T) {
	sketch.FontDirs = append(sketch.FontDirs, "../assets/fonts")
	s := sketch.New("s", geom.World)
	a := s.AddPoint(geom.V2(0, 0))
	s.AddText(a.ID, "o", "Inter-Medium", 10)
	var ink, counter int
	for _, r := range s.Regions() {
		if r.Text == 0 {
			continue
		}
		if r.Ink {
			ink++
			if len(r.Holes) != 1 {
				t.Fatalf("the o's ring has %d holes", len(r.Holes))
			}
		} else {
			counter++
		}
	}
	if ink != 1 || counter != 1 {
		t.Fatalf("o: %d ink, %d counter regions", ink, counter)
	}
}
