package doc

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"forma/geom"
)

// pinInPlate is the plate and the pin (100 mm off) with a joint of typ from
// the pin's bottom rim to the hole's top rim.
func pinInPlate(t *testing.T, typ string) (*Document, *Feature, *Feature, *Feature) {
	t.Helper()
	d := New()
	plate := d.Insert(NewComponent("C:/parts/plate.forma", plateDesign(t)))
	pin := d.Insert(NewComponent("C:/parts/pin.forma", pinDesign(t)))
	pin.Inputs["Move X"] = "100"
	d.Regenerate()
	mustOK(t, plate, pin)
	pk, plk := BodyKey(pin.ID, 0), BodyKey(plate.ID, 0)
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
	j := d.Insert(NewJoint(ja, jb, typ))
	d.Regenerate()
	mustOK(t, j)
	return d, plate, pin, j
}

// A grounded component stays where it is: the joint moves the other end,
// a move feature is refused, and two grounded ends are an error.
func TestGrounding(t *testing.T) {
	d, plate, pin, j := pinInPlate(t, "Revolute")
	pin.Inputs["Grounded"] = "1"
	d.Regenerate()
	mustOK(t, j)
	if c := bodyOf(d, BodyKey(pin.ID, 0)).Body.Bounds().Center(); c.Dist(geom.V3(100, 0, 15)) > 1e-9 {
		t.Fatalf("the grounded pin moved to %v", c)
	}
	// the plate's hole came to the pin instead
	pb := bodyOf(d, BodyKey(plate.ID, 0)).Body.Bounds().Center()
	if math.Hypot(pb.X-100, pb.Y) > 25+1e-6 || j.Moved != plate.ID {
		t.Fatalf("plate at %v, moved %d", pb, j.Moved)
	}
	plate.Inputs["Grounded"] = "1"
	d.Regenerate()
	if j.Err == "" || !strings.Contains(j.Err, "grounded") {
		t.Fatalf("both grounded: %q", j.Err)
	}
	plate.Inputs["Grounded"] = "0"
	mv := d.Insert(NewMove([]int{BodyKey(pin.ID, 0)}, geom.Vec3{}))
	mv.Inputs["Move Z"] = "5"
	d.Regenerate()
	if !strings.Contains(mv.Err, "grounded") {
		t.Fatalf("moving a grounded pin: %q", mv.Err)
	}
}

// A joint's motion is held within its limits.
func TestJointLimits(t *testing.T) {
	d, _, pin, j := pinInPlate(t, "Slider")
	z := func() float64 { return bodyOf(d, BodyKey(pin.ID, 0)).Body.Bounds().Min.Z }
	z0 := z()
	j.Inputs["Distance"] = "50"
	j.Inputs["Distance min"], j.Inputs["Distance max"] = "-5", "20"
	d.Regenerate()
	mustOK(t, j)
	if moved := math.Abs(z() - z0); math.Abs(moved-20) > 1e-6 {
		t.Fatalf("slid %.4f, the limit is 20", moved)
	}
	if !strings.Contains(j.Warn, "limit") {
		t.Fatalf("no note of the limit: %q", j.Warn)
	}
	j.Inputs["Distance"] = "10"
	d.Regenerate()
	if moved := math.Abs(z() - z0); math.Abs(moved-10) > 1e-6 || j.Warn != "" {
		t.Fatalf("slid %.4f (want 10), %q", moved, j.Warn)
	}
	if lo, hi, ok := d.Limits(j, "Distance"); !ok || lo != -5 || hi != 20 {
		t.Fatalf("limits %v %v %v", lo, hi, ok)
	}
	if _, _, ok := d.Limits(j, "Angle"); ok {
		t.Fatal("the angle has no limits")
	}
}

