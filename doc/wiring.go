package doc

import (
	"fmt"
	"sort"
	"strings"

	"forma/elec"
	"forma/geom"
)

// Circuit wiring (M21): a circuit keeps its wires, each joining two
// terminals ("uno.D9", "servo.SIG"; the board is elec.Board). A circuit
// made before wires picked a board pin per role of each part and named a
// driver's supply and motor; migrate turns that into the wires it meant —
// with a resistor in series with each LED and the supplies and grounds a
// servo, a pot, a button and a driver need — the first time the circuit is
// built.

// CircuitWire is a wire between two terminals ("part.terminal"), its
// colour ("red", "#3080ff"; empty: by what it carries) and the corners it
// is routed through on the wiring sheet.
type CircuitWire struct {
	A      string      `json:"a"`
	B      string      `json:"b"`
	Colour string      `json:"colour,omitempty"`
	Path   []geom.Vec2 `json:"path,omitempty"`
}

// Has says whether the wire has the terminal ref at an end.
func (w CircuitWire) Has(ref string) bool { return w.A == ref || w.B == ref }

// Other is the wire's end that isn't ref.
func (w CircuitWire) Other(ref string) string {
	if w.A == ref {
		return w.B
	}
	return w.A
}

// Connect adds a wire between two terminals (none if they are wired
// already); it says whether it added one.
func (c *Circuit) Connect(a, b string) bool {
	if a == b {
		return false
	}
	for _, w := range c.Wires {
		if w.Has(a) && w.Other(a) == b {
			return false
		}
	}
	c.Wires = append(c.Wires, CircuitWire{A: a, B: b})
	return true
}

// Disconnect takes off the wires between two terminals.
func (c *Circuit) Disconnect(a, b string) {
	out := c.Wires[:0]
	for _, w := range c.Wires {
		if !(w.Has(a) && w.Other(a) == b) {
			out = append(out, w)
		}
	}
	c.Wires = out
}

