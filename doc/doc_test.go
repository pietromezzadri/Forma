package doc

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"testing"

	"forma/brep"
	"forma/geom"
	"forma/sketch"
)

func volume(t *testing.T, b *brep.Body) float64 {
	t.Helper()
	if err := brep.Check(b); err != nil {
		t.Fatal(err)
	}
	return brep.Tessellate(b, brep.DefaultMeshOptions(b)).Volume()
}

// rect adds a w × h rectangle with its corner fixed at the origin and
// dimensions written as expressions.
func rect(s *sketch.Sketch, wExpr, hExpr string) {
	p := func(x, y float64) int { return s.AddPoint(geom.V2(x, y)).ID }
	a, b, c, d := p(0, 0), p(30, 0), p(30, 20), p(0, 20)
	l0, l1, l2, l3 := s.AddLine(a, b), s.AddLine(b, c), s.AddLine(c, d), s.AddLine(d, a)
	s.AddConstraint(sketch.Horizontal, []int{l0.ID}, 0)
	s.AddConstraint(sketch.Horizontal, []int{l2.ID}, 0)
	s.AddConstraint(sketch.Vertical, []int{l1.ID}, 0)
	s.AddConstraint(sketch.Vertical, []int{l3.ID}, 0)
	s.AddConstraint(sketch.Fix, []int{a}, 0)
	cw, _ := s.AddConstraint(sketch.Distance, []int{a, b}, 30)
	ch, _ := s.AddConstraint(sketch.Distance, []int{b, c}, 20)
	cw.Expr, ch.Expr = wExpr, hExpr
}

// a design: parameters w and h, a sketch dimensioned by them, an extrude.
func design(t *testing.T) (*Document, *Feature, *Feature) {
	d := New()
	d.AddParam("w", "40")
	d.AddParam("depth", "w * 0.75")
	d.AddParam("h", "w / 2")
	sk := d.Insert(NewSketch(PlaneRef{Std: "xy"}))
	rect(sk.Sketch, "w", "depth")
	d.Regenerate()
	if sk.Err != "" {
		t.Fatal(sk.Err)
	}
	if len(sk.Regions) != 1 {
		t.Fatalf("%d regions", len(sk.Regions))
	}
	ex := d.Insert(NewExtrude([]ProfileRef{RefFor(sk.ID, sk.Regions[0])}))
	ex.Inputs["Distance"] = "h"
	d.Regenerate()
	if ex.Err != "" {
		t.Fatal(ex.Err)
	}
	return d, sk, ex
}

func TestParametricExtrude(t *testing.T) {
	d, _, ex := design(t)
	if v := volume(t, ex.Bodies[0]); math.Abs(v-40*30*20) > 1e-6 {
		t.Fatalf("volume %.3f", v)
	}
	d.Params[0].Expr = "60"
	d.Regenerate()
	if ex.Err != "" {
		t.Fatal(ex.Err)
	}
	if v := volume(t, ex.Bodies[0]); math.Abs(v-60*45*30) > 1e-6 {
		t.Fatalf("after w = 60: volume %.3f", v)
	}
	// nothing changed: nothing is rebuilt
	g := ex.Gen
	d.Regenerate()
	if ex.Gen != g {
		t.Fatal("an unchanged extrude was rebuilt")
	}
}

func TestSaveLoadUndo(t *testing.T) {
	d, _, _ := design(t)
	var buf bytes.Buffer
	if err := d.Encode(&buf); err != nil {
		t.Fatal(err)
	}
	d2, err := Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	ex2 := d2.Features[1]
	if ex2.Err != "" || len(ex2.Bodies) != 1 {
		t.Fatalf("loaded extrude: %q, %d bodies", ex2.Err, len(ex2.Bodies))
	}
	if v := volume(t, ex2.Bodies[0]); math.Abs(v-24000) > 1e-6 {
		t.Fatalf("loaded volume %.3f", v)
	}

	h := NewHistory(d)
	d.Params[0].Expr = "50"
	d.Regenerate()
	h.Push(d)
	u, err := h.Undo()
	if err != nil {
		t.Fatal(err)
	}
	if v := volume(t, u.Features[1].Bodies[0]); math.Abs(v-24000) > 1e-6 {
		t.Fatalf("undone volume %.3f", v)
	}
	r, err := h.Redo()
	if err != nil {
		t.Fatal(err)
	}
	if v := volume(t, r.Features[1].Bodies[0]); math.Abs(v-50*37.5*25) > 1e-6 {
		t.Fatalf("redone volume %.3f", v)
	}
	if h.CanRedo() {
		t.Fatal("redo past the end")
	}
}

