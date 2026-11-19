package doc

import (
	"math"
	"testing"

	"forma/brep"
	"forma/geom"
)

// plateDesign is a 40 × 30 × 10 plate with a Ø10 hole through at (20, 15).
func plateDesign(t *testing.T) []byte {
	d := New()
	d.Insert(NewPrim(KBox)).Inputs = map[string]string{"Width": "40", "Depth": "30", "Height": "10", "X": "0", "Y": "0", "Z": "0"}
	c := d.Insert(NewPrim(KCylinder))
	c.Inputs = map[string]string{"Radius": "5", "Height": "14", "X": "20", "Y": "15", "Z": "-2", "Operation": "Cut"}
	d.Regenerate()
	mustOK(t, d.Features...)
	if len(d.Bodies) != 1 {
		t.Fatalf("plate: %d bodies", len(d.Bodies))
	}
	return d.Marshal()
}

// pinDesign is a Ø10 × 30 pin standing on the origin.
func pinDesign(t *testing.T) []byte {
	d := New()
	d.Insert(NewPrim(KCylinder)).Inputs = map[string]string{"Radius": "5", "Height": "30", "X": "0", "Y": "0", "Z": "0"}
	d.Regenerate()
	mustOK(t, d.Features...)
	return d.Marshal()
}

// circleEdge finds the edge of body key that is a circle of radius r round
// (x, y) at height z.
func circleEdge(t *testing.T, d *Document, key int, x, y, z, r float64) (*brep.Edge, geom.Vec3) {
	t.Helper()
	for _, b := range d.Bodies {
		if b.Key != key {
			continue
		}
		for _, e := range b.Body.Edges() {
			ok := true
			for i := 0; i <= 8; i++ {
				p := e.Curve.Eval(e.T0 + (e.T1-e.T0)*float64(i)/8)
				if math.Abs(p.Z-z) > 1e-6 || math.Abs(math.Hypot(p.X-x, p.Y-y)-r) > 5e-3 {
					ok = false
				}
			}
			if ok {
				return e, e.Curve.Eval(e.T0)
			}
		}
	}
	t.Fatalf("no circle r %g at (%g, %g, %g) on body %d", r, x, y, z, key)
	return nil, geom.Vec3{}
}

func bodyOf(d *Document, key int) *BodyRef {
	for _, b := range d.Bodies {
		if b.Key == key {
			return b
		}
	}
	return nil
}

