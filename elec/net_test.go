package elec

import (
	"math"
	"strings"
	"testing"

	"forma/avr"
)

// idle is a board running nothing (every pin an input, no pull-ups).
func idle() *avr.Uno { return avr.NewUno(prog(rjmp(-1))) }

func w(a, b string) Wire { return Wire{a, b} }

func build(t *testing.T, b *avr.Uno, specs []Spec, wires []Wire) (*Sim, map[string]Part) {
	t.Helper()
	s, parts, err := Build(b, specs, wires)
	if err != nil {
		t.Fatal(err)
	}
	return s, parts
}

func near(t *testing.T, what string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s: %.5g, want %.5g", what, got, want)
	}
}

// Two 10 kΩ resistors from 5 V to ground: A0 between them reads half,
// through the ADC too.
func TestNetDivider(t *testing.T) {
	uno := avr.NewUno(prog(
		stsReg(0x7C, 0x40), // ADMUX: AVcc, A0
		stsReg(0x7A, 0xC7), // start a conversion
		rjmp(-1),
	))
	s, _ := build(t, uno,
		[]Spec{{"resistor", "r1", map[string]float64{"Resistance": 10000}}, {"resistor", "r2", map[string]float64{"Resistance": 10000}}},
		[]Wire{w("uno.5V", "r1.1"), w("r1.2", "r2.1"), w("r2.1", "uno.A0"), w("r2.2", "uno.GND")})
	s.Run(0.01, 0.001)
	v, _ := s.Parts[0].(*Network).Volts(0)
	near(t, "A0", v, 2.5, 0.001)
	if got := int(uno.Data[0x78]) | int(uno.Data[0x79])<<8; got < 510 || got > 513 {
		t.Fatalf("ADC read %d, want 511 or 512", got)
	}
}

// An LED and its 220 Ω resistor across 5 V: (5 − 2) / 235 ≈ 12.8 mA.
func TestNetLEDOn5V(t *testing.T) {
	s, parts := build(t, idle(),
		[]Spec{{"resistor", "r", nil}, {"led", "led", nil}},
		[]Wire{w("uno.5V", "r.1"), w("r.2", "led.A"), w("led.K", "uno.GND")})
	s.Run(0.003, 0.001)
	l := parts["led"].(*LED)
	near(t, "current", l.Amps, 3.0/(220+ledOhms+railOhms), 1e-6)
	near(t, "brightness", l.Brightness, l.Amps/ledFull, 1e-9)
	if r := parts["r"].(*Resistor); math.Abs(r.Amps-l.Amps) > 1e-7 {
		t.Fatalf("the resistor carries %.4g A, the LED %.4g", r.Amps, l.Amps)
	}
}

// A battery lights an LED through a switch, nothing of the board in it:
// on, (6 − 2) / (220 + 15 + 0.6) A; off, nothing.
func TestNetBatterySwitch(t *testing.T) {
	s, parts := build(t, idle(),
		[]Spec{{"battery-4aa", "bat", nil}, {"switch", "sw", nil}, {"resistor", "r", nil}, {"led", "led", nil}},
		[]Wire{w("bat.+", "sw.1"), w("sw.2", "r.1"), w("r.2", "led.A"), w("led.K", "bat.-")})
	l := parts["led"].(*LED)
	s.Run(0.003, 0.001)
	if l.Amps != 0 {
		t.Fatalf("switched off, the LED draws %.4g A", l.Amps)
	}
	parts["sw"].(*Switch).On = true
	s.Run(0.003, 0.001)
	near(t, "on", l.Amps, 4.0/(220+0.6+ledOhms+closedOhms), 1e-6)
}

