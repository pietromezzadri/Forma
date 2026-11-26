package doc

import (
	"slices"
	"strings"
	"testing"
)

// A circuit wired the old way (pins by role, a driver's supply and motor
// by name) becomes wires: each LED with a resistor of its own, the
// supplies and grounds its parts need; it then checks clean, survives a
// save, and the helpers move a pin, a supply and a name.
func TestCircuitWiresFromPins(t *testing.T) {
	d := New()
	c := d.Insert(NewCircuit())
	c.Data = firmware(t, "sweep_drive")
	c.Circuit.Parts = []CircuitPart{
		{Model: "sg90", Name: "arm", Pins: map[string]string{"signal": "D9"}},
		{Model: "battery-2s", Name: "bat"},
		{Model: "tt-motor", Name: "wheel"},
		{Model: "tb6612", Name: "drv", Pins: map[string]string{"in1": "D7", "in2": "D8", "enable": "D6"}, Supply: "bat", Motor: "wheel"},
		{Model: "pot", Name: "speed", Pins: map[string]string{"wiper": "A0"}},
		{Model: "button", Name: "rev", Pins: map[string]string{"pin": "2"}},
		{Model: "led", Name: "lamp", Pins: map[string]string{"pin": "D13"}},
	}
	d.Regenerate()
	if c.Err != "" || c.Warn != "" {
		t.Fatalf("migrated: err %q, warn %q", c.Err, c.Warn)
	}
	cc := c.Circuit
	for _, p := range cc.Parts {
		if len(p.Pins) > 0 || p.Supply != "" || p.Motor != "" {
			t.Fatalf("%s still wired the old way: %+v", p.Name, p)
		}
	}
	has := func(a, b string) bool {
		return slices.Contains(cc.Net(a), b)
	}
	for _, pair := range [][2]string{
		{"uno.D9", "arm.SIG"}, {"arm.V+", "uno.5V"}, {"arm.GND", "uno.GND"},
		{"uno.D7", "drv.IN1"}, {"uno.D8", "drv.IN2"}, {"uno.D6", "drv.EN"},
		{"drv.VM", "bat.+"}, {"bat.-", "uno.GND"}, {"drv.OUT1", "wheel.1"}, {"drv.OUT2", "wheel.2"},
		{"uno.A0", "speed.W"}, {"speed.1", "uno.GND"}, {"speed.2", "uno.5V"},
		{"uno.D2", "rev.1"}, {"rev.2", "uno.GND"},
		{"uno.D13", "lamp resistor.1"}, {"lamp resistor.2", "lamp.A"}, {"lamp.K", "uno.GND"},
	} {
		if !has(pair[0], pair[1]) {
			t.Errorf("%s isn't wired to %s", pair[0], pair[1])
		}
	}
	// a study runs it as before
	st, err := d.NewStudy(c)
	if err != nil {
		t.Fatal(err)
	}
	st.Run(1)
	if !strings.HasPrefix(string(st.Sim.Serial), "sweep and drive") {
		t.Fatalf("serial %q", st.Sim.Serial)
	}
	// saved and opened again
	d2, err := Unmarshal(d.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	if got := len(d2.Feature(c.ID).Circuit.Wires); got != len(cc.Wires) {
		t.Fatalf("reopened with %d wires, want %d", got, len(cc.Wires))
	}
	// the helpers
	if p := cc.BoardPin("arm", "SIG"); p != "D9" {
		t.Fatalf("arm.SIG on %q", p)
	}
	cc.SetBoardPin("arm", "SIG", "D10")
	if p := cc.BoardPin("arm", "SIG"); p != "D10" || has("uno.D9", "arm.SIG") {
		t.Fatalf("moved to %q (D9 still: %v)", p, has("uno.D9", "arm.SIG"))
	}
	if b := cc.Linked("drv", "VM", "battery", "+"); b != "bat" {
		t.Fatalf("the driver's supply %q", b)
	}
	cc.SetSupply("drv", "")
	if b := cc.Linked("drv", "VM", "battery", "+"); b != "" {
		t.Fatalf("supply taken off, still %q", b)
	}
	cc.RenamePart("lamp", "light")
	if !has("light.K", "uno.GND") || has("lamp.K", "uno.GND") {
		t.Fatal("the rename didn't move the wires")
	}
	n := len(cc.Wires)
	cc.RemovePart("light")
	if len(cc.Wires) != n-2 {
		t.Fatalf("removing the LED left %d of %d wires", len(cc.Wires), n)
	}
	// a part added now comes wired: a second LED with its resistor on a
	// free pin
	p := cc.AddPart("led", "")
	if p == nil || cc.BoardPin(p.Name+" resistor", "1") == "" || !has(p.Name+".K", "uno.GND") {
		t.Fatalf("a new LED: %+v, wires %+v", p, cc.Wires)
	}
}

// An LED added with its 3D model once had the model linked to its resistor
// (added after it), so the model never lit: building gives it back.
func TestCircuitModelBackToLED(t *testing.T) {
	c := &Circuit{Board: "uno", Parts: []CircuitPart{
		{Model: "led", Name: "LED"},
		{Model: "resistor", Name: "LED resistor", Model3D: 2},
		{Model: "button", Name: "Push button", Model3D: 3},
	}}
	c.migrate()
	if c.Parts[0].Model3D != 2 || c.Parts[1].Model3D != 0 || c.Parts[2].Model3D != 3 {
		t.Fatalf("models: LED %d, resistor %d, button %d", c.Parts[0].Model3D, c.Parts[1].Model3D, c.Parts[2].Model3D)
	}
	// adding a part now links nothing to the resistor
	c2 := &Circuit{Board: "uno"}
	p := c2.AddPart("led", "")
	if p == nil || p.Model != "led" {
		t.Fatalf("AddPart gave %+v", p)
	}
}