func TestComponentsAndJoints(t *testing.T) {
	plateSrc, pinSrc := plateDesign(t), pinDesign(t)
	d := New()
	plate := d.Insert(NewComponent("C:/parts/plate.forma", plateSrc))
	pin := d.Insert(NewComponent("C:/parts/pin.forma", pinSrc))
	pin.Inputs["Move X"] = "100"
	d.Regenerate()
	mustOK(t, plate, pin)
	if plate.Name != "plate" || len(d.Bodies) != 2 {
		t.Fatalf("name %q, %d bodies", plate.Name, len(d.Bodies))
	}
	pk, plk := BodyKey(pin.ID, 0), BodyKey(plate.ID, 0)
	if c := bodyOf(d, pk).Body.Bounds().Center(); c.Dist(geom.V3(100, 0, 15)) > 1e-9 {
		t.Fatalf("pin placed at %v", c)
	}
	if d.ComponentOf(pk) != pin.ID {
		t.Fatal("the pin's body doesn't know its component")
	}

	// the pin's bottom rim into the hole's top rim, turning about it
	holeRim, hp := circleEdge(t, d, plk, 20, 15, 10, 5)
	pinRim, pp := circleEdge(t, d, pk, 100, 0, 0, 5)
	ja, err := d.JointRefFor(plk, nil, holeRim, nil, hp)
	if err != nil {
		t.Fatal(err)
	}
	jb, err := d.JointRefFor(pk, nil, pinRim, nil, pp)
	if err != nil {
		t.Fatal(err)
	}
	j := d.Insert(NewJoint(ja, jb, "Revolute"))
	d.Regenerate()
	mustOK(t, j)
	bb := bodyOf(d, pk).Body.Bounds()
	c := bb.Center()
	if math.Hypot(c.X-20, c.Y-15) > 1e-6 {
		t.Fatalf("pin centred at %v, the hole is at (20, 15)", c)
	}
	low, high := math.Abs(bb.Min.Z-10) < 1e-6, math.Abs(bb.Max.Z-10) < 1e-6
	if !low && !high {
		t.Fatalf("pin spans z %.4f…%.4f: its rim should sit on the hole's (z 10)", bb.Min.Z, bb.Max.Z)
	}
	// flipped, it goes the other way along the axis
	j.Inputs["Flip"] = "1"
	d.Regenerate()
	mustOK(t, j)
	fb := bodyOf(d, pk).Body.Bounds()
	if low == (math.Abs(fb.Min.Z-10) < 1e-6) {
		t.Fatalf("flipping left the pin at z %.4f…%.4f", fb.Min.Z, fb.Max.Z)
	}
	j.Inputs["Flip"] = "0"

	// the motion turns it about the hole's axis: the seam vertex goes round
	seam := func() geom.Vec3 {
		return bodyOf(d, pk).Body.Vertices()[0].P
	}
	d.Regenerate()
	v0 := seam()
	j.Inputs["Angle"] = "90"
	d.Regenerate()
	mustOK(t, j)
	v1 := seam()
	a0, a1 := math.Atan2(v0.Y-15, v0.X-20), math.Atan2(v1.Y-15, v1.X-20)
	turn := math.Abs(math.Remainder(a1-a0, 2*math.Pi)) * 180 / math.Pi
	if math.Abs(turn-90) > 1e-6 || math.Abs(v1.Z-v0.Z) > 1e-6 {
		t.Fatalf("turned %.4f° (want 90), z %.4f → %.4f", turn, v0.Z, v1.Z)
	}
	// its placement follows: the body is still the placed component
	r := bodyOf(d, pk)
	if r.Comp != pin.ID || r.Local == nil {
		t.Fatal("the joint lost the component")
	}
	for k, v := range r.Body.Vertices() {
		if r.Place.Pt(r.Local.Vertices()[k].P).Dist(v.P) > 1e-9 {
			t.Fatalf("vertex %d: placement doesn't map the local body onto the placed one", k)
		}
	}

	// a second plate, sharing the first one's design
	inst := d.Insert(NewInstance(plate))
	inst.Inputs["Move Y"] = "50"
	d.Regenerate()
	mustOK(t, inst)
	if inst.Data != "" || len(d.Bodies) != 3 {
		t.Fatalf("instance data %d bytes, %d bodies", len(inst.Data), len(d.Bodies))
	}
	vp := brep.Mass(bodyOf(d, plk).Body, 1e-3).Volume
	vi := brep.Mass(bodyOf(d, BodyKey(inst.ID, 0)).Body, 1e-3).Volume
	approx(t, "instance volume", vi, vp, 1e-9)
	approx(t, "plate volume", vp, 40*30*10-math.Pi*25*10, 1e-3)

	// it all comes back from a file
	d2, err := Unmarshal(d.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	d2.Regenerate()
	mustOK(t, d2.Features...)
	if got := bodyOf(d2, pk).Body.Bounds(); got.Min.Dist(r.Body.Bounds().Min) > 1e-9 {
		t.Fatalf("after loading the pin is at %v, was %v", got.Min, r.Body.Bounds().Min)
	}
}

// A joint on two flat faces sets one on the other, centre to centre.
func TestFaceJoint(t *testing.T) {
	d := New()
	plate := d.Insert(NewComponent("plate.forma", plateDesign(t)))
	pin := d.Insert(NewComponent("pin.forma", pinDesign(t)))
	pin.Inputs["Move X"] = "-80"
	pin.Inputs["Rotate X"] = "90"
	d.Regenerate()
	mustOK(t, plate, pin)
	top, bottom := "", ""
	for _, fc := range bodyOf(d, BodyKey(plate.ID, 0)).Body.Faces() {
		if p, ok := fc.Surf.(geom.Plane); ok && math.Abs(p.F.O.Z-10) < 1e-9 && math.Abs(p.F.Z.Z) > 0.99 {
			top = fc.Name
		}
	}
	for _, fc := range bodyOf(d, BodyKey(pin.ID, 0)).Body.Faces() {
		if fc.Name == "f2/f1/bottom" {
			bottom = fc.Name
		}
	}
	if top == "" || bottom == "" {
		t.Fatalf("faces %q %q", top, bottom)
	}
	j := d.Insert(NewJoint(JointRef{Feature: plate.ID, Key: BodyKey(plate.ID, 0), Face: top},
		JointRef{Feature: pin.ID, Key: BodyKey(pin.ID, 0), Face: bottom}, "Rigid"))
	j.Inputs["Distance"] = "3"
	d.Regenerate()
	mustOK(t, j)
	b := bodyOf(d, BodyKey(pin.ID, 0)).Body.Bounds()
	// standing on the top's centre (20, 15), lifted the offset
	if math.Hypot(b.Center().X-20, b.Center().Y-15) > 1e-6 || math.Abs(b.Min.Z-13) > 1e-6 || math.Abs(b.Max.Z-43) > 1e-6 {
		t.Fatalf("pin at %v … %v", b.Min, b.Max)
	}
}

// A pin too thick for its hole clashes with the plate; one beside it doesn't.
func TestInterference(t *testing.T) {
	d := New()
	plate := d.Insert(NewComponent("plate.forma", plateDesign(t)))
	fat := New()
	fat.Insert(NewPrim(KCylinder)).Inputs = map[string]string{"Radius": "6", "Height": "30", "X": "20", "Y": "15", "Z": "-10"}
	fat.Regenerate()
	pin := d.Insert(NewComponent("fat.forma", fat.Marshal()))
	other := d.Insert(NewInstance(pin))
	other.Inputs["Move X"] = "60"
	d.Regenerate()
	mustOK(t, plate, pin, other)
	all := func(*BodyRef) bool { return true }
	cl := d.Interference(all)
	if len(cl) != 1 || cl[0].Err != nil {
		t.Fatalf("%d clashes: %+v", len(cl), cl)
	}
	approx(t, "overlap", cl[0].Volume, math.Pi*(36-25)*10, 2e-3)

	// as an assembly for STEP: one part per design, an instance each
	a := d.StepAssembly("asm", all)
	if len(a.Parts) != 2 || len(a.Instances) != 3 || len(a.Bodies) != 0 {
		t.Fatalf("%d parts, %d instances, %d own bodies", len(a.Parts), len(a.Instances), len(a.Bodies))
	}
	if a.Instances[2].Part != a.Instances[1].Part || a.Instances[2].Place.T.X != 60 {
		t.Fatalf("the copy: part %d, at %v", a.Instances[2].Part, a.Instances[2].Place.T)
	}
}
