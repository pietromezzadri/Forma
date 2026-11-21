package elec

import (
	"fmt"
	"math"

	"forma/avr"
)

// NoPin marks an input left unconnected (a driver's enable tied high).
const NoPin avr.Pin = -1

// Battery is a supply: its voltage, sagging over its internal resistance
// with the current drawn last step.
type Battery struct {
	Name  string
	Volts float64
	R     float64 // internal resistance, Ω
	draw  float64 // amps drawn last step
	loads []func() float64
}

func (b *Battery) Label() string { return b.Name }
func (b *Battery) Wire(*Sim)     {}
func (b *Battery) Step(*Sim, float64) {
	b.draw = 0
	for _, l := range b.loads {
		b.draw += l()
	}
}

// Out is the voltage at its terminals.
func (b *Battery) Out() float64 { return math.Max(0, b.Volts-b.R*b.draw) }

func (b *Battery) State() string { return fmt.Sprintf("%.2f V, %.2f A", b.Out(), b.draw) }

// Driver is an H-bridge (an L298N or a TB6612 channel): IN1 / IN2 choose
// the direction, EN (or PWM on an IN) the share of the supply that
// reaches the motor.
type Driver struct {
	Name     string
	In1, In2 avr.Pin
	En       avr.Pin // NoPin: tied high
	Supply   *Battery
	Drop     float64 // volts lost in the bridge (L298N ~1.4, TB6612 ~0.1)
	Motor    *Motor
	out      float64
}

func (d *Driver) Label() string { return d.Name }

func (d *Driver) Wire(s *Sim) {
	s.Watch(d.In1)
	s.Watch(d.In2)
	if d.En != NoPin {
		s.Watch(d.En)
	}
	if d.Motor != nil && d.Supply != nil {
		d.Supply.loads = append(d.Supply.loads, func() float64 { return math.Abs(d.Motor.Current()) })
	}
}

func (d *Driver) Step(s *Sim, dt float64) {
	en := 1.0
	if d.En != NoPin {
		en = s.Duty(d.En)
	}
	v := 0.0
	if d.Supply != nil {
		v = math.Max(0, d.Supply.Out()-d.Drop)
	}
	d.out = (s.Duty(d.In1) - s.Duty(d.In2)) * en * v
	if d.Motor != nil {
		d.Motor.Volts = d.out
	}
}

// Out is the mean voltage across the motor last step.
func (d *Driver) Out() float64 { return d.out }

func (d *Driver) State() string { return fmt.Sprintf("%.2f V", d.out) }

// Motor is a DC gear motor, its figures at the output shaft: the rated
// voltage, the no-load speed and the stall torque there. With back-EMF and
// torque constants equal (SI), they give its winding's resistance; the
// speed follows the voltage and the load torque through the rotor's (and
// load's) inertia — solved exactly over each step.
type Motor struct {
	Name        string
	Rated       float64 // V
	NoLoadRPM   float64 // at the rated voltage
	StallTorque float64 // N·m at the rated voltage
	Inertia     float64 // kg·m² at the output shaft (with what it drives)
	Load        float64 // N·m resisting it (friction of what it drives)

	Volts float64 // set by its driver
	Omega float64 // rad/s
	Angle float64 // rad, from the start
	amps  float64
}

func (m *Motor) Label() string { return m.Name }
func (m *Motor) Wire(*Sim)     {}

// constants: k (V·s/rad = N·m/A) and R (Ω).
func (m *Motor) constants() (float64, float64) {
	w0 := m.NoLoadRPM * 2 * math.Pi / 60
	k := m.Rated / w0
	r := k * m.Rated / m.StallTorque
	return k, r
}

func (m *Motor) Step(_ *Sim, dt float64) {
	k, r := m.constants()
	j := m.Inertia
	if j <= 0 {
		j = 2e-5
	}
	// J ω' = k (V − k ω) / R − load: linear in ω, solved exactly
	load := math.Copysign(m.Load, m.Volts)
	if math.Abs(m.Volts) < 1e-9 {
		load = math.Copysign(m.Load, m.Omega)
	}
	a := (k*m.Volts/r - load) / j
	b := k * k / (r * j)
	inf := a / b
	e := math.Exp(-b * dt)
	w := inf + (m.Omega-inf)*e
	// friction stops it, doesn't turn it back
	if m.Load > 0 && math.Abs(k*m.Volts/r) < m.Load && w*m.Omega <= 0 {
		w = 0
	}
	m.Angle += inf*dt + (m.Omega-inf)*(1-e)/b
	m.Omega = w
	m.amps = (m.Volts - k*m.Omega) / r
}

