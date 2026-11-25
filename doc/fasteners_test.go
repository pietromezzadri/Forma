package doc

import (
	"math"
	"testing"

	"forma/brep"
	"forma/geom"
)

// Every standard's every size at its shortest and a middle length makes one
// valid solid whose volume is about the head's and the shank's; the shank
// reaches −Length; a countersunk head's top is at 0.
func TestScrewDesigns(t *testing.T) {
	n := 0
	for _, std := range ScrewStds {
		for _, size := range std.Sizes() {
			ls := std.Lengths(size)
			for _, l := range []float64{ls[0], ls[len(ls)/2]} {
				spec := ScrewSpec{std.ID, size, l}
				d, err := ScrewDesign(spec)
				if err != nil {
					t.Fatalf("%s: %v", spec.Label(), err)
				}
				b := d.Bodies[0].Body
				if err := brep.Check(b); err != nil {
					t.Fatalf("%s: %v", spec.Label(), err)
				}
				bx := b.Bounds()
				m, h := metrics[size], std.dims[size]
				if math.Abs(bx.Min.Z+l) > 1e-6 {
					t.Fatalf("%s: the shank ends at %.3f, want %.3f", spec.Label(), bx.Min.Z, -l)
				}
				wantTop := h.k
				if std.Sunk {
					wantTop = 0
				}
				// (a button head's socket takes the top of its dome)
				if math.Abs(bx.Max.Z-wantTop) > 1e-6 && !(std.Button && bx.Max.Z < wantTop && bx.Max.Z > wantTop-h.t) {
					t.Fatalf("%s: the head's top at %.3f, want %.3f", spec.Label(), bx.Max.Z, wantTop)
				}
				// the volume lies between the shank alone and shank plus a
				// full cylinder of the head's size
				shank := math.Pi * m.d * m.d / 4 * l
				head := math.Pi * h.dk * h.dk / 4 * h.k
				if std.Hex {
					head = math.Sqrt(3) / 2 * h.dk * h.dk * h.k
				}
				v := volume(t, b)
				if v < shank*0.6 || v > shank+head*1.05 {
					t.Fatalf("%s: volume %.2f outside %.2f..%.2f", spec.Label(), v, shank*0.6, shank+head)
				}
				n++
			}
		}
	}
	t.Logf("%d screws built", n)
}

// A screw component: its name and file, its body placed by the
// component's move; its size edited rebuilds it; two of the same are one
// row of the parts list.
func TestScrewComponent(t *testing.T) {
	d := New()
	f, err := NewScrew(ScrewSpec{"ISO 4762", "M5", 16})
	if err != nil {
		t.Fatal(err)
	}
	d.Insert(f)
	g, _ := NewScrew(ScrewSpec{"ISO 4762", "M5", 16})
	g.Inputs["Move X"] = "20"
	d.Insert(g)
	d.Regenerate()
	if f.Err != "" || g.Err != "" || len(d.Bodies) != 2 {
		t.Fatalf("%q %q, %d bodies", f.Err, g.Err, len(d.Bodies))
	}
	if f.Name != "ISO 4762 M5×16" {
		t.Fatalf("name %q", f.Name)
	}
	rows := d.BOM()
	if len(rows) != 1 || rows[0].Qty != 2 {
		t.Fatalf("parts list: %+v", rows)
	}
	// the second is moved 20 along X
	if c := d.Bodies[1].Body.Bounds().Center(); math.Abs(c.X-20) > 1e-6 {
		t.Fatalf("the moved screw is at x %.3f", c.X)
	}
	// M5 → M8: a bigger head
	g.Inputs["Size"] = "M8"
	d.Regenerate()
	if g.Err != "" {
		t.Fatal(g.Err)
	}
	bx := d.Bodies[1].Body.Bounds()
	if w := bx.Max.X - bx.Min.X; math.Abs(w-13) > 1e-3 {
		t.Fatalf("an M8 head is %.3f across, want 13", w)
	}
	if len(d.BOM()) != 2 {
		t.Fatal("an M5 and an M8 should be two rows")
	}
	// a bad size says so
	g.Inputs["Size"] = "M7"
	d.Regenerate()
	if g.Err == "" {
		t.Fatal("M7 should fail")
	}
	_ = geom.Tol
}

