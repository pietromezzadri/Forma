package elec

import (
	"math"
	"strings"
	"testing"

	"forma/avr"
)

// Compiled sketches from ../avr/testdata (arduino-cli, arduino:avr 1.8.8).

func loadSketch(t *testing.T, name string) *avr.Uno {
	t.Helper()
	u, err := avr.LoadUno("../avr/testdata/" + name + ".hex")
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// The Servo library (timer 1's compare interrupts toggling D9) moves an
// SG90 to 45°, then 135°.
func TestSketchServo(t *testing.T) {
	servo := SG90("servo", avr.D(9))
	s := New(loadSketch(t, "servo"), servo)
	s.Run(0.45, 0.001)
	if math.Abs(servo.Angle-45) > 1 {
		t.Fatalf("at 0.45 s the servo is at %.1f°, want 45 (%s)", servo.Angle, s.Report())
	}
	s.Run(0.5, 0.001)
	if math.Abs(servo.Angle-135) > 1 {
		t.Fatalf("at 0.95 s the servo is at %.1f°, want 135 (%s)", servo.Angle, s.Report())
	}
}

// analogWrite(6, 128) on a driver's enable: half speed forward; then full
// reverse.
func TestSketchMotor(t *testing.T) {
	bat := &Battery{Name: "battery", Volts: 6}
	m := TTMotor("motor")
	drv := &Driver{Name: "TB6612", In1: avr.D(7), In2: avr.D(8), En: avr.D(6), Supply: bat, Drop: 0.1, Motor: m}
	s := New(loadSketch(t, "motor"), bat, drv, m)
	s.Run(0.9, 0.001)
	half := 129.0 / 256 * 5.9 / 6 * 200
	if math.Abs(m.RPM()-half) > 0.03*half {
		t.Fatalf("forward %.1f rpm, want about %.1f", m.RPM(), half)
	}
	s.Run(1, 0.001)
	full := -5.9 / 6 * 200
	if math.Abs(m.RPM()-full) > 0.03*math.Abs(full) {
		t.Fatalf("reverse %.1f rpm, want about %.1f", m.RPM(), full)
	}
}

// analogRead(A0) → analogWrite on D3 (timer 2) and D10 (timer 1, both
// phase-correct PWM); a button on D2 lights D13.
func TestSketchPotLED(t *testing.T) {
	pot := &Pot{Name: "pot", Channel: 0, Value: 0.5}
	a := &LED{Name: "D3", Pin: avr.D(3)}
	b := &LED{Name: "D10", Pin: avr.D(10)}
	btn := &Button{Name: "button", Pin: avr.D(2)}
	led := &LED{Name: "D13", Pin: avr.D(13)}
	s := New(loadSketch(t, "pot_led"), pot, a, b, btn, led)
	s.Run(0.1, 0.01)
	if math.Abs(a.Brightness-0.5) > 0.02 || math.Abs(b.Brightness-0.5) > 0.02 || led.Brightness > 0.01 {
		t.Fatalf("pot at half: %s", s.Report())
	}
	pot.Value = 0.9
	btn.Pressed = true
	s.Run(0.1, 0.01)
	if math.Abs(a.Brightness-0.9) > 0.02 || math.Abs(b.Brightness-0.1) > 0.02 || led.Brightness < 0.99 {
		t.Fatalf("pot at 0.9, button pressed: %s", s.Report())
	}
}

// sweep_drive: the arm sweeps 2° every 15 ms between 20° and 160°; the pot
// sets the wheel's speed and the button reverses it.
func TestSketchSweepDrive(t *testing.T) {
	servo := SG90("arm", avr.D(9))
	bat := &Battery{Name: "battery", Volts: 6}
	m := TTMotor("wheel")
	drv := &Driver{Name: "driver", In1: avr.D(7), In2: avr.D(8), En: avr.D(6), Supply: bat, Drop: 0.1, Motor: m}
	pot := &Pot{Name: "speed", Channel: 0, Value: 0.5}
	btn := &Button{Name: "reverse", Pin: avr.D(2)}
	s := New(loadSketch(t, "sweep_drive"), servo, bat, drv, m, pot, btn)
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
	if math.Abs(m.RPM()-half) > 0.05*half {
		t.Fatalf("wheel at %.1f rpm, want about %.1f", m.RPM(), half)
	}
	btn.Pressed = true
	s.Run(1, 0.001)
	if m.RPM() > -0.9*half {
		t.Fatalf("reversed, the wheel is at %.1f rpm", m.RPM())
	}
	if !strings.HasPrefix(string(s.Serial), "sweep and drive\r\narm ") {
		t.Fatalf("serial %q", s.Serial)
	}
}
