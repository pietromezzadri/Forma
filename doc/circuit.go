package doc

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"forma/avr"
	"forma/elec"
)

// Electronics: a circuit feature is a board (an Arduino Uno), the firmware
// it runs (a compiled sketch's Intel HEX, kept in the feature's Data) and
// parts from the library wired to its pins; a servo's or a motor's shaft
// can drive a joint. It makes no bodies: regenerating checks the wiring,
// and a motion study (Study) runs the sketch and moves the joints.

// KCircuit is the circuit feature's kind.
const KCircuit = "circuit"

// Circuit is what a circuit feature holds.
type Circuit struct {
	Board string        `json:"board"`          // "uno"
	File  string        `json:"file,omitempty"` // where its firmware came from (a .hex, or the .ino compiled)
	Parts []CircuitPart `json:"parts,omitempty"`
}

// CircuitPart is a library part in the circuit.
type CircuitPart struct {
	Model  string             `json:"model"`
	Name   string             `json:"name"`
	Pins   map[string]string  `json:"pins,omitempty"`   // role → board pin ("D9", "A0")
	Params map[string]float64 `json:"params,omitempty"` // figures over the model's defaults
	Supply string             `json:"supply,omitempty"` // a driver's battery
	Motor  string             `json:"motor,omitempty"`  // and its motor
	// a servo's or motor's shaft driving a joint: the joint moves Ratio
	// (0 means 1) per degree past Zero (degrees; a slider: mm per degree)
	Joint int     `json:"joint,omitempty"`
	Ratio float64 `json:"ratio,omitempty"`
	Zero  float64 `json:"zero,omitempty"`
}

// NewCircuit is an empty Uno with no firmware yet.
func NewCircuit() *Feature {
	return &Feature{Kind: KCircuit, Circuit: &Circuit{Board: "uno"}}
}

// NewCircuitPart is a part of a model named name, with the usual wiring
// for its kind on a free pin and (a servo) its middle as the joint's zero.
func NewCircuitPart(c *Circuit, model, name string) CircuitPart {
	p := CircuitPart{Model: model, Name: name, Pins: map[string]string{}}
	m := elec.Library[model]
	if m == nil {
		return p
	}
	used := map[string]bool{}
	for _, q := range c.Parts {
		for _, v := range q.Pins {
			used[strings.ToUpper(v)] = true
		}
	}
	free := func(cands ...string) string {
		for _, s := range cands {
			if !used[s] {
				used[s] = true
				return s
			}
		}
		return ""
	}
	pwm := []string{"D9", "D10", "D6", "D5", "D3", "D11"}
	digital := []string{"D2", "D4", "D7", "D8", "D12", "D13"}
	for _, role := range m.Pins {
		switch {
		case m.Analog:
			p.Pins[role] = free("A0", "A1", "A2", "A3", "A4", "A5")
		case m.Kind == "servo" || role == "enable" || m.Kind == "led":
			p.Pins[role] = free(pwm...)
		default:
			p.Pins[role] = free(digital...)
		}
	}
	if m.Kind == "servo" {
		p.Zero = 90
		for _, q := range m.Params {
			if q.Name == "Start" {
				p.Zero = q.Default
			}
		}
	}
	// a driver takes the first battery and the first motor no driver has
	if m.Kind == "driver" {
		taken := map[string]bool{}
		for _, q := range c.Parts {
			taken[q.Motor] = true
		}
		for _, q := range c.Parts {
			k := elec.Library[q.Model]
			if k == nil {
				continue
			}
			if k.Kind == "battery" && p.Supply == "" {
				p.Supply = q.Name
			}
			if k.Kind == "motor" && p.Motor == "" && !taken[q.Name] {
				p.Motor = q.Name
			}
		}
	}
	return p
}

// PartName is a name for another part of a model ("SG90 micro servo 2").
func (c *Circuit) PartName(model string) string {
	base := model
	if m := elec.Library[model]; m != nil {
		base = m.Label
	}
	used := map[string]bool{}
	for _, p := range c.Parts {
		used[p.Name] = true
	}
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		if n := fmt.Sprintf("%s %d", base, i); !used[n] {
			return n
		}
	}
}

// circuitDeps are the joints a circuit drives.
func (f *Feature) circuitDeps() []int {
	if f.Circuit == nil {
		return nil
	}
	var out []int
	for _, p := range f.Circuit.Parts {
		if p.Joint != 0 {
			out = append(out, p.Joint)
		}
	}
	return out
}

// Firmware is the program in a circuit feature's HEX.
func Firmware(f *Feature) ([]uint16, error) {
	if strings.TrimSpace(f.Data) == "" {
		return nil, fmt.Errorf("no firmware loaded")
	}
	return avr.ParseHex(strings.NewReader(f.Data))
}

func (d *Document) buildCircuit(f *Feature) error {
	if f.Circuit == nil {
		return fmt.Errorf("the circuit is missing")
	}
	if b := f.Circuit.Board; b != "" && b != "uno" {
		return fmt.Errorf("board %q: only the Arduino Uno is emulated", b)
	}
	if _, _, err := d.circuitParts(f); err != nil {
		return err
	}
	if _, err := Firmware(f); err != nil {
		if f.Data == "" {
			f.Warn = "no firmware: load a sketch (.hex, or .ino with arduino-cli)"
			return nil
		}
		return err
	}
	return nil
}

