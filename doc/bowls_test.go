package doc

import (
	"math"
	"testing"

	"forma/brep"
	"forma/geom"
	"forma/sketch"
)

// Two 3D-printed pet bowls from photos, and the kernel cases they found:
// a hole across a tapered wall, a body split along a wave, a name raised
// or cut on a revolved spline.

// waveSlab draws a region under a wave of cubics with level tangents at
// its crests and troughs (heights hs, evenly spaced from x0 to −x0), down
// to lo, and returns its curves.
func waveSlab(s *sketch.Sketch, x0, lo float64, hs []float64) []int {
	p := func(x, y float64) int { return s.AddPoint(geom.V2(x, y)).ID }
	n := len(hs) - 1
	w := 2 * x0 / float64(n)
	a, b := p(-x0, lo), p(x0, lo)
	ids := []int{s.AddLine(a, b).ID}
	prev := p(x0, hs[0])
	ids = append(ids, s.AddLine(b, prev).ID)
	for i := 1; i <= n; i++ {
		x1, x2 := x0-w*float64(i-1), x0-w*float64(i)
		end := p(x2, hs[i])
		ids = append(ids, s.AddSpline(prev, p(x1-w/3, hs[i-1]), p(x2+w/3, hs[i]), end).ID)
		prev = end
	}
	return append(ids, s.AddLine(prev, a).ID)
}

func regen(t *testing.T, d *Document, fs ...*Feature) {
	t.Helper()
	d.Regenerate()
	for _, f := range fs {
		if f.Err != "" {
			t.Fatalf("%s: %s", f.Name, f.Err)
		}
	}
}

func newBody(t *testing.T, d *Document, f *Feature) *brep.Body {
	t.Helper()
	if f.Inputs["Operation"] == "" && f.Kind != KRevolve {
		f.Inputs["Operation"] = "New body"
	}
	regen(t, d, f)
	return f.Bodies[0]
}

// taperedCup is a revolved cone 72 → 60 mm in radius, 80 tall; with a
// pocket for a steel bowl when pocket is set.
func taperedCup(t *testing.T, d *Document, pocket bool) *Feature {
	sk := d.Insert(NewSketch(PlaneRef{Std: "xz"}))
	pts := []geom.Vec2{geom.V2(0, 0), geom.V2(72, 0), geom.V2(60, 80), geom.V2(0, 80)}
	if pocket {
		pts = []geom.Vec2{geom.V2(0, 0), geom.V2(72, 0), geom.V2(60, 80), geom.V2(53, 80), geom.V2(53, 45), geom.V2(0, 45)}
	}
	return d.Insert(NewRevolve([]ProfileRef{{Sketch: sk.ID, Outer: polyProfile(sk, pts...)}}, AxisRef{Kind: "y"}))
}

// A round hole across a tapered wall: an extruded circle is two half
// cylinders, and the cut round the hole comes back to each half a turn on.
func TestHoleAcrossTaperedWall(t *testing.T) {
	for _, c := range []struct {
		name  string
		plane string
		z     float64
		cup   bool
	}{
		{"along X, on the cone's seam", "yz", 30, false},
		{"along Y, off the seam", "xz", 30, false},
		{"along X through the wall and the pocket", "yz", 62, true},
		{"along Y through the wall and the pocket", "xz", 62, true},
	} {
		d := New()
		cone := newBody(t, d, taperedCup(t, d, c.cup))
		sk := d.Insert(NewSketch(PlaneRef{Std: c.plane}))
		ci := sk.Sketch.AddCircle(sk.Sketch.AddPoint(geom.V2(0, c.z)).ID, 10)
		e := d.Insert(NewExtrude([]ProfileRef{{Sketch: sk.ID, Outer: []int{ci.ID}}}))
		e.Inputs["Direction"], e.Inputs["Distance"] = "Symmetric", "200"
		hole := newBody(t, d, e)
		cut, err := brep.Boolean(cone, hole, brep.Subtract)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		in, err := brep.Boolean(cone, hole, brep.Intersect)
		if err != nil {
			t.Fatalf("%s (the plug): %v", c.name, err)
		}
		v0, v1, v2 := volume(t, cone), volume(t, cut), volume(t, in)
		if math.Abs(v1+v2-v0) > 1e-4*v0 || v2 < 1000 {
			t.Fatalf("%s: %.0f + %.0f mm³, the cone %.0f", c.name, v1, v2, v0)
		}
	}
}

