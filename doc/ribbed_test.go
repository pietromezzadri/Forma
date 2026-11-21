package doc

import (
	"math"
	"os"
	"testing"
	"time"

	"forma/brep"
	"forma/geom"
)

// polyProfile draws a closed polygon in a sketch and returns its lines.
func polyProfile(f *Feature, pts ...geom.Vec2) []int {
	s := f.Sketch
	var ids, ls []int
	for _, p := range pts {
		ids = append(ids, s.AddPoint(p).ID)
	}
	for i := range ids {
		ls = append(ls, s.AddLine(ids[i], ids[(i+1)%len(ids)]).ID)
	}
	return ls
}

// A ribbed bowl: a revolved bowl, one fin against its outer wall joined to
// it, and the fin patterned round the axis.
func TestRibbedBowl(t *testing.T) {
	d := New()
	sk := d.Insert(NewSketch(PlaneRef{Std: "xz"}))
	wall := polyProfile(sk, geom.V2(0, 0), geom.V2(60, 0), geom.V2(75, 45), geom.V2(71, 45), geom.V2(57, 4), geom.V2(0, 4))
	bowl := d.Insert(NewRevolve([]ProfileRef{{Sketch: sk.ID, Outer: wall}}, AxisRef{Kind: "y"}))
	sk2 := d.Insert(NewSketch(PlaneRef{Std: "xz"}))
	x := func(z float64) float64 { return 60 + z/3 }
	fin := polyProfile(sk2, geom.V2(x(2)-0.5, 2), geom.V2(x(42)-0.5, 42), geom.V2(x(42)+3.5, 42), geom.V2(x(2)+3.5, 2))
	ext := d.Insert(NewExtrude([]ProfileRef{{Sketch: sk2.ID, Outer: fin}}))
	ext.Inputs["Direction"], ext.Inputs["Distance"], ext.Inputs["Operation"] = "Symmetric", "2", "Join"
	t0 := time.Now()
	d.Regenerate()
	for _, f := range []*Feature{bowl, ext} {
		if f.Err != "" {
			t.Fatalf("%s: %s", f.Name, f.Err)
		}
	}
	t.Logf("bowl and a fin in %v", time.Since(t0))
	pat := d.Insert(NewPattern(nil, []int{ext.ID}, true))
	pat.Inputs["Count"] = "24"
	t0 = time.Now()
	d.Regenerate()
	if pat.Err != "" {
		t.Fatal(pat.Err)
	}
	took := time.Since(t0)
	if len(d.Bodies) != 1 {
		t.Fatalf("%d bodies, want the bowl alone", len(d.Bodies))
	}
	b := d.Bodies[0].Body
	if err := brep.Check(b); err != nil {
		t.Fatal(err)
	}
	// each fin adds 2 × 3.5 × 40 mm³ outside the wall
	one := brep.Mass(bowl.Bodies[0], 1e-5).Volume
	all := brep.Mass(b, 1e-5).Volume
	t.Logf("24 fins in %v: bowl %.0f mm³, ribbed %.0f mm³, %d faces", took, one, all, len(b.Faces()))
	if math.Abs(all-one-24*280) > 24*3 {
		t.Fatalf("ribbed bowl %.0f mm³, want %.0f", all, one+24*280)
	}
	// FORMA_SAVE_DESIGN=<file> keeps it (captures/data/ribbed.forma)
	if p := os.Getenv("FORMA_SAVE_DESIGN"); p != "" {
		if err := d.Save(p); err != nil {
			t.Fatal(err)
		}
	}
}

