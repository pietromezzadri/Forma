// Package elec runs an Arduino sketch against the parts it drives: an
// emulated Uno (package avr) wired to simple lumped models — a battery, a
// motor driver, DC gear motors, hobby servos, LEDs, buttons and
// potentiometers — stepped together in time. What comes out (a servo's
// angle, a motor's shaft angle and speed) is what drives an assembly's
// joints in a motion study.
//
// The models are averages, not circuits: a motor sees the mean voltage of
// its driver's PWM over a step, a servo the width of the last pulse on its
// signal line. That is how such parts behave at the scale of a mechanism
// (a PWM period is a millisecond, a motor's time constant tens of them).
package elec

import (
	"fmt"
	"math"

	"forma/avr"
)

// Sim is a board and its parts, stepped together.
type Sim struct {
	Board *avr.Uno
	Parts []Part
	T     float64 // seconds since the start
	// Serial is what the sketch printed.
	Serial []byte

	meters map[avr.Pin]*meter
	start  uint64 // the step's first cycle
}

// Part is something wired to the board.
type Part interface {
	// Label names it ("left motor", "steering servo").
	Label() string
	// Wire is called once, when the part joins the simulation.
	Wire(s *Sim)
	// Step moves it on by dt seconds, the board having run them.
	Step(s *Sim, dt float64)
}

// New makes a simulation of a board running a sketch and its parts.
func New(b *avr.Uno, parts ...Part) *Sim {
	s := &Sim{Board: b, meters: map[avr.Pin]*meter{}}
	b.OnPin = func(p avr.Pin, high bool, cyc uint64) {
		if m := s.meters[p]; m != nil {
			m.edge(high, cyc)
		}
	}
	b.USART.OnTX = func(c byte) { s.Serial = append(s.Serial, c) }
	b.ADC.Volts = s.volts
	for _, p := range parts {
		s.Add(p)
	}
	return s
}

// Add wires another part in.
func (s *Sim) Add(p Part) {
	s.Parts = append(s.Parts, p)
	p.Wire(s)
}

// Step runs the board and the parts for dt seconds.
func (s *Sim) Step(dt float64) {
	s.start = s.Board.Cycles
	for _, m := range s.meters {
		m.begin(s.start)
	}
	s.Board.RunCycles(uint64(dt * avr.ClockHz))
	s.Board.Sync()
	for _, p := range s.Parts {
		p.Step(s, dt)
	}
	s.T += dt
}

// Run steps the simulation for a span of time in steps of dt.
func (s *Sim) Run(seconds, dt float64) {
	for t := 0.0; t < seconds-1e-12; t += dt {
		s.Step(math.Min(dt, seconds-t))
	}
}

// Watch makes sure a pin's level is metered (parts call it as they wire).
func (s *Sim) Watch(p avr.Pin) {
	if s.meters[p] == nil {
		s.meters[p] = &meter{high: s.Board.Level(p), since: s.Board.Cycles, start: s.Board.Cycles}
	}
}

// Duty is the share of the last step a pin was high (0..1).
func (s *Sim) Duty(p avr.Pin) float64 {
	m := s.meters[p]
	if m == nil {
		if s.Board.Level(p) {
			return 1
		}
		return 0
	}
	return m.duty(s.Board.Cycles)
}

// Pulses are the widths (µs) of the high pulses that ended on a pin in
// the last step.
func (s *Sim) Pulses(p avr.Pin) []float64 {
	if m := s.meters[p]; m != nil {
		return m.pulses
	}
	return nil
}

// volts is what the parts put on each analogue channel.
func (s *Sim) volts(ch int) float64 {
	for _, p := range s.Parts {
		if a, ok := p.(analogSource); ok {
			if v, on := a.Volts(ch); on {
				return v
			}
		}
	}
	return 0
}

type analogSource interface {
	Volts(channel int) (float64, bool)
}

// meter keeps a pin's high time over a step and the widths of its pulses.
type meter struct {
	high       bool
	since      uint64 // the cycle of the last edge
	start      uint64 // the step's first cycle
	highCycles uint64
	rise       uint64
	rose       bool
	pulses     []float64
}

func (m *meter) begin(cyc uint64) {
	m.start, m.highCycles, m.pulses = cyc, 0, m.pulses[:0]
}

func (m *meter) edge(high bool, cyc uint64) {
	if m.high {
		from := m.since
		if from < m.start {
			from = m.start
		}
		m.highCycles += cyc - from
	}
	if high && !m.high {
		m.rise, m.rose = cyc, true
	}
	if !high && m.high && m.rose {
		m.pulses = append(m.pulses, float64(cyc-m.rise)/avr.ClockHz*1e6)
	}
	m.high, m.since = high, cyc
}

func (m *meter) duty(now uint64) float64 {
	span := now - m.start
	if span == 0 {
		return b2f(m.high)
	}
	h := m.highCycles
	if m.high {
		from := m.since
		if from < m.start {
			from = m.start
		}
		h += now - from
	}
	return float64(h) / float64(span)
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// Report is a line about every part's state.
func (s *Sim) Report() string {
	out := fmt.Sprintf("t %.3f s", s.T)
	for _, p := range s.Parts {
		if r, ok := p.(interface{ State() string }); ok {
			out += " · " + p.Label() + ": " + r.State()
		}
	}
	return out
}