func TestRollbackAndSuppress(t *testing.T) {
	d, sk, ex := design(t)
	d.Rollback = 1
	d.Regenerate()
	if ex.Active || len(ex.Bodies) != 0 {
		t.Fatal("rolled-back extrude still has a body")
	}
	// a feature inserted now goes at the marker, before the extrude
	b := d.Insert(NewPrim(KBox))
	if d.Index(b.ID) != 1 || d.Rollback != 2 {
		t.Fatalf("inserted at %d, marker %d", d.Index(b.ID), d.Rollback)
	}
	d.Rollback = -1
	sk.Off = true
	d.Regenerate()
	if !strings.Contains(ex.Err, "suppressed") {
		t.Fatalf("extrude of a suppressed sketch: %q", ex.Err)
	}
}

// A sketch on a box's top face follows the face when the box grows.
func TestSketchOnFace(t *testing.T) {
	d := New()
	d.AddParam("H", "20")
	box := d.Insert(NewPrim(KBox))
	box.Inputs["Height"] = "H"
	sk := d.Insert(NewSketch(PlaneRef{Feature: box.ID, Face: "top"}))
	sk.Sketch.AddCircle(sk.Sketch.AddPoint(geom.V2(20, 15)).ID, 8)
	d.Regenerate()
	if sk.Err != "" || len(sk.Regions) != 1 {
		t.Fatalf("sketch: %q, %d regions", sk.Err, len(sk.Regions))
	}
	boss := d.Insert(NewExtrude([]ProfileRef{RefFor(sk.ID, sk.Regions[0])}))
	boss.Inputs["Distance"] = "10"
	d.Regenerate()
	if boss.Err != "" {
		t.Fatal(boss.Err)
	}
	bb := boss.Bodies[0].Bounds()
	if math.Abs(bb.Min.Z-20) > 1e-6 || math.Abs(bb.Max.Z-30) > 1e-6 {
		t.Fatalf("boss spans z %.3f..%.3f", bb.Min.Z, bb.Max.Z)
	}
	if c := bb.Center(); math.Abs(c.X-20) > 1e-6 || math.Abs(c.Y-15) > 1e-6 {
		t.Fatalf("boss centred at %v", c)
	}
	d.Params[0].Expr = "50"
	d.Regenerate()
	bb = boss.Bodies[0].Bounds()
	if math.Abs(bb.Min.Z-50) > 1e-6 {
		t.Fatalf("after H = 50 the boss starts at z %.3f", bb.Min.Z)
	}
	// delete the box: the sketch loses its face
	d.Remove(box.ID)
	d.Regenerate()
	if sk.Err == "" {
		t.Fatal("sketch on a deleted face has no error")
	}
}

func TestRevolveFeature(t *testing.T) {
	d := New()
	sk := d.Insert(NewSketch(PlaneRef{Std: "xz"}))
	s := sk.Sketch
	p := func(x, y float64) int { return s.AddPoint(geom.V2(x, y)).ID }
	a, b, c, e := p(10, 0), p(20, 0), p(20, 30), p(10, 30)
	s.AddLine(a, b)
	s.AddLine(b, c)
	s.AddLine(c, e)
	s.AddLine(e, a)
	d.Regenerate()
	rv := d.Insert(NewRevolve([]ProfileRef{RefFor(sk.ID, sk.Regions[0])}, AxisRef{Kind: "y"}))
	d.Regenerate()
	if rv.Err != "" {
		t.Fatal(rv.Err)
	}
	want := math.Pi * 300 * 30
	if v := volume(t, rv.Bodies[0]); math.Abs(v-want)/want > 5e-3 {
		t.Fatalf("volume %.2f, want %.2f", v, want)
	}
	rv.Inputs["Angle"] = "90"
	d.Regenerate()
	if v := volume(t, rv.Bodies[0]); math.Abs(v-want/4)/want > 5e-3 {
		t.Fatalf("quarter volume %.2f, want %.2f", v, want/4)
	}
}