// Net is every terminal wired (directly or through others) to ref,
// ref included, sorted.
func (c *Circuit) Net(ref string) []string {
	seen := map[string]bool{ref: true}
	queue := []string{ref}
	for len(queue) > 0 {
		r := queue[0]
		queue = queue[1:]
		for _, w := range c.Wires {
			if w.Has(r) && !seen[w.Other(r)] {
				seen[w.Other(r)] = true
				queue = append(queue, w.Other(r))
			}
		}
	}
	var out []string
	for r := range seen {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// boardPin is a board pin's name if ref is one ("D9", "A0"), else "".
func boardPin(ref string) string {
	p, t, ok := elec.Split(ref)
	if !ok || p != elec.Board || len(t) < 2 || (t[0] != 'D' && t[0] != 'A') {
		return ""
	}
	return t
}

// BoardPin is the board pin a part's terminal is wired to ("" if none).
func (c *Circuit) BoardPin(part, term string) string {
	for _, r := range c.Net(part + "." + term) {
		if p := boardPin(r); p != "" {
			return p
		}
	}
	return ""
}

// SetBoardPin wires a part's terminal to a board pin instead of the pin
// it was on ("" takes it off its pin).
func (c *Circuit) SetBoardPin(part, term, pin string) {
	ref := part + "." + term
	out := c.Wires[:0]
	for _, w := range c.Wires {
		if w.Has(ref) && boardPin(w.Other(ref)) != "" {
			continue
		}
		out = append(out, w)
	}
	c.Wires = out
	if pin != "" {
		c.Connect(elec.Board+"."+pin, ref)
	}
}

// Linked is the part of a kind one of whose terminals (theirs) is on the
// same net as part.term ("" if none): a driver's VM to a battery's +.
func (c *Circuit) Linked(part, term, kind, theirs string) string {
	net := c.Net(part + "." + term)
	for _, p := range c.Parts {
		if m := elec.Library[p.Model]; m != nil && m.Kind == kind {
			for _, r := range net {
				if r == p.Name+"."+theirs {
					return p.Name
				}
			}
		}
	}
	return ""
}

// SetSupply wires a driver to a battery (VM to +, the battery's − to the
// driver's GND and the board's); "" takes it off.
func (c *Circuit) SetSupply(drv, bat string) {
	for _, p := range c.Parts {
		if m := elec.Library[p.Model]; m != nil && m.Kind == "battery" {
			c.Disconnect(drv+".VM", p.Name+".+")
			c.Disconnect(drv+".GND", p.Name+".-")
		}
	}
	if bat != "" {
		c.Connect(drv+".VM", bat+".+")
		c.Connect(drv+".GND", bat+".-")
		c.Connect(bat+".-", elec.Board+".GND")
	}
}

// SetMotor wires a driver's outputs to a motor ("" takes it off).
func (c *Circuit) SetMotor(drv, motor string) {
	for _, p := range c.Parts {
		if m := elec.Library[p.Model]; m != nil && m.Kind == "motor" {
			for _, o := range []string{"OUT1", "OUT2"} {
				for _, t := range []string{"1", "2"} {
					c.Disconnect(drv+"."+o, p.Name+"."+t)
				}
			}
		}
	}
	if motor != "" {
		c.Connect(drv+".OUT1", motor+".1")
		c.Connect(drv+".OUT2", motor+".2")
	}
}

// RenamePart renames a part and the ends of its wires.
func (c *Circuit) RenamePart(old, name string) {
	for i := range c.Parts {
		if c.Parts[i].Name == old {
			c.Parts[i].Name = name
		}
	}
	re := func(ref string) string {
		if p, t, ok := elec.Split(ref); ok && p == old {
			return name + "." + t
		}
		return ref
	}
	for i := range c.Wires {
		c.Wires[i].A, c.Wires[i].B = re(c.Wires[i].A), re(c.Wires[i].B)
	}
}

// RemovePart takes a part and its wires out.
func (c *Circuit) RemovePart(name string) {
	parts := c.Parts[:0]
	for _, p := range c.Parts {
		if p.Name != name {
			parts = append(parts, p)
		}
	}
	c.Parts = parts
	wires := c.Wires[:0]
	for _, w := range c.Wires {
		pa, _, _ := elec.Split(w.A)
		pb, _, _ := elec.Split(w.B)
		if pa != name && pb != name {
			wires = append(wires, w)
		}
	}
	c.Wires = wires
}

// AddPart puts a part of a model in the circuit wired as such a part
// usually is: its signal on a free pin, its supply and ground (an LED with
// a resistor of its own), a driver to the first battery and the first
// motor no driver has. It returns the part (in c.Parts).
func (c *Circuit) AddPart(model, name string) *CircuitPart {
	if name == "" {
		name = c.PartName(model)
	}
	c.Parts = append(c.Parts, NewCircuitPart(c, model, name))
	c.migrate()
	for i := range c.Parts {
		if c.Parts[i].Name == name {
			return &c.Parts[i]
		}
	}
	return nil
}

// usedPins are the board pins wires or old-style parts are on.
func (c *Circuit) usedPins() map[string]bool {
	used := map[string]bool{}
	for _, q := range c.Parts {
		for _, v := range q.Pins {
			used[strings.ToUpper(v)] = true
		}
	}
	for _, w := range c.Wires {
		for _, r := range []string{w.A, w.B} {
			if p := boardPin(r); p != "" {
				used[p] = true
			}
		}
	}
	return used
}

// migrate turns parts wired the old way (pins by role, a driver's supply
// and motor by name) into wires.
func (c *Circuit) migrate() {
	gnd, v5 := elec.Board+".GND", elec.Board+".5V"
	var add []CircuitPart
	names := map[string]bool{}
	for _, p := range c.Parts {
		names[p.Name] = true
	}
	for i := range c.Parts {
		p := &c.Parts[i]
		m := elec.Library[p.Model]
		if m == nil || len(p.Pins) == 0 && p.Supply == "" && p.Motor == "" {
			continue
		}
		n := p.Name
		for _, role := range m.Pins {
			pin := strings.ToUpper(strings.TrimSpace(p.Pins[role]))
			if pin == "" {
				continue
			}
			if pin[0] >= '0' && pin[0] <= '9' {
				pin = "D" + pin
			}
			term := m.Role[role]
			if m.Kind == "led" {
				// an LED gets the resistor it was always assumed to have
				r := n + " resistor"
				for k := 2; names[r]; k++ {
					r = fmt.Sprintf("%s resistor %d", n, k)
				}
				names[r] = true
				add = append(add, CircuitPart{Model: "resistor", Name: r})
				c.Connect(elec.Board+"."+pin, r+".1")
				c.Connect(r+".2", n+"."+term)
				continue
			}
			c.Connect(elec.Board+"."+pin, n+"."+term)
		}
		switch m.Kind {
		case "led":
			c.Connect(n+".K", gnd)
		case "servo":
			c.Connect(n+".V+", v5)
			c.Connect(n+".GND", gnd)
		case "pot":
			c.Connect(n+".1", gnd)
			c.Connect(n+".2", v5)
		case "button":
			c.Connect(n+".2", gnd)
		case "driver":
			c.Connect(n+".GND", gnd)
			if p.Supply != "" {
				c.SetSupply(n, p.Supply)
			}
			if p.Motor != "" {
				c.SetMotor(n, p.Motor)
			}
		}
		p.Pins, p.Supply, p.Motor = nil, "", ""
	}
	c.Parts = append(c.Parts, add...)
	c.fixModels()
}

// fixModels gives back a 3D model a part with none of its own was linked
// to by mistake: adding an LED with its 3D model once linked the model to
// the part added last, the LED's own resistor ("LED resistor"), so the LED
// on the model never lit. The model goes to the part the name says it came
// with, if that has none.
func (c *Circuit) fixModels() {
	for i := range c.Parts {
		p := &c.Parts[i]
		if p.Model3D == 0 || PartModels[p.Model] != nil {
			continue
		}
		for j := range c.Parts {
			q := &c.Parts[j]
			if j != i && q.Model3D == 0 && PartModels[q.Model] != nil && strings.HasPrefix(p.Name, q.Name+" ") {
				q.Model3D, p.Model3D = p.Model3D, 0
				break
			}
		}
	}
}

// specs and wires are the circuit as the simulation takes it.
func (c *Circuit) specs() ([]elec.Spec, []elec.Wire) {
	var s []elec.Spec
	for _, p := range c.Parts {
		s = append(s, elec.Spec{Model: p.Model, Name: p.Name, Params: p.Params})
	}
	var w []elec.Wire
	for _, x := range c.Wires {
		w = append(w, elec.Wire{A: x.A, B: x.B})
	}
	return s, w
}

// Issues are the checks on the circuit's wiring (see elec.Check).
func (c *Circuit) Issues() []elec.Issue {
	s, w := c.specs()
	return elec.Check(s, w)
}
