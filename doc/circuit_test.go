package doc

import (
	"math"
	"os"
	"strconv"
	"strings"
	"testing"

	"forma/elec"
)

// hornOnJoint is a pin turning in a plate's hole on a revolute joint.
func hornOnJoint(t *testing.T) (*Document, *Feature, func() float64) {
	t.Helper()
	d := New()
	plate := d.Insert(NewComponent("C:/parts/plate.forma", plateDesign(t)))
	pin := d.Insert(NewComponent("C:/parts/pin.forma", pinDesign(t)))
	pin.Inputs["Move X"] = "100"
	d.Regenerate()
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
	j := d.Insert(NewJoint(ja, jb, "Revolute"))
	d.Regenerate()
	mustOK(t, j)
	seam := func() float64 {
		v := bodyOf(d, pk).Body.Vertices()[0].P
		return math.Atan2(v.Y-15, v.X-20) * 180 / math.Pi
	}
	return d, j, seam
}

func firmware(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../avr/testdata/" + name + ".hex")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The servo sketch (45°, then 135°) turns an SG90 whose horn is the pin:
// the joint follows the servo less its middle.
func TestCircuitServoDrivesJoint(t *testing.T) {
	d, j, seam := hornOnJoint(t)
	c := d.Insert(NewCircuit())
	c.Data = firmware(t, "servo")
	c.Circuit.File = "servo.hex"
	sp := NewCircuitPart(c.Circuit, "sg90", c.Circuit.PartName("sg90"))
	if sp.Pins["signal"] != "D9" || sp.Zero != 90 {
		t.Fatalf("a new servo: %+v", sp)
	}
	sp.Joint = j.ID
	c.Circuit.Parts = append(c.Circuit.Parts, sp)
	d.Regenerate()
	mustOK(t, d.Features...)
	if c.Name != "Electronics 1" && !strings.HasPrefix(c.Name, "Electronics") {
		t.Fatalf("named %q", c.Name)
	}
	a0 := seam()

	st, err := d.NewStudy(c)
	if err != nil {
		t.Fatal(err)
	}
	st.Run(0.45)
	if !st.Pose(d) {
		t.Fatal("the joint didn't move")
	}
	d.Regenerate()
	mustOK(t, d.Features...)
	if v, _ := strconv.ParseFloat(j.Inputs["Angle"], 64); math.Abs(v+45) > 1 {
		t.Fatalf("at 0.45 s the joint is at %s°, want −45", j.Inputs["Angle"])
	}
	turn := math.Remainder(seam()-a0, 360)
	if math.Abs(math.Abs(turn)-45) > 1 {
		t.Fatalf("the pin turned %.2f°, want 45", turn)
	}
	st.Run(0.5)
	st.Pose(d)
	d.Regenerate()
	if v, _ := strconv.ParseFloat(j.Inputs["Angle"], 64); math.Abs(v-45) > 1 {
		t.Fatalf("at 0.95 s the joint is at %s°, want 45", j.Inputs["Angle"])
	}
	st.Restore(d)
	if j.Inputs["Angle"] != "0" {
		t.Fatalf("restored to %q", j.Inputs["Angle"])
	}

	// the circuit comes back from a file
	d2, err := Unmarshal(d.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	d2.Regenerate()
	mustOK(t, d2.Features...)
	c2 := d2.Feature(c.ID)
	if c2.Circuit == nil || len(c2.Circuit.Parts) != 1 || c2.Data != c.Data {
		t.Fatal("the circuit didn't round-trip")
	}
	if _, err := d2.NewStudy(c2); err != nil {
		t.Fatal(err)
	}
	if deps := d.Dependents(j.ID); len(deps) != 1 || deps[0] != c {
		t.Fatalf("the joint's dependents: %v", deps)
	}
}

// A TT motor behind a TB6612 turns the pin round and round (the motor
// sketch: half speed forward for a second): the joint's angle is the
// shaft's, wrapped, times the ratio.
func TestCircuitMotorDrivesJoint(t *testing.T) {
	d, j, _ := hornOnJoint(t)
	c := d.Insert(NewCircuit())
	c.Data = firmware(t, "motor")
	ci := c.Circuit
	ci.Parts = append(ci.Parts, NewCircuitPart(ci, "battery-4aa", "battery"))
	m := NewCircuitPart(ci, "tt-motor", "motor")
	m.Joint, m.Ratio = j.ID, 0.5 // a 2:1 belt
	ci.Parts = append(ci.Parts, m)
	drv := NewCircuitPart(ci, "tb6612", "driver")
	if drv.Supply != "battery" || drv.Motor != "motor" {
		t.Fatalf("a new driver: %+v", drv)
	}
	drv.Pins = map[string]string{"in1": "D7", "in2": "D8", "enable": "D6"}
	ci.Parts = append(ci.Parts, drv)
	d.Regenerate()
	mustOK(t, d.Features...)
	st, err := d.NewStudy(c)
	if err != nil {
		t.Fatal(err)
	}
	st.Run(0.9)
	st.Pose(d)
	d.Regenerate()
	mustOK(t, d.Features...)
	mot := st.Parts["motor"].(*elec.Motor)
	want := math.Remainder(mot.Angle*180/math.Pi*0.5, 360)
	got, _ := strconv.ParseFloat(j.Inputs["Angle"], 64)
	if math.Abs(got-want) > 0.01 || mot.Angle < 2*math.Pi {
		t.Fatalf("joint %g°, want %g° (shaft %.1f°)", got, want, mot.Angle*180/math.Pi)
	}
}

// Wiring mistakes are the feature's error.
func TestCircuitErrors(t *testing.T) {
	d, j, _ := hornOnJoint(t)
	c := d.Insert(NewCircuit())
	d.Regenerate()
	if c.Err != "" || !strings.Contains(c.Warn, "no firmware") {
		t.Fatalf("empty circuit: err %q, warn %q", c.Err, c.Warn)
	}
	c.Data = firmware(t, "servo")
	for _, tc := range []struct {
		part CircuitPart
		want string
	}{
		{CircuitPart{Model: "sg90", Name: "s", Pins: map[string]string{"signal": "D14"}}, "no pin"},
		{CircuitPart{Model: "pot", Name: "p", Pins: map[string]string{"wiper": "D3"}}, "not an analogue"},
		{CircuitPart{Model: "sg90", Name: "s", Pins: map[string]string{"signal": "D9"}, Joint: 999}, "gone"},
		{CircuitPart{Model: "led", Name: "l", Pins: map[string]string{"pin": "D9"}, Joint: j.ID}, "no shaft"},
		{CircuitPart{Model: "warp-drive", Name: "w"}, "library"},
	} {
		c.Circuit.Parts = []CircuitPart{tc.part}
		d.Regenerate()
		if !strings.Contains(c.Err, tc.want) {
			t.Errorf("%s: error %q, want %q", tc.part.Model, c.Err, tc.want)
		}
	}
	c.Circuit.Parts = []CircuitPart{
		{Model: "sg90", Name: "a", Pins: map[string]string{"signal": "D9"}},
		{Model: "led", Name: "b", Pins: map[string]string{"pin": "9"}},
	}
	d.Regenerate()
	if !strings.Contains(c.Err, "both on D9") {
		t.Errorf("two parts on a pin: %q", c.Err)
	}
	// a rigid joint can't be driven
	j.Inputs["Type"] = "Rigid"
	c.Circuit.Parts = []CircuitPart{{Model: "sg90", Name: "a", Pins: map[string]string{"signal": "D9"}, Joint: j.ID}}
	d.Regenerate()
	if !strings.Contains(c.Err, "rigid") {
		t.Errorf("rigid joint: %q", c.Err)
	}
}