func TestParamErrors(t *testing.T) {
	d := New()
	d.AddParam("a", "b + 1")
	d.AddParam("b", "a + 1")
	d.AddParam("c", "2 * 3")
	d.AddParam("2x", "1")
	d.Regenerate()
	if d.Params[0].Err == "" || d.Params[1].Err == "" {
		t.Fatal("cycle not reported")
	}
	if d.Params[2].Err != "" || d.Params[2].Value != 6 {
		t.Fatalf("c: %q %g", d.Params[2].Err, d.Params[2].Value)
	}
	if d.Params[3].Err == "" {
		t.Fatal("bad name accepted")
	}
	f := d.Insert(NewPrim(KBox))
	f.Inputs["Width"] = "a"
	d.Regenerate()
	if f.Err == "" {
		t.Fatal("box with an unresolved parameter built")
	}
}

func designVolume(t *testing.T, d *Document) float64 {
	t.Helper()
	v := 0.0
	for _, b := range d.Bodies {
		v += volume(t, b.Body)
	}
	return v
}

// A boss sketched on the plate's top face and joined to it; the plate's
// thickness is a parameter, and the joined body follows it.
func TestJoinBoss(t *testing.T) {
	d := New()
	d.AddParam("T", "10")
	plate := d.Insert(NewPrim(KBox))
	plate.Inputs["Height"] = "T" // 40 × 30 × T
	sk := d.Insert(NewSketch(PlaneRef{Feature: plate.ID, Face: "top"}))
	sk.Sketch.AddCircle(sk.Sketch.AddPoint(geom.V2(20, 15)).ID, 6)
	d.Regenerate()
	boss := d.Insert(NewExtrude([]ProfileRef{RefFor(sk.ID, sk.Regions[0])}))
	boss.Inputs["Distance"] = "8"
	boss.Inputs["Operation"] = "Join"
	d.Regenerate()
	if boss.Err != "" {
		t.Fatal(boss.Err)
	}
	if len(d.Bodies) != 1 {
		t.Fatalf("%d bodies after a join", len(d.Bodies))
	}
	want := 40*30*10 + math.Pi*36*8
	if v := designVolume(t, d); math.Abs(v-want)/want > 2e-3 {
		t.Fatalf("volume %.2f, want %.2f", v, want)
	}
	d.Params[0].Expr = "16"
	d.Regenerate()
	want = 40*30*16 + math.Pi*36*8
	if v := designVolume(t, d); boss.Err != "" || math.Abs(v-want)/want > 2e-3 {
		t.Fatalf("after T = 16: %q, volume %.2f, want %.2f", boss.Err, v, want)
	}
	// a sketch on the boss's top, which now belongs to the plate's body
	top := d.Insert(NewSketch(PlaneRef{Feature: plate.ID, Face: fmt.Sprintf("f%d/top", boss.ID)}))
	d.Regenerate()
	if top.Err != "" {
		t.Fatal(top.Err)
	}
	if z := top.Sketch.Plane.O.Z; math.Abs(z-24) > 1e-6 {
		t.Fatalf("sketch on the boss top at z %.3f, want 24", z)
	}
}

func TestCutAndIntersect(t *testing.T) {
	d := New()
	d.Insert(NewPrim(KBox)) // 40 × 30 × 20
	pin := d.Insert(NewPrim(KCylinder))
	pin.Inputs["X"], pin.Inputs["Y"], pin.Inputs["Z"] = "20", "15", "-5"
	pin.Inputs["Radius"], pin.Inputs["Height"] = "6", "30"
	pin.Inputs["Operation"] = "Cut"
	d.Regenerate()
	if pin.Err != "" {
		t.Fatal(pin.Err)
	}
	want := 24000 - math.Pi*36*20
	if v := designVolume(t, d); len(d.Bodies) != 1 || math.Abs(v-want)/want > 2e-3 {
		t.Fatalf("%d bodies, volume %.2f, want %.2f", len(d.Bodies), v, want)
	}
	pin.Inputs["Operation"] = "Intersect"
	d.Regenerate()
	want = math.Pi * 36 * 20
	if v := designVolume(t, d); pin.Err != "" || math.Abs(v-want)/want > 2e-3 {
		t.Fatalf("intersect: %q volume %.2f, want %.2f", pin.Err, v, want)
	}
	// a cut that takes everything leaves nothing
	big := d.Insert(NewPrim(KBox))
	big.Inputs["X"], big.Inputs["Y"], big.Inputs["Z"] = "-10", "-10", "-10"
	big.Inputs["Width"], big.Inputs["Depth"], big.Inputs["Height"] = "100", "100", "100"
	big.Inputs["Operation"] = "Cut"
	d.Regenerate()
	if len(d.Bodies) != 0 || big.Err != "" {
		t.Fatalf("after cutting everything: %d bodies, %q", len(d.Bodies), big.Err)
	}
}