// A cone split along a wave into two bodies (a two-colour print): the
// wave's spline faces carried on past their ends, so the curve round the
// cone crosses each face's edge and nothing past it is taken for a cut.
func TestWavySplit(t *testing.T) {
	for _, c := range []struct {
		name  string
		plane string
		hs    []float64
	}{
		{"wave along X", "xz", []float64{26, 36, 26, 36, 26}},
		{"wave along Y, a trough on the seam", "yz", []float64{26, 36, 26, 36, 26}},
		{"wave along Y, a crest on the seam", "yz", []float64{36, 26, 36, 26, 36}},
	} {
		for _, pocket := range []bool{false, true} {
			d := New()
			cone := newBody(t, d, taperedCup(t, d, pocket))
			sk := d.Insert(NewSketch(PlaneRef{Std: c.plane}))
			e := d.Insert(NewExtrude([]ProfileRef{{Sketch: sk.ID, Outer: waveSlab(sk.Sketch, 95, -5, c.hs)}}))
			e.Inputs["Direction"], e.Inputs["Distance"] = "Symmetric", "200"
			wave := newBody(t, d, e)
			lo, err := brep.Boolean(cone, wave, brep.Intersect)
			if err != nil {
				t.Fatalf("%s: below: %v", c.name, err)
			}
			hi, err := brep.Boolean(cone, wave, brep.Subtract)
			if err != nil {
				t.Fatalf("%s: above: %v", c.name, err)
			}
			v0, v1, v2 := volume(t, cone), volume(t, lo), volume(t, hi)
			if math.Abs(v1+v2-v0) > 5e-4*v0 { // meshed volumes: the curved seam
				t.Fatalf("%s: %.0f + %.0f mm³, the cone %.0f", c.name, v1, v2, v0)
			}
		}
	}
}

// bulb is a bowl whose outside is one revolved cubic, with a pocket for a
// steel bowl; it returns the feature and its spline face.
func bulb(t *testing.T, d *Document) (*Feature, string) {
	sk := d.Insert(NewSketch(PlaneRef{Std: "xz"}))
	s := sk.Sketch
	p := func(x, y float64) int { return s.AddPoint(geom.V2(x, y)).ID }
	a, b, c1, c2, e, f, g, h := p(0, 0), p(62, 0), p(84, 10), p(82, 42), p(62, 55), p(52, 55), p(52, 25), p(0, 25)
	ids := []int{s.AddLine(a, b).ID, s.AddSpline(b, c1, c2, e).ID, s.AddLine(e, f).ID, s.AddLine(f, g).ID, s.AddLine(g, h).ID, s.AddLine(h, a).ID}
	bowl := d.Insert(NewRevolve([]ProfileRef{{Sketch: sk.ID, Outer: ids}}, AxisRef{Kind: "y"}))
	regen(t, d, bowl)
	for _, fc := range bowl.Bodies[0].Faces() {
		if _, ok := fc.Surf.(geom.Revolved); ok {
			return bowl, fc.Name
		}
	}
	t.Fatal("no spline face")
	return nil, ""
}

// A name raised and cut on a revolved spline: the layer it is built in is
// offset copies of the face (split at a profile knot), carried on past
// their profiles' ends; the letters' flat feet lie on the circle the
// letters cut before them left on the bowl.
func TestNameOnRevolvedSpline(t *testing.T) {
	sketch.FontDirs = append(sketch.FontDirs, "../assets/fonts")
	for _, c := range []struct {
		typ  string
		size float64
	}{{"Raise", 22}, {"Raise", 12}, {"Engrave", 22}, {"Engrave", 12}} {
		d := New()
		bowl, face := bulb(t, d)
		v0 := volume(t, bowl.Bodies[0])
		sk := d.Insert(NewSketch(PlaneRef{Std: "xz"}))
		e := sk.Sketch.AddText(sk.Sketch.AddPoint(geom.V2(0, 27-c.size/2)).ID, "ARA", sketch.DefaultFont, c.size)
		e.Align = sketch.AlignCenter
		em := d.Insert(NewEmboss([]ProfileRef{{Sketch: sk.ID, Outer: []int{e.ID}, Text: e.ID}}, BodyKey(bowl.ID, 0), FaceRef{Feature: bowl.ID, Face: face}))
		em.Inputs["Type"], em.Inputs["Depth"] = c.typ, "2"
		regen(t, d, em)
		v := volume(t, d.Bodies[0].Body)
		if (c.typ == "Raise") != (v > v0) {
			t.Fatalf("%s %g mm: volume %.0f, the bowl %.0f", c.typ, c.size, v, v0)
		}
	}
}