// One fin joined to the bowl all the way round: on the revolved wall's seam
// (0°) a cut closes on the seam, and at 180° a point of a cut lies right on
// an edge of the fin's face.
func TestFinRoundTheBowl(t *testing.T) {
	d := New()
	sk := d.Insert(NewSketch(PlaneRef{Std: "xz"}))
	wall := polyProfile(sk, geom.V2(0, 0), geom.V2(60, 0), geom.V2(75, 45), geom.V2(71, 45), geom.V2(57, 4), geom.V2(0, 4))
	bowl := d.Insert(NewRevolve([]ProfileRef{{Sketch: sk.ID, Outer: wall}}, AxisRef{Kind: "y"}))
	sk2 := d.Insert(NewSketch(PlaneRef{Std: "xz"}))
	x := func(z float64) float64 { return 60 + z/3 }
	fin := polyProfile(sk2, geom.V2(x(2)-0.5, 2), geom.V2(x(42)-0.5, 42), geom.V2(x(42)+3.5, 42), geom.V2(x(2)+3.5, 2))
	ext := d.Insert(NewExtrude([]ProfileRef{{Sketch: sk2.ID, Outer: fin}}))
	ext.Inputs["Direction"], ext.Inputs["Distance"] = "Symmetric", "2"
	d.Regenerate()
	one := brep.Mass(bowl.Bodies[0], 1e-5).Volume
	for deg := 0.0; deg < 360; deg += 22.5 {
		g, err := brep.Transform(ext.Bodies[0], geom.Rotate(geom.Vec3{}, geom.V3(0, 0, 1), deg*math.Pi/180), nil)
		if err != nil {
			t.Fatal(err)
		}
		u, err := brep.Boolean(bowl.Bodies[0], g, brep.Union)
		if err != nil {
			t.Fatalf("at %v°: %v", deg, err)
		}
		if v := brep.Mass(u, 1e-5).Volume - one; math.Abs(v-280) > 3 {
			t.Fatalf("at %v°: the fin adds %.1f mm³, want 280", deg, v)
		}
	}
}

// A bowl whose wall is a revolved spline, ribbed: each fin's top and
// bottom lie on the same circles round the wall as its neighbours'.
func TestRibbedSplineBowl(t *testing.T) {
	d := New()
	sk := d.Insert(NewSketch(PlaneRef{Std: "xz"}))
	s := sk.Sketch
	p := func(x, y float64) int { return s.AddPoint(geom.V2(x, y)).ID }
	a, b, c1, c2, e, f, g, h := p(0, 0), p(67.5, 0), p(78, 0), p(76, 20), p(72.5, 30), p(67.5, 30), p(0, 3), p(64.5, 3)
	ids := []int{s.AddLine(a, b).ID, s.AddSpline(b, c1, c2, e).ID, s.AddLine(e, f).ID, s.AddLine(f, h).ID, s.AddLine(h, g).ID, s.AddLine(g, a).ID}
	bowl := d.Insert(NewRevolve([]ProfileRef{{Sketch: sk.ID, Outer: ids}}, AxisRef{Kind: "y"}))
	sk2 := d.Insert(NewSketch(PlaneRef{Std: "xz"}))
	fin := polyProfile(sk2, geom.V2(69, 4), geom.V2(69, 26), geom.V2(78, 26), geom.V2(80, 4))
	ext := d.Insert(NewExtrude([]ProfileRef{{Sketch: sk2.ID, Outer: fin}}))
	ext.Inputs["Direction"], ext.Inputs["Distance"] = "Symmetric", "2"
	d.Regenerate()
	plain := bowl.Bodies[0]
	finBody := ext.Bodies[0]
	in, err := brep.Boolean(plain, finBody, brep.Intersect)
	if err != nil {
		t.Fatal(err)
	}
	outside := brep.Mass(finBody, 1e-5).Volume - brep.Mass(in, 1e-5).Volume
	ext.Inputs["Operation"] = "Join"
	pat := d.Insert(NewPattern(nil, []int{ext.ID}, true))
	pat.Inputs["Count"] = "24"
	t0 := time.Now()
	d.Regenerate()
	if pat.Err != "" || ext.Err != "" {
		t.Fatal(ext.Err, pat.Err)
	}
	if len(d.Bodies) != 1 {
		t.Fatalf("%d bodies", len(d.Bodies))
	}
	r := d.Bodies[0].Body
	if err := brep.Check(r); err != nil {
		t.Fatal(err)
	}
	want := brep.Mass(plain, 1e-5).Volume + 24*outside
	got := brep.Mass(r, 1e-5).Volume
	t.Logf("24 fins in %v: %.0f mm³, want %.0f", time.Since(t0), got, want)
	if math.Abs(got-want) > 1e-3*want {
		t.Fatalf("ribbed bowl %.0f mm³, want %.0f", got, want)
	}
}