// A button from D2 to ground (D2's pull-up on), an LED and resistor on
// D13: the program lights D13 while D2 reads high.
func TestNetButtonLED(t *testing.T) {
	uno := avr.NewUno(prog(
		sbi(0x04, 5), // DDRB5 (D13) out
		sbi(0x0B, 2), // PORTD2: D2's pull-up
		sbis(0x09, 2), rjmp(2), sbi(0x05, 5), rjmp(-4),
		cbi(0x05, 5), rjmp(-6),
	))
	s, parts := build(t, uno,
		[]Spec{{"button", "btn", nil}, {"resistor", "r", nil}, {"led", "led", nil}},
		[]Wire{w("uno.D2", "btn.1"), w("btn.2", "uno.GND"), w("uno.D13", "r.1"), w("r.2", "led.A"), w("led.K", "uno.GND")})
	l, btn := parts["led"].(*LED), parts["btn"].(*Button)
	s.Run(0.01, 0.001)
	near(t, "released", l.Amps, 3.0/(pinOhms+220+ledOhms), 1e-5)
	btn.Pressed = true
	s.Run(0.01, 0.001)
	if l.Amps > 1e-9 {
		t.Fatalf("pressed, the LED draws %.4g A", l.Amps)
	}
}

// The pot_led sketch wired by wires: the pot between ground and 5 V, its
// wiper on A0; D3's PWM (the pot's share) lights its LED by its duty,
// mixed between the pin high and low (not by the average voltage, which
// would barely clear the LED's drop); the button on D2 lights D13.
func TestNetPWMLED(t *testing.T) {
	specs := []Spec{{"pot", "pot", map[string]float64{"Value": 0.5}}, {"button", "btn", nil}}
	wires := []Wire{w("pot.1", "uno.GND"), w("pot.2", "uno.5V"), w("pot.W", "uno.A0"), w("uno.D2", "btn.1"), w("btn.2", "uno.GND")}
	for _, pin := range []string{"D3", "D10", "D13"} {
		r, l := "r"+pin, "led"+pin
		specs = append(specs, Spec{"resistor", r, nil}, Spec{"led", l, nil})
		wires = append(wires, w("uno."+pin, r+".1"), w(r+".2", l+".A"), w(l+".K", "uno.GND"))
	}
	s, parts := build(t, loadSketch(t, "pot_led"), specs, wires)
	full := 3.0 / (pinOhms + 220 + ledOhms)
	s.Run(0.1, 0.01)
	d3, d10, d13 := parts["ledD3"].(*LED), parts["ledD10"].(*LED), parts["ledD13"].(*LED)
	near(t, "D3 at half", d3.Amps/full, 0.5, 0.03)
	near(t, "D10 at half", d10.Amps/full, 0.5, 0.03)
	if d13.Amps > 1e-9 {
		t.Fatalf("D13 lit with the button up: %s", s.Report())
	}
	parts["pot"].(*Pot).Value = 0.9
	parts["btn"].(*Button).Pressed = true
	s.Run(0.1, 0.01)
	near(t, "D3 at 0.9", d3.Amps/full, 0.9, 0.03)
	near(t, "D10 at 0.1", d10.Amps/full, 0.1, 0.03)
	near(t, "D13 pressed", d13.Amps/full, 1, 0.01)
}

