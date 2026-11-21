package elec

import (
	"fmt"
	"math"
	"sort"

	"forma/avr"
)

// Model is a part in the library: what it is, the pins it is wired by
// (roles such as "signal" or "in1"), its figures (with their defaults) and
// whether it can drive a joint.
type Model struct {
	ID     string
	Kind   string // battery, driver, motor, servo, led, button, pot
	Label  string
	About  string
	Pins   []string // roles wired to the board's pins
	Params []Param  // its figures, defaults first
	Drives bool     // its shaft can drive a joint
	Analog bool     // its pin is an analogue channel (A0..A5)
	Needs  []string // other parts it is wired to: "supply", "motor"
	make   func(n string, pin map[string]avr.Pin, p map[string]float64) Part
}

// Param is one of a model's figures.
type Param struct {
	Name, Unit string
	Default    float64
}

// Library is every model, by ID.
var Library = map[string]*Model{}

// Models are the library's IDs in order.
func Models() []string {
	ids := make([]string, 0, len(Library))
	for id := range Library {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := Library[ids[i]], Library[ids[j]]
		if a.Kind != b.Kind {
			return kindOrder[a.Kind] < kindOrder[b.Kind]
		}
		return a.Label < b.Label
	})
	return ids
}

var kindOrder = map[string]int{"servo": 0, "motor": 1, "driver": 2, "battery": 3, "pot": 4, "button": 5, "led": 6}

func add(m *Model) { Library[m.ID] = m }

func init() {
	add(&Model{ID: "sg90", Kind: "servo", Label: "SG90 micro servo", Drives: true,
		About:  "9 g, 180°, 0.1 s for 60° at 4.8 V; signal 544–2400 µs (the Servo library's range)",
		Pins:   []string{"signal"},
		Params: []Param{{"Range", "°", 180}, {"Speed", "°/s", 600}, {"Min pulse", "µs", 544}, {"Max pulse", "µs", 2400}, {"Start", "°", 90}},
		make: func(n string, pin map[string]avr.Pin, p map[string]float64) Part {
			s := SG90(n, pin["signal"])
			s.Range, s.Speed, s.MinUs, s.MaxUs = p["Range"], p["Speed"], p["Min pulse"], p["Max pulse"]
			s.Angle, s.Target = p["Start"], p["Start"]
			return s
		}})
	add(&Model{ID: "mg996r", Kind: "servo", Label: "MG996R servo", Drives: true,
		About:  "55 g metal-gear servo, 180°, 0.17 s for 60° at 6 V",
		Pins:   []string{"signal"},
		Params: []Param{{"Range", "°", 180}, {"Speed", "°/s", 350}, {"Min pulse", "µs", 544}, {"Max pulse", "µs", 2400}, {"Start", "°", 90}},
		make:   Library["sg90"].make})
	add(&Model{ID: "tt-motor", Kind: "motor", Label: "TT gear motor", Drives: true,
		About:  "the yellow 1:48 hobby gear motor: 200 rpm and 0.078 N·m at 6 V",
		Params: []Param{{"Rated", "V", 6}, {"No-load speed", "rpm", 200}, {"Stall torque", "N·m", 0.078}, {"Inertia", "kg·m²", 2e-5}, {"Load", "N·m", 0}},
		make:   motorMake})
	add(&Model{ID: "n20-motor", Kind: "motor", Label: "N20 gear motor", Drives: true,
		About:  "12 mm micro gear motor (1:100): 300 rpm and 0.04 N·m at 6 V",
		Params: []Param{{"Rated", "V", 6}, {"No-load speed", "rpm", 300}, {"Stall torque", "N·m", 0.04}, {"Inertia", "kg·m²", 5e-6}, {"Load", "N·m", 0}},
		make:   motorMake})
	add(&Model{ID: "tb6612", Kind: "driver", Label: "TB6612 motor driver", Needs: []string{"supply", "motor"},
		About:  "MOSFET H-bridge: 0.1 V lost; IN1 / IN2 the direction, PWM on the enable",
		Pins:   []string{"in1", "in2", "enable"},
		Params: []Param{{"Drop", "V", 0.1}},
		make:   driverMake})
	add(&Model{ID: "l298n", Kind: "driver", Label: "L298N motor driver", Needs: []string{"supply", "motor"},
		About:  "bipolar H-bridge: about 1.4 V lost",
		Pins:   []string{"in1", "in2", "enable"},
		Params: []Param{{"Drop", "V", 1.4}},
		make:   driverMake})
	add(&Model{ID: "battery-4aa", Kind: "battery", Label: "4 × AA pack", About: "6 V, 0.6 Ω",
		Params: []Param{{"Volts", "V", 6}, {"Resistance", "Ω", 0.6}}, make: batteryMake})
	add(&Model{ID: "battery-2s", Kind: "battery", Label: "2S Li-ion pack", About: "7.4 V, 0.1 Ω",
		Params: []Param{{"Volts", "V", 7.4}, {"Resistance", "Ω", 0.1}}, make: batteryMake})
	add(&Model{ID: "pot", Kind: "pot", Label: "Potentiometer", Analog: true,
		About:  "10 kΩ across 5 V, the wiper on an analogue input",
		Pins:   []string{"wiper"},
		Params: []Param{{"Value", "0..1", 0.5}},
		make: func(n string, pin map[string]avr.Pin, p map[string]float64) Part {
			return &Pot{Name: n, Channel: int(pin["wiper"]), Value: p["Value"]}
		}})
	add(&Model{ID: "button", Kind: "button", Label: "Push button", About: "between the pin and ground (INPUT_PULLUP: pressed reads LOW)",
		Pins: []string{"pin"},
		make: func(n string, pin map[string]avr.Pin, _ map[string]float64) Part {
			return &Button{Name: n, Pin: pin["pin"]}
		}})
	add(&Model{ID: "led", Kind: "led", Label: "LED", About: "its brightness the pin's duty",
		Pins: []string{"pin"},
		make: func(n string, pin map[string]avr.Pin, _ map[string]float64) Part {
			return &LED{Name: n, Pin: pin["pin"]}
		}})
}