// circuitParts makes the circuit's parts, wired to each other, and checks
// what they drive.
func (d *Document) circuitParts(f *Feature) ([]elec.Part, map[string]elec.Part, error) {
	c := f.Circuit
	byName := map[string]elec.Part{}
	var parts []elec.Part
	pins := map[string]string{} // board pin → the part on it
	joints := map[int]string{}
	for _, cp := range c.Parts {
		if cp.Name == "" {
			return nil, nil, fmt.Errorf("a %s has no name", cp.Model)
		}
		if byName[cp.Name] != nil {
			return nil, nil, fmt.Errorf("two parts are called %q", cp.Name)
		}
		p, err := elec.Make(cp.Model, cp.Name, cp.Pins, cp.Params)
		if err != nil {
			return nil, nil, err
		}
		for _, v := range cp.Pins {
			if v == "" {
				continue
			}
			pin, _ := avr.ParseUnoPin(v)
			k := avr.UnoName(pin)
			if other, ok := pins[k]; ok && other != cp.Name {
				return nil, nil, fmt.Errorf("%s and %s are both on %s", other, cp.Name, k)
			}
			pins[k] = cp.Name
		}
		if cp.Joint != 0 {
			if _, ok := elec.ShaftDegrees(p); !ok {
				return nil, nil, fmt.Errorf("%s has no shaft to drive a joint", cp.Name)
			}
			j := d.Feature(cp.Joint)
			if j == nil || j.Kind != KJoint {
				return nil, nil, fmt.Errorf("%s drives a joint that is gone", cp.Name)
			}
			if MotionInput(j) == "" {
				return nil, nil, fmt.Errorf("%s drives %s, a rigid joint", cp.Name, j.Name)
			}
			if other, ok := joints[cp.Joint]; ok {
				return nil, nil, fmt.Errorf("%s and %s both drive %s", other, cp.Name, j.Name)
			}
			joints[cp.Joint] = cp.Name
		}
		byName[cp.Name] = p
		parts = append(parts, p)
	}
	for _, cp := range c.Parts {
		if m := elec.Library[cp.Model]; m == nil || m.Kind != "driver" {
			continue
		}
		var sup, mot elec.Part
		if cp.Supply != "" {
			if sup = byName[cp.Supply]; sup == nil {
				return nil, nil, fmt.Errorf("%s: no part %q for its supply", cp.Name, cp.Supply)
			}
		}
		if cp.Motor != "" {
			if mot = byName[cp.Motor]; mot == nil {
				return nil, nil, fmt.Errorf("%s: no part %q for its motor", cp.Name, cp.Motor)
			}
		}
		if err := elec.Connect(byName[cp.Name], sup, mot); err != nil {
			return nil, nil, err
		}
	}
	return parts, byName, nil
}

// Study is a motion study: the circuit's sketch running on the emulated
// board, its parts stepped with it, its shafts setting their joints.
type Study struct {
	F     *Feature
	Sim   *elec.Sim
	Parts map[string]elec.Part
	// Was is each driven joint's motion input as it was before the study
	Was    map[int]string
	drives []drive
	left   float64 // time not yet run
}

type drive struct {
	part        elec.Part
	joint       int
	ratio, zero float64
}

// NewStudy starts a motion study of a circuit feature.
func (d *Document) NewStudy(f *Feature) (*Study, error) {
	if f == nil || f.Kind != KCircuit || f.Circuit == nil {
		return nil, fmt.Errorf("not a circuit")
	}
	prog, err := Firmware(f)
	if err != nil {
		return nil, err
	}
	parts, byName, err := d.circuitParts(f)
	if err != nil {
		return nil, err
	}
	s := &Study{F: f, Sim: elec.New(avr.NewUno(prog), parts...), Parts: byName, Was: map[int]string{}}
	for _, cp := range f.Circuit.Parts {
		if cp.Joint == 0 {
			continue
		}
		r := cp.Ratio
		if r == 0 {
			r = 1
		}
		s.drives = append(s.drives, drive{byName[cp.Name], cp.Joint, r, cp.Zero})
		if j := d.Feature(cp.Joint); j != nil {
			s.Was[cp.Joint] = j.Inputs[MotionInput(j)]
		}
	}
	return s, nil
}

// StudyStep is the step the board and parts are run in (1 ms: a PWM
// period, a fraction of a motor's time constant).
const StudyStep = 0.001

// Run advances the study by dt seconds, in whole steps (what is left over
// is run with the next call).
func (s *Study) Run(dt float64) {
	s.left += dt
	n := int(s.left/StudyStep + 1e-9)
	for i := 0; i < n; i++ {
		s.Sim.Step(StudyStep)
	}
	s.left -= float64(n) * StudyStep
}

// Pose sets the driven joints' motion from the shafts (regenerate after);
// it says whether any changed.
func (s *Study) Pose(d *Document) bool {
	changed := false
	for _, dr := range s.drives {
		j := d.Feature(dr.joint)
		if j == nil {
			continue
		}
		in := MotionInput(j)
		if in == "" {
			continue
		}
		deg, _ := elec.ShaftDegrees(dr.part)
		v := (deg - dr.zero) * dr.ratio
		if in == "Angle" {
			v = math.Remainder(v, 360)
		}
		txt := strconv.FormatFloat(math.Round(v*1000)/1000, 'f', -1, 64)
		if j.Inputs[in] != txt {
			j.Inputs[in] = txt
			changed = true
		}
	}
	return changed
}

// Restore puts the joints back as they were before the study.
func (s *Study) Restore(d *Document) {
	for id, v := range s.Was {
		if j := d.Feature(id); j != nil {
			if in := MotionInput(j); in != "" {
				j.Inputs[in] = v
			}
		}
	}
}

// Drives says which joints the study moves.
func (s *Study) Drives() []int {
	var ids []int
	for _, dr := range s.drives {
		ids = append(ids, dr.joint)
	}
	return ids
}