// Holes for screws: an M5 counterbored clearance hole through a block
// takes the volume the tables say, and an ISO 4762 M5×16 sat on the
// counterbore's floor doesn't touch the block; tapped and countersunk holes
// take their diameters.
func TestScrewHoles(t *testing.T) {
	block := func(typ, fit string) (*Document, *Feature) {
		d := New()
		bx := d.Insert(NewPrim(KBox))
		bx.Inputs["Width"], bx.Inputs["Depth"], bx.Inputs["Height"] = "40", "30", "20"
		sk := d.Insert(NewSketch(PlaneRef{Std: "xy"}))
		sk.Inputs = map[string]string{"Offset": "20"}
		p := sk.Sketch.AddPoint(geom.V2(10, 10)).ID
		h := d.Insert(NewHole(sk.ID, []int{p}))
		h.Inputs["Type"], h.Inputs["Extent"], h.Inputs["Screw"], h.Inputs["Fit"] = typ, "Through all", "M5", fit
		d.Regenerate()
		if h.Err != "" {
			t.Fatalf("%s %s: %s", typ, fit, h.Err)
		}
		return d, h
	}
	d, _ := block("Counterbore", "Normal")
	b := d.Bodies[0].Body
	cut := 24000 - volume(t, b)
	want := math.Pi * (2.75*2.75*(20-5.4) + 5*5*5.4)
	t.Logf("M5 counterbored: %.2f mm³ cut, %.2f by the tables", cut, want)
	if math.Abs(cut-want) > 0.01*want {
		t.Fatalf("cut %.2f, want %.2f", cut, want)
	}
	s, err := NewScrew(ScrewSpec{"ISO 4762", "M5", 16})
	if err != nil {
		t.Fatal(err)
	}
	s.Inputs["Move X"], s.Inputs["Move Y"], s.Inputs["Move Z"] = "10", "10", num(20-5.4)
	d.Insert(s)
	d.Regenerate()
	if s.Err != "" {
		t.Fatal(s.Err)
	}
	if _, err := brep.Boolean(d.Bodies[0].Body, d.Bodies[1].Body, brep.Intersect); err != brep.ErrEmpty {
		t.Fatalf("the screw and the block overlap (%v)", err)
	}
	// tapped: the 4.2 mm tap drill; countersunk: an 11.4 mm countersink
	d, _ = block("Simple", "Tapped")
	if cut := 24000 - volume(t, d.Bodies[0].Body); math.Abs(cut-math.Pi*2.1*2.1*20) > 0.01*cut {
		t.Fatalf("tapped: %.2f cut, want %.2f", cut, math.Pi*2.1*2.1*20)
	}
	d, _ = block("Countersink", "Normal")
	top := 0.0
	for _, e := range d.Bodies[0].Body.Edges() {
		if c, ok := e.Curve.(geom.Circle); ok && math.Abs(c.F.O.Z-20) < 1e-6 {
			top = math.Max(top, 2*c.R)
		}
	}
	if math.Abs(top-11.4) > 1e-6 {
		t.Fatalf("countersink %.3f across the top, want 11.4", top)
	}
	// a size the tables don't have
	d, _ = New(), (*Feature)(nil)
	if _, _, _, _, err := ScrewHoleSizes("M7", "Normal"); err == nil {
		t.Fatal("M7 should fail")
	}
}