func motorMake(n string, _ map[string]avr.Pin, p map[string]float64) Part {
	return &Motor{Name: n, Rated: p["Rated"], NoLoadRPM: p["No-load speed"], StallTorque: p["Stall torque"],
		Inertia: p["Inertia"], Load: p["Load"]}
}

func driverMake(n string, pin map[string]avr.Pin, p map[string]float64) Part {
	return &Driver{Name: n, In1: pin["in1"], In2: pin["in2"], En: pin["enable"], Drop: p["Drop"]}
}

func batteryMake(n string, _ map[string]avr.Pin, p map[string]float64) Part {
	return &Battery{Name: n, Volts: p["Volts"], R: p["Resistance"]}
}

// Make builds a part of a model: pins by role (names such as "D9" or "A0";
// an empty one is unconnected), figures over the defaults.
func Make(model, name string, pins map[string]string, params map[string]float64) (Part, error) {
	m := Library[model]
	if m == nil {
		return nil, fmt.Errorf("no part %q in the library", model)
	}
	pin := map[string]avr.Pin{}
	for _, role := range m.Pins {
		s := pins[role]
		if s == "" {
			if m.Kind == "driver" && role == "enable" {
				pin[role] = NoPin
				continue
			}
			return nil, fmt.Errorf("%s: its %s pin isn't wired", name, role)
		}
		p, err := avr.ParseUnoPin(s)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", name, role, err)
		}
		if m.Analog {
			ch, ok := avr.AnalogChannel(p)
			if !ok {
				return nil, fmt.Errorf("%s: %s is not an analogue input (A0..A5)", name, s)
			}
			p = avr.Pin(ch)
		}
		pin[role] = p
	}
	p := map[string]float64{}
	for _, q := range m.Params {
		p[q.Name] = q.Default
		if v, ok := params[q.Name]; ok {
			p[q.Name] = v
		}
	}
	return m.make(name, pin, p), nil
}

// Connect wires a driver to its supply and motor.
func Connect(drv Part, supply, motor Part) error {
	d, ok := drv.(*Driver)
	if !ok {
		return fmt.Errorf("%s is not a motor driver", drv.Label())
	}
	if supply != nil {
		b, ok := supply.(*Battery)
		if !ok {
			return fmt.Errorf("%s: its supply %s is not a battery", d.Name, supply.Label())
		}
		d.Supply = b
	}
	if motor != nil {
		m, ok := motor.(*Motor)
		if !ok {
			return fmt.Errorf("%s: %s is not a motor", d.Name, motor.Label())
		}
		d.Motor = m
	}
	return nil
}

// ShaftDegrees is how far a part's shaft has turned (a servo: its angle,
// a motor: from the start), and whether it has a shaft.
func ShaftDegrees(p Part) (float64, bool) {
	switch v := p.(type) {
	case *Servo:
		return v.Angle, true
	case *Motor:
		return v.Angle * 180 / math.Pi, true
	}
	return 0, false
}