// The tapered bowl: split along a wave into a base and a top for two
// colours, oval hand holes through the top's tapered wall, and its name
// raised and wrapped round the cone.
func TestTwoColourBowl(t *testing.T) {
	sketch.FontDirs = append(sketch.FontDirs, "../assets/fonts")
	d := New()
	sk := d.Insert(NewSketch(PlaneRef{Std: "xz"}))
	prof := polyProfile(sk, geom.V2(0, 0), geom.V2(72, 0), geom.V2(60, 80), geom.V2(53, 80), geom.V2(53, 45), geom.V2(0, 45))
	r1 := d.Insert(NewRevolve([]ProfileRef{{Sketch: sk.ID, Outer: prof}}, AxisRef{Kind: "y"}))
	r2 := d.Insert(NewRevolve([]ProfileRef{{Sketch: sk.ID, Outer: prof}}, AxisRef{Kind: "y"}))
	r2.Inputs["Operation"] = "New body"
	sk2 := d.Insert(NewSketch(PlaneRef{Std: "xz"}))
	tool := d.Insert(NewExtrude([]ProfileRef{{Sketch: sk2.ID, Outer: waveSlab(sk2.Sketch, 95, -5, []float64{26, 36, 26, 36, 26})}}))
	tool.Inputs["Direction"], tool.Inputs["Distance"], tool.Inputs["Operation"] = "Symmetric", "200", "New body"
	regen(t, d, r1, r2, tool)
	whole := volume(t, r1.Bodies[0])
	base := d.Insert(NewCombine(BodyKey(r1.ID, 0), []int{BodyKey(tool.ID, 0)}))
	base.Inputs["Operation"], base.Inputs["Keep tools"] = "Intersect", "1"
	top := d.Insert(NewCombine(BodyKey(r2.ID, 0), []int{BodyKey(tool.ID, 0)}))
	top.Inputs["Operation"] = "Cut"
	regen(t, d, base, top)
	if len(d.Bodies) != 2 {
		t.Fatalf("%d bodies, want the base and the top", len(d.Bodies))
	}
	if v := volume(t, d.Bodies[0].Body) + volume(t, d.Bodies[1].Body); math.Abs(v-whole) > 5e-4*whole {
		t.Fatalf("base and top %.0f mm³, the bowl %.0f", v, whole)
	}
	// the hand holes: a slot, two half circles and two lines, across X
	sk3 := d.Insert(NewSketch(PlaneRef{Std: "yz"}))
	hs := sk3.Sketch
	hp := func(x, y float64) int { return hs.AddPoint(geom.V2(x, y)).ID }
	rc, lc := hp(7, 62), hp(-7, 62)
	t1, t2, b1, b2 := hp(-7, 69), hp(7, 69), hp(7, 55), hp(-7, 55)
	slot := []int{hs.AddLine(t1, t2).ID, hs.AddArc(rc, b1, t2).ID, hs.AddLine(b1, b2).ID, hs.AddArc(lc, t1, b2).ID}
	holes := d.Insert(NewExtrude([]ProfileRef{{Sketch: sk3.ID, Outer: slot}}))
	holes.Inputs["Direction"], holes.Inputs["Distance"], holes.Inputs["Operation"] = "Symmetric", "200", "Cut"
	regen(t, d, holes)
	// the name on the top's cone, in front
	var side string
	for _, b := range d.Bodies {
		if b.Key != BodyKey(r2.ID, 0) {
			continue
		}
		for _, fc := range b.Body.Faces() {
			if _, ok := fc.Surf.(geom.Cone); ok {
				side = fc.Name
			}
		}
	}
	if side == "" {
		t.Fatal("no cone face on the top")
	}
	at := geom.V3(0, -62.7, 62)
	sk4 := d.Insert(NewSketch(PlaneRef{Feature: r2.ID, Face: side, At: &at}))
	e := sk4.Sketch.AddText(sk4.Sketch.AddPoint(geom.V2(0, -8)).ID, "GUTA", sketch.DefaultFont, 20)
	e.Align = sketch.AlignCenter
	em := d.Insert(NewEmboss([]ProfileRef{{Sketch: sk4.ID, Outer: []int{e.ID}, Text: e.ID}}, BodyKey(r2.ID, 0), FaceRef{Feature: r2.ID, Face: side}))
	em.Inputs["Type"], em.Inputs["Depth"], em.Inputs["Wrap"] = "Raise", "1.2", "1"
	regen(t, d, sk4, em)
	for _, b := range d.Bodies {
		volume(t, b.Body) // valid
	}
}

