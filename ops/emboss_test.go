package ops

import (
	"math"
	"testing"
	"time"

	"forma/brep"
	"forma/geom"
	"forma/sketch"
)

// a name on a sketch standing in front of a cylinder, facing it
func nameSketch(t *testing.T, text string, y, z, size, grow float64) ([]sketch.Region, geom.Frame) {
	sketch.FontDirs = append(sketch.FontDirs, "../assets/fonts")
	// the XZ plane moved to y (its normal −Y, away from the body)
	fr := geom.Frame{O: geom.V3(0, y, 0), X: geom.XAxis, Y: geom.ZAxis, Z: geom.YAxis.Neg()}
	s := sketch.New("name", fr)
	a := s.AddPoint(geom.V2(0, z)).ID
	e := s.AddText(a, text, sketch.DefaultFont, size)
	e.Align = sketch.AlignCenter
	e.Grow = grow
	var regs []sketch.Region
	for _, r := range s.Regions() {
		if r.Ink {
			regs = append(regs, r)
		}
	}
	if len(regs) == 0 {
		t.Skip("no font")
	}
	return regs, fr
}

func sideOf(b *brep.Body) *brep.Face {
	for _, f := range b.Faces() {
		if _, ok := f.Surf.(geom.Cylinder); ok {
			return f
		}
	}
	return nil
}

func inkArea(rs []sketch.Region) float64 {
	a := 0.0
	for _, r := range rs {
		a += math.Abs(r.Area)
	}
	return a
}

// A name cut into, raised on and inlaid in a cylinder's side: valid solids,
// volumes about the letters' area times the depth.
func TestEmbossCylinder(t *testing.T) {
	cyl, err := Cylinder(geom.World, 40, 30)
	if err != nil {
		t.Fatal(err)
	}
	v0 := checkSolid(t, cyl)
	regs, fr := nameSketch(t, "Lu", -60, 15, 10, 0)
	area := inkArea(regs)
	for _, c := range []struct {
		name string
		mode int
		sign float64
	}{{"cut", EmbossCut, -1}, {"raise", EmbossRaise, 1}} {
		t0 := time.Now()
		b, _, err := EmbossFace(cyl, sideOf(cyl), regs, nil, fr, EmbossSpec{Mode: c.mode, Depth: 1})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		dv := (checkSolid(t, b) - v0) * c.sign
		t.Logf("%s: %.1f mm³ for %.1f mm² of letters (%v)", c.name, dv, area, time.Since(t0))
		// projected onto the curve the letters spread a little; cut in they
		// shrink toward the axis, raised they grow away from it
		if dv < 0.85*area || dv > 1.2*area {
			t.Fatalf("%s: volume changed by %.2f, letters %.2f mm² × 1 mm", c.name, dv, area)
		}
	}
	grown, _ := nameSketch(t, "Lu", -60, 15, 10, 0.2)
	t0 := time.Now()
	b, inl, err := EmbossFace(cyl, sideOf(cyl), regs, grown, fr, EmbossSpec{Mode: EmbossInlay, Depth: 2, Clearance: 0.2})
	if err != nil {
		t.Fatal(err)
	}
	pocket := v0 - checkSolid(t, b)
	vin := 0.0
	for _, l := range inl {
		vin += checkSolid(t, l)
	}
	t.Logf("inlay: pocket %.1f, letters %.1f (%v)", pocket, vin, time.Since(t0))
	ga := inkArea(grown)
	if len(inl) != len(regs) || math.Abs(vin-area*1.8) > 0.1*area*1.8 || math.Abs(pocket-ga*2) > 0.1*ga*2 {
		t.Fatalf("pocket %.2f (grown letters %.2f mm² × 2), inlay %.2f (%.2f mm² × 1.8) in %d pieces", pocket, ga, vin, area, len(inl))
	}
	// the inlay sits flush and in the pocket: nothing of it is left of the body
	for _, l := range inl {
		if _, err := brep.Boolean(l, b, brep.Intersect); err != brep.ErrEmpty {
			t.Fatalf("the inlay overlaps the pocketed body (%v)", err)
		}
	}
}