// The bill of materials counts copies of a component as one part and lists
// bodies made in the design itself.
func TestBOM(t *testing.T) {
	d, plate, pin, _ := pinInPlate(t, "Revolute")
	cp := d.Insert(NewInstance(pin))
	cp.Inputs["Move Y"] = "60"
	// the pin's file inserted again is the same part
	again := d.Insert(NewComponent("C:/parts/pin.forma", pinDesign(t)))
	again.Inputs["Move Y"] = "-60"
	b := d.Insert(NewPrim(KBox))
	b.Inputs["Width"], b.Inputs["Depth"], b.Inputs["Height"], b.Inputs["X"] = "10", "10", "10", "200"
	d.Regenerate()
	mustOK(t, cp, b)
	rows := d.BOM()
	if len(rows) != 3 {
		t.Fatalf("%d rows: %+v", len(rows), rows)
	}
	want := []struct {
		name string
		qty  int
		vol  float64
	}{{"plate", 1, 40*30*10 - math.Pi*25*10}, {"pin", 3, math.Pi * 25 * 30}, {"", 1, 1000}}
	for i, w := range want {
		r := rows[i]
		if (w.name != "" && r.Name != w.name) || r.Qty != w.qty || math.Abs(r.Volume-w.vol) > 2e-3*w.vol || r.Item != i+1 {
			t.Fatalf("row %d: %+v, want %s ×%d %.1f mm³", i, r, w.name, w.qty, w.vol)
		}
	}
	if rows[0].Comp != plate.ID || rows[2].Comp != 0 {
		t.Fatal("rows don't know their components")
	}
	var buf bytes.Buffer
	if err := WriteBOM(&buf, rows, 7.85); err != nil {
		t.Fatal(err)
	}
	csv := buf.String()
	if !strings.Contains(csv, "Mass (g)") || !strings.Contains(csv, "2,pin,pin.forma,3,1,") {
		t.Fatalf("csv:\n%s", csv)
	}
}

// Exploded, a pin joined into a grounded plate comes out along the hole's
// axis, away from the plate; an unjoined part moves out from the middle.
func TestExplode(t *testing.T) {
	d, plate, pin, _ := pinInPlate(t, "Revolute")
	plate.Inputs["Grounded"] = "1"
	loose := d.Insert(NewInstance(pin))
	loose.Inputs["Move X"] = "-60"
	d.Regenerate()
	if len(d.Explode(0)) != 0 {
		t.Fatal("assembled, nothing moves")
	}
	off := d.Explode(0.5)
	if off[plate.ID].Len() != 0 {
		t.Fatalf("the grounded plate moved %v", off[plate.ID])
	}
	o := off[pin.ID]
	if math.Hypot(o.X, o.Y) > 1e-9 || o.Len() < 1 {
		t.Fatalf("the pin moved %v, want along Z", o)
	}
	// away from the plate: the pin's middle is on the far side of z 5
	pc := bodyOf(d, BodyKey(pin.ID, 0)).Body.Bounds().Center()
	if (pc.Z-5)*o.Z <= 0 {
		t.Fatalf("the pin (z %.1f) moved %v toward the plate", pc.Z, o)
	}
	if l := off[loose.ID]; l.X >= 0 {
		t.Fatalf("the loose pin (at x −60) moved %v, not outward", l)
	}
}

// Bodies of the design's own (no components) come apart too.
func TestExplodeBodies(t *testing.T) {
	d := New()
	a := d.Insert(NewPrim(KBox))
	a.Inputs["Width"], a.Inputs["Depth"], a.Inputs["Height"] = "20", "20", "20"
	c := d.Insert(NewPrim(KCylinder))
	c.Inputs["X"], c.Inputs["Radius"], c.Inputs["Height"] = "40", "5", "20"
	d.Regenerate()
	mustOK(t, a, c)
	off := d.Explode(0.5)
	oa, oc := off[ExplodeGroup(0, BodyKey(a.ID, 0))], off[ExplodeGroup(0, BodyKey(c.ID, 0))]
	if oa.X >= 0 || oc.X <= 0 {
		t.Fatalf("box moved %v, cylinder %v: they should part along X", oa, oc)
	}
}