// Current is what it draws (A).
func (m *Motor) Current() float64 { return m.amps }

// RPM is its speed.
func (m *Motor) RPM() float64 { return m.Omega * 60 / (2 * math.Pi) }

func (m *Motor) State() string {
	return fmt.Sprintf("%.0f rpm, %.0f°, %.2f A", m.RPM(), m.Angle*180/math.Pi, m.amps)
}

// TTMotor is the common yellow 1:48 gear motor of hobby cars: about
// 200 rpm and 0.08 N·m at 6 V.
func TTMotor(name string) *Motor {
	return &Motor{Name: name, Rated: 6, NoLoadRPM: 200, StallTorque: 0.078, Inertia: 2e-5}
}

// Servo is a hobby servo on a signal pin: the width of the last pulse
// (544–2400 µs, the Arduino Servo library's range) is the angle it turns
// to, at its speed.
type Servo struct {
	Name         string
	Signal       avr.Pin
	MinUs, MaxUs float64 // pulse widths for 0 and Range degrees
	Range        float64 // degrees
	Speed        float64 // degrees a second
	Angle        float64 // degrees now
	Target       float64
	attached     bool
}

// SG90 is the small blue micro servo: 180°, 0.1 s for 60° at 4.8 V.
func SG90(name string, signal avr.Pin) *Servo {
	return &Servo{Name: name, Signal: signal, MinUs: 544, MaxUs: 2400, Range: 180, Speed: 600, Angle: 90, Target: 90}
}

func (v *Servo) Label() string { return v.Name }
func (v *Servo) Wire(s *Sim)   { s.Watch(v.Signal) }

func (v *Servo) Step(s *Sim, dt float64) {
	ps := s.Pulses(v.Signal)
	for i := len(ps) - 1; i >= 0; i-- {
		if w := ps[i]; w > 300 && w < 3000 {
			t := (w - v.MinUs) / (v.MaxUs - v.MinUs) * v.Range
			v.Target = math.Max(0, math.Min(v.Range, t))
			v.attached = true
			break
		}
	}
	step := v.Speed * dt
	d := v.Target - v.Angle
	if math.Abs(d) <= step {
		v.Angle = v.Target
	} else {
		v.Angle += math.Copysign(step, d)
	}
}

func (v *Servo) State() string {
	if !v.attached {
		return "no signal"
	}
	return fmt.Sprintf("%.1f°", v.Angle)
}

// LED is a light on a pin: its brightness the pin's duty.
type LED struct {
	Name       string
	Pin        avr.Pin
	Brightness float64
}

func (l *LED) Label() string { return l.Name }
func (l *LED) Wire(s *Sim)   { s.Watch(l.Pin) }
func (l *LED) Step(s *Sim, _ float64) {
	l.Brightness = s.Duty(l.Pin)
}
func (l *LED) State() string { return fmt.Sprintf("%.0f %%", l.Brightness*100) }

// Button is a push button between a pin and ground (the sketch turns on
// the pin's pull-up: pressed reads LOW).
type Button struct {
	Name    string
	Pin     avr.Pin
	Pressed bool
	was     bool
	wired   bool
}

func (b *Button) Label() string { return b.Name }
func (b *Button) Wire(s *Sim)   { b.set(s) }
func (b *Button) Step(s *Sim, _ float64) {
	if b.Pressed != b.was || !b.wired {
		b.set(s)
	}
}

func (b *Button) set(s *Sim) {
	if b.Pressed {
		s.Board.Drive(b.Pin, false)
	} else {
		s.Board.Release(b.Pin)
	}
	b.was, b.wired = b.Pressed, true
}

// Pot is a potentiometer across 5 V with its wiper on an analogue input.
type Pot struct {
	Name    string
	Channel int     // A0..A5
	Value   float64 // 0..1 of its turn
}

func (p *Pot) Label() string      { return p.Name }
func (p *Pot) Wire(*Sim)          {}
func (p *Pot) Step(*Sim, float64) {}
func (p *Pot) Volts(ch int) (float64, bool) {
	if ch != p.Channel {
		return 0, false
	}
	return 5 * math.Max(0, math.Min(1, p.Value)), true
}