// sweep_drive wired by wires behaves as wired by name: the arm sweeps
// 20°..160°, the pot sets the wheel's speed, the button reverses it; with
// its V+ unwired the servo has no power and stays where it is.
func TestNetSweepDrive(t *testing.T) {
	specs := []Spec{{"sg90", "arm", nil}, {"battery-2s", "bat", map[string]float64{"Volts": 6, "Resistance": 0}},
		{"tb6612", "driver", nil}, {"tt-motor", "wheel", nil}, {"pot", "speed", map[string]float64{"Value": 0.5}}, {"button", "reverse", nil}}
	wires := []Wire{
		w("arm.SIG", "uno.D9"), w("arm.V+", "uno.5V"), w("arm.GND", "uno.GND"),
		w("driver.IN1", "uno.D7"), w("driver.IN2", "uno.D8"), w("driver.EN", "uno.D6"),
		w("driver.VM", "bat.+"), w("driver.GND", "bat.-"), w("bat.-", "uno.GND"),
		w("driver.OUT1", "wheel.1"), w("driver.OUT2", "wheel.2"),
		w("speed.1", "uno.GND"), w("speed.2", "uno.5V"), w("speed.W", "uno.A0"),
		w("reverse.1", "uno.D2"), w("reverse.2", "uno.GND"),
	}
	s, parts := build(t, loadSketch(t, "sweep_drive"), specs, wires)
	servo, m := parts["arm"].(*Servo), parts["wheel"].(*Motor)
	lo, hi := 180.0, 0.0
	for i := 0; i < 3000; i++ {
		s.Step(0.001)
		if s.T > 0.5 {
			lo, hi = math.Min(lo, servo.Angle), math.Max(hi, servo.Angle)
		}
	}
	if lo > 22 || hi < 158 || lo < 18 || hi > 162 {
		t.Fatalf("the arm swept %.1f°..%.1f°, want 20..160 (%s)", lo, hi, s.Report())
	}
	half := 127.0 / 256 * 5.9 / 6 * 200
	near(t, "wheel rpm", m.RPM(), half, 0.05*half)
	parts["reverse"].(*Button).Pressed = true
	s.Run(1, 0.001)
	if m.RPM() > -0.9*half {
		t.Fatalf("reversed, the wheel is at %.1f rpm", m.RPM())
	}

	// the servo's V+ left off: no power
	var cut []Wire
	for _, x := range wires {
		if x.A != "arm.V+" {
			cut = append(cut, x)
		}
	}
	s, parts = build(t, loadSketch(t, "sweep_drive"), specs, cut)
	s.Run(1, 0.001)
	if sv := parts["arm"].(*Servo); sv.Angle != 90 || !strings.Contains(s.Report(), "no power") {
		t.Fatalf("unpowered, the servo is at %.1f° (%s)", sv.Angle, s.Report())
	}
}

// The checks: an LED with no resistor, a battery shorted, a motor on a
// pin, a terminal left unwired, a wire to a terminal that doesn't exist.
func TestNetCheck(t *testing.T) {
	has := func(issues []Issue, frag string, err bool) bool {
		for _, i := range issues {
			if strings.Contains(i.Text, frag) && i.Err == err {
				return true
			}
		}
		return false
	}
	is := Check([]Spec{{"led", "led", nil}}, []Wire{w("uno.D13", "led.A"), w("led.K", "uno.GND")})
	if !has(is, "no resistor", false) {
		t.Fatalf("LED straight on a pin: %+v", is)
	}
	is = Check([]Spec{{"battery-4aa", "bat", nil}}, []Wire{w("bat.+", "bat.-")})
	if !has(is, "short", true) {
		t.Fatalf("battery shorted: %+v", is)
	}
	is = Check([]Spec{{"tt-motor", "m", nil}}, []Wire{w("uno.D5", "m.1"), w("m.2", "uno.GND")})
	if !has(is, "motor driver", false) {
		t.Fatalf("motor on a pin: %+v", is)
	}
	is = Check([]Spec{{"resistor", "r", nil}}, []Wire{w("uno.5V", "r.1")})
	if !has(is, "r: 2 isn't wired", false) {
		t.Fatalf("an end left unwired: %+v", is)
	}
	if _, _, err := Build(idle(), []Spec{{"led", "led", nil}}, []Wire{w("led.X", "uno.GND")}); err == nil || !strings.Contains(err.Error(), "no terminal") {
		t.Fatalf("a wire to led.X: %v", err)
	}
	if is := Check([]Spec{{"resistor", "r", nil}, {"led", "led", nil}},
		[]Wire{w("uno.D13", "r.1"), w("r.2", "led.A"), w("led.K", "uno.GND")}); len(is) != 0 {
		t.Fatalf("a good LED circuit: %+v", is)
	}
}