// namedBulb is the bulb with ARA raised 4 mm on it and one rib following
// its curve (op: the rib's Operation).
func namedBulb(t *testing.T, op string) (*Document, *Feature, *Feature) {
	sketch.FontDirs = append(sketch.FontDirs, "../assets/fonts")
	d := New()
	bowl, face := bulb(t, d)
	sk := d.Insert(NewSketch(PlaneRef{Std: "xz"}))
	e := sk.Sketch.AddText(sk.Sketch.AddPoint(geom.V2(0, 16)).ID, "ARA", sketch.DefaultFont, 22)
	e.Align = sketch.AlignCenter
	em := d.Insert(NewEmboss([]ProfileRef{{Sketch: sk.ID, Outer: []int{e.ID}, Text: e.ID}}, BodyKey(bowl.ID, 0), FaceRef{Feature: bowl.ID, Face: face}))
	em.Inputs["Type"], em.Inputs["Depth"] = "Raise", "4"
	fsk := d.Insert(NewSketch(PlaneRef{Std: "xz"}))
	fs := fsk.Sketch
	q := func(x, y float64) int { return fs.AddPoint(geom.V2(x, y)).ID }
	i0, i1, o0, k1, k2, o1 := q(56, 2), q(56, 53), q(64.5, 2), q(87.5, 11), q(85.5, 42), q(65, 53)
	fin := []int{fs.AddLine(i0, o0).ID, fs.AddSpline(o0, k1, k2, o1).ID, fs.AddLine(o1, i1).ID, fs.AddLine(i1, i0).ID}
	ext := d.Insert(NewExtrude([]ProfileRef{{Sketch: fsk.ID, Outer: fin}}))
	ext.Inputs["Direction"], ext.Inputs["Distance"], ext.Inputs["Operation"] = "Symmetric", "2.4", op
	regen(t, d, em, ext)
	return d, em, ext
}

// Ribs passing a hair from a letter's corners (K8): at 285° a rib's side
// misses the A's top corner by 0.6 µm (its crossings by the corner are the
// corner), at 252° and 276° a rib's outer edge passes 0.02 mm from a
// letter's corner line (sorted out at finer tolerances).
func TestRibByLetterCorners(t *testing.T) {
	d, _, ext := namedBulb(t, "New body")
	bowl := d.Bodies[0].Body
	for _, deg := range []float64{252, 276, 285} {
		rib, err := brep.Transform(ext.Bodies[0], geom.Rotate(geom.Vec3{}, geom.V3(0, 0, 1), deg*math.Pi/180), nil)
		if err != nil {
			t.Fatal(err)
		}
		u, err := brep.Boolean(bowl, rib, brep.Union)
		if err != nil {
			t.Fatalf("the rib at %g°: %v", deg, err)
		}
		if v, v0 := volume(t, u), volume(t, bowl); v <= v0 {
			t.Fatalf("the rib at %g° added nothing (%.0f → %.0f mm³)", deg, v0, v)
		}
	}
}

// The ribbed bowl: the bulb, the name raised on it, then 36 ribs following
// its curve patterned round and joined through the letters (60 work too,
// in about a minute: TestRibByLetterCorners has the ribs that needed K8).
func TestRibbedBulbWithName(t *testing.T) {
	d, em, ext := namedBulb(t, "Join")
	pat := d.Insert(NewPattern(nil, []int{ext.ID}, true))
	pat.Inputs["Count"] = "36"
	regen(t, d, em, ext, pat)
	if len(d.Bodies) != 1 {
		t.Fatalf("%d bodies, want one", len(d.Bodies))
	}
	volume(t, d.Bodies[0].Body)
}
