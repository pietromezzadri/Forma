package doc

import (
	"math"
	"testing"

	"forma/brep"
	"forma/geom"
)

func mustOK(t *testing.T, fs ...*Feature) {
	t.Helper()
	for _, f := range fs {
		if f.Err != "" {
			t.Fatalf("%s: %s", f.Name, f.Err)
		}
	}
}

func approx(t *testing.T, what string, got, want, rel float64) {
	t.Helper()
	if math.Abs(got-want) > rel*math.Abs(want) {
		t.Fatalf("%s: %.4f, want %.4f", what, got, want)
	}
}

// edgeRef finds the edge of the design nearest p and refers to it.
func edgeRef(t *testing.T, d *Document, p geom.Vec3) EdgeRef {
	t.Helper()
	var best *brep.Edge
	bd := math.Inf(1)
	for _, r := range d.Bodies {
		for _, e := range r.Body.Edges() {
			if dd := edgeDist(e, p); dd < bd {
				best, bd = e, dd
			}
		}
	}
	ref, ok := RefForEdge(best, p)
	if !ok {
		t.Fatal("no edge")
	}
	return ref
}

func TestMoveFeature(t *testing.T) {
	d := New()
	box := d.Insert(NewPrim(KBox)) // 40 × 30 × 20 at the origin
	d.Regenerate()
	mv := d.Insert(NewMove([]int{BodyKey(box.ID, 0)}, geom.V3(0, 0, 0)))
	mv.Inputs["Move X"] = "100"
	mv.Inputs["Rotate Z"] = "90"
	d.Regenerate()
	mustOK(t, mv)
	b := d.Bodies[0].Body.Bounds()
	// turned a quarter about Z: x ∈ [−30, 0], then moved 100
	if b.Min.Dist(geom.V3(70, 0, 0)) > 1e-9 || b.Max.Dist(geom.V3(100, 40, 20)) > 1e-9 {
		t.Fatalf("bounds %v", b)
	}
	// a sketch on the moved top face lies where it went
	sk := d.Insert(NewSketch(PlaneRef{Feature: box.ID, Face: "top"}))
	d.Regenerate()
	mustOK(t, sk)
	if math.Abs(sk.Sketch.Plane.O.Z-20) > 1e-9 {
		t.Fatalf("sketch at z %g", sk.Sketch.Plane.O.Z)
	}
}

func TestPatternAndMirror(t *testing.T) {
	d := New()
	d.Insert(NewPrim(KBox)) // 40 × 30 × 20
	pin := d.Insert(NewPrim(KCylinder))
	pin.Inputs["X"], pin.Inputs["Y"], pin.Inputs["Z"] = "6", "6", "-5"
	pin.Inputs["Radius"], pin.Inputs["Height"] = "3", "30"
	pin.Inputs["Operation"] = "Cut"
	d.Regenerate()
	mustOK(t, pin)
	// the hole patterned 3 along X by 2 along Y: six holes
	pat := d.Insert(NewPattern(nil, []int{pin.ID}, false))
	pat.Inputs["Count"], pat.Inputs["Spacing"] = "3", "14"
	pat.Inputs["Direction 2"], pat.Inputs["Count 2"], pat.Inputs["Spacing 2"] = "Y", "2", "18"
	d.Regenerate()
	mustOK(t, pat)
	hole := math.Pi * 9 * 20
	approx(t, "six holes", designVolume(t, d), 24000-6*hole, 2e-3)
	// mirrored across the YZ plane (the box's left face): one solid twice as long
	boss := d.Insert(NewPrim(KCylinder))
	boss.Inputs["X"], boss.Inputs["Y"], boss.Inputs["Z"] = "20", "15", "20"
	boss.Inputs["Radius"], boss.Inputs["Height"] = "5", "10"
	boss.Inputs["Operation"] = "Join"
	mir := d.Insert(NewMirror([]int{BodyKey(1, 0)}, nil))
	d.Regenerate()
	mustOK(t, boss, mir)
	if len(d.Bodies) != 1 {
		t.Fatalf("%d bodies", len(d.Bodies))
	}
	one := 24000 - 6*hole + math.Pi*25*10
	approx(t, "mirrored", designVolume(t, d), 2*one, 2e-3)
	if n := len(d.Bodies[0].Body.Shells); n != 1 {
		t.Fatalf("%d shells", n)
	}
	// across the right face, named: the same again the other way
	mir.Plane = &PlaneRef{Feature: 1, Face: "right"}
	d.Regenerate()
	mustOK(t, mir)
	approx(t, "mirrored about the right face", designVolume(t, d), 2*one, 2e-3)
	if b := d.Bodies[0].Body.Bounds(); math.Abs(b.Max.X-80) > 1e-6 {
		t.Fatalf("about the right face: reaches x %g", b.Max.X)
	}
	// circular: four bosses round the middle
	d2 := New()
	d2.Insert(NewPrim(KCylinder)) // r 12, h 30
	lug := d2.Insert(NewPrim(KBox))
	lug.Inputs["X"], lug.Inputs["Y"], lug.Inputs["Z"] = "10", "-3", "0"
	lug.Inputs["Width"], lug.Inputs["Depth"], lug.Inputs["Height"] = "10", "6", "10"
	lug.Inputs["Operation"] = "Join"
	cp := d2.Insert(NewPattern(nil, []int{lug.ID}, true))
	cp.Inputs["Count"] = "4"
	d2.Regenerate()
	mustOK(t, lug, cp)
	v1 := designVolume(t, d2)
	cp.Inputs["Count"] = "1"
	d2.Regenerate()
	v0 := designVolume(t, d2)
	// what one lug adds, by exact volumes: its box less the part in the cylinder
	lug1 := v0 - math.Pi*144*30
	approx(t, "four lugs", v1-v0, 3*lug1, 4e-2)
}