// The dog bowl with a name inlaid on its slanted side: the pocket cut, the
// letters made to fit with 0.2 mm all round.
func TestEmbossBowl(t *testing.T) {
	ring := sketch.New("ring", geom.World)
	c := ring.AddPoint(geom.V2(0, 0)).ID
	ring.AddCircle(c, 60)
	ring.AddCircle(c, 67.5)
	var reg sketch.Region
	for _, r := range ring.Regions() {
		if len(r.Holes) == 1 {
			reg = r
		}
	}
	body, err := Extrude("ring", reg, geom.World, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	xz := geom.Frame{X: geom.XAxis, Y: geom.ZAxis, Z: geom.YAxis.Neg()}
	s := sketch.New("foot", xz)
	a := s.AddPoint(geom.V2(67.5, 0)).ID
	h1 := s.AddPoint(geom.V2(75, 0)).ID
	h2 := s.AddPoint(geom.V2(75, 15)).ID
	e := s.AddPoint(geom.V2(72.5, 27.5)).ID
	top := s.AddPoint(geom.V2(67.5, 50)).ID
	s.AddSpline(a, h1, h2, e)
	s.AddLine(e, top)
	s.AddLine(top, a)
	foot, err := Revolve("foot", s.Regions()[0], xz, geom.V2(0, 0), geom.V2(0, 1), 2*math.Pi)
	if err != nil {
		t.Fatal(err)
	}
	bowl, err := brep.Boolean(body, foot, brep.Union)
	if err != nil {
		t.Fatal(err)
	}
	v0 := checkSolid(t, bowl)
	var side *brep.Face
	for _, f := range bowl.Faces() {
		if _, ok := f.Surf.(geom.Cone); ok {
			side = f
		}
	}
	if side == nil {
		t.Fatal("no slanted side")
	}
	regs, fr := nameSketch(t, "Luna", -90, 33, 12, 0)
	grown, _ := nameSketch(t, "Luna", -90, 33, 12, 0.2)
	t0 := time.Now()
	b, inl, err := EmbossFace(bowl, side, regs, grown, fr, EmbossSpec{Mode: EmbossInlay, Depth: 3, Clearance: 0.2})
	if err != nil {
		t.Fatal(err)
	}
	pocket := v0 - checkSolid(t, b)
	vin := 0.0
	for _, l := range inl {
		vin += checkSolid(t, l)
	}
	area, ga := inkArea(regs), inkArea(grown)
	t.Logf("pocket %.1f mm³ (%.1f mm² grown), inlay %.1f mm³ (%.1f mm²) in %v", pocket, ga, vin, area, time.Since(t0))
	// the side leans 12.5° from upright and the sketch stands upright: the
	// letters land a little taller on it
	if math.Abs(pocket-ga*3) > 0.12*ga*3 || math.Abs(vin-area*2.8) > 0.12*area*2.8 {
		t.Fatalf("pocket %.2f for %.2f mm² × 3, inlay %.2f for %.2f mm² × 2.8", pocket, ga, vin, area)
	}
	for _, l := range inl {
		if _, err := brep.Boolean(l, b, brep.Intersect); err != brep.ErrEmpty {
			t.Fatalf("the inlay overlaps the bowl (%v)", err)
		}
	}
}

// Letters round the bowl's side, not only in front: an "a" (counter,
// short segments) at several places along a cone, cut in and raised.
func TestEmbossAround(t *testing.T) {
	cone, err := Cone(geom.World, 75, 65, 60)
	if err != nil {
		t.Fatal(err)
	}
	var side *brep.Face
	for _, f := range cone.Faces() {
		if _, ok := f.Surf.(geom.Cone); ok {
			side = f
		}
	}
	v0 := checkSolid(t, cone)
	for _, x := range []float64{-24, -10, 0, 14, 20} {
		regs, fr := nameSketch(t, "a", -90, 33, 12, 0)
		for i := range regs {
			regs[i] = shiftRegion(regs[i], x)
		}
		for _, mode := range []int{EmbossCut, EmbossRaise} {
			b, _, err := EmbossFace(cone, side, regs, nil, fr, EmbossSpec{Mode: mode, Depth: 2})
			if err != nil {
				t.Fatalf("x %g mode %d: %v", x, mode, err)
			}
			if dv := math.Abs(checkSolid(t, b) - v0); dv < 1.5*inkArea(regs) || dv > 2.6*inkArea(regs) {
				t.Fatalf("x %g mode %d: volume changed by %.2f for %.2f mm² × 2", x, mode, dv, inkArea(regs))
			}
		}
	}
}

// shiftRegion moves a region dx along the sketch's X.
func shiftRegion(r sketch.Region, dx float64) sketch.Region {
	mv := func(l []sketch.Seg) []sketch.Seg {
		out := make([]sketch.Seg, len(l))
		d := geom.V2(dx, 0)
		for i, s := range l {
			s.A, s.B, s.C = s.A.Add(d), s.B.Add(d), s.C.Add(d)
			if s.Ctrl != nil {
				c := make([]geom.Vec2, len(s.Ctrl))
				for k, q := range s.Ctrl {
					c[k] = q.Add(d)
				}
				s.Ctrl = c
			}
			out[i] = s
		}
		return out
	}
	r.Outer = mv(r.Outer)
	holes := make([][]sketch.Seg, len(r.Holes))
	for i, h := range r.Holes {
		holes[i] = mv(h)
	}
	r.Holes = holes
	poly := make([]geom.Vec2, len(r.Poly))
	for i, p := range r.Poly {
		poly[i] = p.Add(geom.V2(dx, 0))
	}
	r.Poly = poly
	return r
}