// A screw jointed into its hole: it sits on the counterbore's floor (picked
// by the counterbore's top rim), flush in a countersink, on the face over a
// plain hole — never overlapping the block — and follows the hole when the
// hole's sketch point moves; the joint's angle turns it about the hole's
// axis.
func TestScrewInHole(t *testing.T) {
	for _, tc := range []struct {
		typ, std string
		rimR     float64 // the rim picked (at the top face)
		seatZ    float64 // where the screw's frame should end up
	}{
		{"Counterbore", "ISO 4762", 5, 20 - 5.4},
		{"Countersink", "ISO 10642", 5.7, 20},
		{"Simple", "ISO 7380", 2.75, 20},
	} {
		d := New()
		bx := d.Insert(NewPrim(KBox))
		bx.Inputs["Width"], bx.Inputs["Depth"], bx.Inputs["Height"] = "40", "30", "20"
		sk := d.Insert(NewSketch(PlaneRef{Std: "xy"}))
		sk.Inputs = map[string]string{"Offset": "20"}
		pt := sk.Sketch.AddPoint(geom.V2(10, 10))
		h := d.Insert(NewHole(sk.ID, []int{pt.ID}))
		h.Inputs["Type"], h.Inputs["Extent"], h.Inputs["Screw"], h.Inputs["Fit"] = tc.typ, "Through all", "M5", "Normal"
		d.Regenerate()
		if h.Err != "" {
			t.Fatalf("%s: %s", tc.typ, h.Err)
		}
		var rim *brep.Edge
		for _, e := range d.Bodies[0].Body.Edges() {
			if c, ok := e.Curve.(geom.Circle); ok && math.Abs(c.F.O.Z-20) < 1e-6 && math.Abs(c.R-tc.rimR) < 1e-6 {
				rim = e
			}
		}
		if rim == nil {
			t.Fatalf("%s: no rim of radius %g", tc.typ, tc.rimR)
		}
		if got := d.HoleScrew(rim); got != "M5" {
			t.Fatalf("%s: the hole is for %q", tc.typ, got)
		}
		s, err := NewScrew(ScrewSpec{tc.std, "M5", 16})
		if err != nil {
			t.Fatal(err)
		}
		d.Insert(s) // at the origin: the joint takes it to the hole
		j, err := d.ScrewInHole(s, d.Bodies[0].Key, rim)
		if err != nil {
			t.Fatalf("%s: %v", tc.typ, err)
		}
		d.Insert(j)
		d.Regenerate()
		if s.Err != "" || j.Err != "" {
			t.Fatalf("%s: %q %q", tc.typ, s.Err, j.Err)
		}
		seat := func(x, y float64) {
			t.Helper()
			sb := d.Bodies[1].Body
			// the screw's frame origin (0,0,0) and shank end (0,0,−16)
			o := sb.Bounds()
			if math.Abs(o.Min.Z-(tc.seatZ-16)) > 1e-6 {
				t.Fatalf("%s: the shank ends at z %.4f, want %.4f", tc.typ, o.Min.Z, tc.seatZ-16)
			}
			if c := o.Center(); math.Hypot(c.X-x, c.Y-y) > 1e-6 {
				t.Fatalf("%s: the screw is at (%.4f, %.4f), want (%g, %g)", tc.typ, c.X, c.Y, x, y)
			}
			if _, err := brep.Boolean(d.Bodies[0].Body, sb, brep.Intersect); err != brep.ErrEmpty {
				t.Fatalf("%s: the screw and the block overlap (%v)", tc.typ, err)
			}
		}
		seat(10, 10)
		// the hole moves: the screw goes with it
		sk.Sketch.Params[pt.Par], sk.Sketch.Params[pt.Par+1] = 25, 15
		d.Regenerate()
		if j.Err != "" {
			t.Fatalf("%s after the move: %s", tc.typ, j.Err)
		}
		seat(25, 15)
		// turned 30° in its hole: still in it
		j.Inputs["Angle"] = "30"
		d.Regenerate()
		if j.Err != "" {
			t.Fatal(j.Err)
		}
		seat(25, 15)
		// saved and opened again: the same
		d2, err := Unmarshal(d.Marshal())
		if err != nil {
			t.Fatal(err)
		}
		d2.Regenerate()
		if len(d2.Bodies) != 2 || d2.Bodies[1].Body.Bounds().Min.Dist(d.Bodies[1].Body.Bounds().Min) > 1e-6 {
			t.Fatalf("%s: reopened, the screw moved", tc.typ)
		}
	}
}