func TestFilletFeature(t *testing.T) {
	d := New()
	d.AddParam("R", "4")
	d.Insert(NewPrim(KBox)) // 40 × 30 × 20
	d.Regenerate()
	var refs []EdgeRef
	for _, p := range []geom.Vec3{{X: 20, Y: 0, Z: 20}, {X: 40, Y: 15, Z: 20}, {X: 20, Y: 30, Z: 20}, {X: 0, Y: 15, Z: 20}} {
		refs = append(refs, edgeRef(t, d, p))
	}
	fl := d.Insert(NewFillet(refs, false))
	fl.Inputs["Radius"] = "R"
	d.Regenerate()
	mustOK(t, fl)
	v4 := designVolume(t, d)
	a := 16 * (1 - math.Pi/4)
	if v4 > 24000-a*(140-4*8)+1 || v4 < 24000-a*140 {
		t.Fatalf("four top edges: %.2f", v4)
	}
	// a sketch on the rounded top: the top face keeps its name
	sk := d.Insert(NewSketch(PlaneRef{Feature: 1, Face: "top"}))
	d.Regenerate()
	mustOK(t, sk)
	// the radius changes: the fillet follows
	d.Params[0].Expr = "2"
	d.Regenerate()
	mustOK(t, fl)
	if v := designVolume(t, d); v <= v4 {
		t.Fatalf("R = 2 removes less: %.2f vs %.2f", v, v4)
	}
	// a chamfer on a vertical edge after the fillet
	ch := d.Insert(NewFillet([]EdgeRef{edgeRef(t, d, geom.V3(40, 0, 8))}, true))
	ch.Inputs["Distance"] = "3"
	d.Regenerate()
	mustOK(t, ch)
}

func TestHoleAndShell(t *testing.T) {
	d := New()
	plate := d.Insert(NewPrim(KBox)) // 40 × 30 × 20
	d.Regenerate()
	sk := d.Insert(NewSketch(PlaneRef{Feature: plate.ID, Face: "top"}))
	a := sk.Sketch.AddPoint(geom.V2(10, 15)).ID
	b := sk.Sketch.AddPoint(geom.V2(30, 15)).ID
	d.Regenerate()
	h := d.Insert(NewHole(sk.ID, []int{a, b}))
	h.Inputs["Extent"] = "Through all"
	d.Regenerate()
	mustOK(t, h)
	approx(t, "two holes", designVolume(t, d), 24000-2*math.Pi*9*20, 2e-3)
	h.Inputs["Type"] = "Counterbore"
	d.Regenerate()
	mustOK(t, h)
	approx(t, "counterbored", designVolume(t, d), 24000-2*(math.Pi*9*16+math.Pi*30.25*4), 2e-3)

	// a box hollowed with its top open
	d2 := New()
	box := d2.Insert(NewPrim(KBox))
	d2.Regenerate()
	sh := d2.Insert(NewShell(BodyKey(box.ID, 0), []FaceRef{{Feature: box.ID, Face: "f1/top"}}))
	sh.Inputs["Thickness"] = "2"
	d2.Regenerate()
	mustOK(t, sh)
	approx(t, "shell", designVolume(t, d2), 24000-36*26*18, 1e-6)
}

// Rounded corners first, then the top edge all round (straight edges and
// the arcs of the corners, a smooth chain).
func TestFilletChain(t *testing.T) {
	d := New()
	d.Insert(NewPrim(KBox)) // 40 × 30 × 20
	d.Regenerate()
	var vert []EdgeRef
	for _, p := range []geom.Vec3{{X: 40, Y: 0, Z: 10}, {X: 40, Y: 30, Z: 10}, {X: 0, Y: 30, Z: 10}, {X: 0, Y: 0, Z: 10}} {
		vert = append(vert, edgeRef(t, d, p))
	}
	f1 := d.Insert(NewFillet(vert, false))
	f1.Inputs["Radius"] = "6"
	d.Regenerate()
	mustOK(t, f1)
	v1 := designVolume(t, d)
	// every edge of the top face
	var top []EdgeRef
	for _, r := range d.Bodies {
		for _, e := range r.Body.Edges() {
			a, b := e.HE[0].Loop.Face.Name, e.HE[1].Loop.Face.Name
			if a == "f1/top" || b == "f1/top" {
				ref, _ := RefForEdge(e, e.Curve.Eval((e.T0+e.T1)/2))
				top = append(top, ref)
			}
		}
	}
	if len(top) != 8 {
		t.Fatalf("%d top edges", len(top))
	}
	f2 := d.Insert(NewFillet(top, false))
	f2.Inputs["Radius"] = "2"
	d.Regenerate()
	mustOK(t, f2)
	// a ring of spandrel swept along the top's outline: its straight runs and
	// its four arcs, turned about the corners' axes (Pappus)
	a, cx := 4*(1-math.Pi/4), 2*(10-3*math.Pi)/(3*(4-math.Pi))
	straight := 2*(40-12) + 2*(30-12)
	arcs := 2 * math.Pi * (6 - cx)
	want := v1 - a*(float64(straight)+arcs)
	approx(t, "rounded all round", designVolume(t, d), want, 2e-3)
}
