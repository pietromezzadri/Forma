package doc

import (
	"math"
	"testing"

	"forma/brep"
	"forma/geom"
)

// Every washer and nut of every standard and size is one valid solid of
// the volume its numbers give, standing from z = 0 to its thickness.
func TestFastenerDesigns(t *testing.T) {
	n := 0
	for _, kind := range []string{"washer", "nut"} {
		ids, _ := PartSpec{Kind: kind}.Stds()
		for _, std := range ids {
			for _, size := range (PartSpec{Kind: kind, Std: std}).Sizes() {
				p := PartSpec{Kind: kind, Std: std, Size: size}
				d, err := FastenerDesign(p)
				if err != nil {
					t.Fatalf("%s: %v", p.Label(), err)
				}
				b := d.Bodies[0].Body
				if err := brep.Check(b); err != nil {
					t.Fatalf("%s: %v", p.Label(), err)
				}
				v, _ := p.dims(size)
				want := 0.0
				if kind == "washer" {
					want = math.Pi / 4 * (v[1]*v[1] - v[0]*v[0]) * v[2]
				} else {
					want = (math.Sqrt(3)/2*v[0]*v[0] - math.Pi/4*v[2]*v[2]) * v[1]
				}
				if got := volume(t, b); math.Abs(got-want) > 0.01*want {
					t.Fatalf("%s: volume %.3f, want %.3f", p.Label(), got, want)
				}
				bx := b.Bounds()
				if math.Abs(bx.Min.Z) > 1e-9 || math.Abs(bx.Max.Z-p.Thickness()) > 1e-9 {
					t.Fatalf("%s: from z %.3f to %.3f, want 0 to %.3f", p.Label(), bx.Min.Z, bx.Max.Z, p.Thickness())
				}
				n++
			}
		}
	}
	t.Logf("%d washers and nuts built", n)
}

// A bolted joint through a plate: a washer under the screw's head, a
// washer and a nut on the far side, each put on its rim after the last (the nut by the washer's rim);
// they stack, touch nothing, follow the hole, count in the parts list, and
// nothing goes on over a screw.
func TestBoltedStack(t *testing.T) {
	d := New()
	bx := d.Insert(NewPrim(KBox))
	bx.Inputs["Width"], bx.Inputs["Depth"], bx.Inputs["Height"] = "40", "30", "10"
	sk := d.Insert(NewSketch(PlaneRef{Std: "xy"}))
	sk.Inputs = map[string]string{"Offset": "10"}
	pt := sk.Sketch.AddPoint(geom.V2(10, 15))
	h := d.Insert(NewHole(sk.ID, []int{pt.ID}))
	h.Inputs["Extent"], h.Inputs["Screw"], h.Inputs["Fit"] = "Through all", "M5", "Normal"
	d.Regenerate()
	key := d.Bodies[0].Key
	rim := func(z float64) *brep.Edge {
		for _, r := range d.Bodies {
			if r.Key != key {
				continue
			}
			for _, e := range r.Body.Edges() {
				if c, ok := e.Curve.(geom.Circle); ok && math.Abs(c.F.O.Z-z) < 1e-6 && math.Abs(c.R-2.75) < 1e-6 {
					return e
				}
			}
		}
		t.Fatalf("no rim at z %g", z)
		return nil
	}
	put := func(f *Feature, z float64) *Feature {
		t.Helper()
		d.Insert(f)
		j, err := d.FastenerInHole(f, key, rim(z))
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		d.Insert(j)
		d.Regenerate()
		if f.Err != "" || j.Err != "" {
			t.Fatalf("%s: %q %q", f.Name, f.Err, j.Err)
		}
		return j
	}
	washer := func() *Feature {
		f, err := NewFastener(PartSpec{Kind: "washer", Std: "ISO 7089", Size: "M5"})
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	w1 := washer()
	put(w1, 10)
	s, _ := NewScrew(ScrewSpec{Std: "ISO 4762", Size: "M5", Length: 20})
	sj := put(s, 10)
	w2 := washer()
	put(w2, 0)
	nut, err := NewFastener(PartSpec{Kind: "nut", Std: "ISO 4032", Size: "M5"})
	if err != nil {
		t.Fatal(err)
	}
	// the nut by the washer's rim: on the washer
	d.Insert(nut)
	var wr *BodyRef
	for _, r := range d.Bodies {
		if r.Comp == w2.ID {
			wr = r
		}
	}
	var wrim *brep.Edge
	for _, e := range wr.Body.Edges() {
		if _, ok := e.Curve.(geom.Circle); ok {
			wrim = e
		}
	}
	nj, err := d.FastenerInHole(nut, wr.Key, wrim)
	if err != nil {
		t.Fatal(err)
	}
	d.Insert(nj)
	d.Regenerate()
	if nut.Err != "" || nj.Err != "" {
		t.Fatalf("%q %q", nut.Err, nj.Err)
	}
	if sj.Name != "ISO 4762 M5×20 on ISO 7089 M5" || nj.Name != "ISO 4032 M5 on ISO 7089 M5" {
		t.Fatalf("joints %q, %q", sj.Name, nj.Name)
	}
	at := func(f *Feature) geom.Box {
		for _, r := range d.Bodies {
			if r.Comp == f.ID {
				return r.Body.Bounds()
			}
		}
		t.Fatalf("%s has no body", f.Name)
		return geom.Box{}
	}
	check := func(x, y float64) {
		t.Helper()
		for _, c := range []struct {
			f      *Feature
			lo, hi float64
		}{{w1, 10, 11}, {s, 11 - 20, 11 + 5}, {w2, -1, 0}, {nut, -1 - 4.7, -1}} {
			b := at(c.f)
			if math.Abs(b.Min.Z-c.lo) > 1e-6 || math.Abs(b.Max.Z-c.hi) > 1e-6 {
				t.Fatalf("%s from z %.4f to %.4f, want %g to %g", c.f.Name, b.Min.Z, b.Max.Z, c.lo, c.hi)
			}
			if m := b.Center(); math.Hypot(m.X-x, m.Y-y) > 1e-6 {
				t.Fatalf("%s at (%.4f, %.4f), want (%g, %g)", c.f.Name, m.X, m.Y, x, y)
			}
		}
	}
	check(10, 15)
	// nothing overlaps (the nut's bore is the screw's diameter: they touch)
	if cl := d.Interference(func(*BodyRef) bool { return true }); len(cl) != 0 {
		for _, c := range cl {
			t.Logf("%s / %s: %.4f mm³ %v", c.A.Name, c.B.Name, c.Volume, c.Err)
		}
		t.Fatalf("%d clashes", len(cl))
	}
	// the parts list: the two washers are one row
	rows := d.BOM()
	if len(rows) != 4 {
		t.Fatalf("parts list %+v", rows)
	}
	for _, r := range rows {
		if r.Name == "ISO 7089 M5" && r.Qty != 2 {
			t.Fatalf("washers: %+v", r)
		}
	}
	// the hole moves: the stack goes with it
	sk.Sketch.Params[pt.Par], sk.Sketch.Params[pt.Par+1] = 25, 12
	d.Regenerate()
	check(25, 12)
	// a washer can't go on over the screw
	w3 := washer()
	d.Insert(w3)
	if _, err := d.FastenerInHole(w3, key, rim(10)); err == nil {
		t.Fatal("a washer went on over the screw")
	}
}
