package doc

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"forma/elec"
)

// Wokwi (M21): a project made on wokwi.com is a diagram.json — its parts
// by type, each with an id, and its connections, each
// ["uno:13", "led1:A", colour, route] — beside its sketch. ReadWokwi turns
// the diagram into a circuit wired the same way: the parts it knows become
// library parts named by their ids, the Uno is the board, a breadboard's
// strips and rails join what is wired to them, and every connection is a
// wire keeping its colour. What it doesn't know is listed and left out.

// wokwiKind is how a Wokwi part type comes in: the library model and its
// pins by Wokwi's names ("" leaves a pin out).
type wokwiKind struct {
	model string
	pins  map[string]string
}

var wokwiKinds = map[string]wokwiKind{
	"wokwi-led":                 {"led", map[string]string{"A": "A", "C": "K"}},
	"wokwi-resistor":            {"resistor", map[string]string{"1": "1", "2": "2"}},
	"wokwi-pushbutton":          {"button", map[string]string{"1.l": "1", "1.r": "1", "2.l": "2", "2.r": "2"}},
	"wokwi-pushbutton-6mm":      {"button", map[string]string{"1.l": "1", "1.r": "1", "2.l": "2", "2.r": "2"}},
	"wokwi-servo":               {"sg90", map[string]string{"PWM": "SIG", "V+": "V+", "GND": "GND"}},
	"wokwi-potentiometer":       {"pot", map[string]string{"GND": "1", "SIG": "W", "VCC": "2"}},
	"wokwi-slide-potentiometer": {"pot", map[string]string{"GND": "1", "SIG": "W", "VCC": "2"}},
	// a slide switch's middle pin is its common: on joins it to pin 1 (the
	// other throw, pin 3, isn't modelled)
	"wokwi-slide-switch": {"switch", map[string]string{"2": "1", "1": "2"}},
}

// wokwiBoardPin is a Wokwi Uno pin as the board's terminal ("" if not one).
func wokwiBoardPin(p string) string {
	switch {
	case p == "5V" || p == "VIN":
		return p
	case p == "3.3V":
		return "3V3"
	case p == "GND" || strings.HasPrefix(p, "GND."):
		return "GND"
	case len(p) == 2 && p[0] == 'A' && p[1] >= '0' && p[1] <= '5':
		return p
	}
	if n, err := strconv.Atoi(p); err == nil && n >= 0 && n <= 13 {
		return fmt.Sprintf("D%d", n)
	}
	return ""
}

// wokwiStrip is the strip or rail a breadboard pin is on: "5t.a" → "5t"
// (a to e of column 5), "tp.3" → "tp" (the top plus rail).
func wokwiStrip(p string) string {
	if g, _, ok := strings.Cut(p, "."); ok {
		return g
	}
	return p
}

// parseOhms reads a resistance as Wokwi writes it: "220", "4.7k", "1M".
func parseOhms(s string) (float64, bool) {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(s, "Ω"), "ohm"))
	mul := 1.0
	switch {
	case strings.HasSuffix(s, "k") || strings.HasSuffix(s, "K"):
		mul, s = 1e3, s[:len(s)-1]
	case strings.HasSuffix(s, "M"):
		mul, s = 1e6, s[:len(s)-1]
	}
	v, err := strconv.ParseFloat(s, 64)
	return v * mul, err == nil && v > 0
}

// ReadWokwi reads a Wokwi diagram.json and the sketch beside it into a
// circuit (its code the sketch, not yet compiled). notes lists what was
// left out.
func ReadWokwi(diagram []byte, sketch string) (c *Circuit, notes []string, err error) {
	var d struct {
		Parts []struct {
			Type  string            `json:"type"`
			ID    string            `json:"id"`
			Attrs map[string]string `json:"attrs"`
		} `json:"parts"`
		Connections [][]json.RawMessage `json:"connections"`
	}
	if err := json.Unmarshal(diagram, &d); err != nil {
		return nil, nil, fmt.Errorf("not a Wokwi diagram: %v", err)
	}
	c = &Circuit{Board: "uno", Source: sketch}
	type known struct {
		name string
		kind wokwiKind
	}
	parts := map[string]known{} // Wokwi id → part
	boards := map[string]bool{} // breadboards by id
	uno := ""
	names := map[string]bool{elec.Board: true}
	for _, p := range d.Parts {
		switch {
		case p.Type == "wokwi-arduino-uno":
			if uno != "" {
				notes = append(notes, "a second Uno ("+p.ID+") left out: a circuit has one board")
				continue
			}
			uno = p.ID
		case strings.HasPrefix(p.Type, "wokwi-breadboard"):
			boards[p.ID] = true
		default:
			k, ok := wokwiKinds[p.Type]
			if !ok {
				notes = append(notes, fmt.Sprintf("%s (%s) left out: not in the library", p.ID, strings.TrimPrefix(p.Type, "wokwi-")))
				continue
			}
			name := p.ID
			for names[name] {
				name += "_"
			}
			names[name] = true
			cp := CircuitPart{Model: k.model, Name: name}
			if k.model == "resistor" {
				if v, ok := parseOhms(p.Attrs["value"]); ok {
					cp.Params = map[string]float64{"Resistance": v}
				}
			}
			c.Parts = append(c.Parts, cp)
			parts[p.ID] = known{name, k}
		}
	}
	if uno == "" {
		notes = append(notes, "no Arduino Uno in the diagram: the parts are wired to an Uno of Forma's own")
	}

	// each end: a terminal of ours, a breadboard strip ("bb:<id>:<strip>"),
	// or nothing (a part or pin left out)
	end := func(s string) (string, bool) {
		id, pin, ok := strings.Cut(s, ":")
		if !ok {
			return "", false
		}
		switch {
		case id == uno:
			if t := wokwiBoardPin(pin); t != "" {
				return elec.Board + "." + t, true
			}
		case boards[id]:
			return "bb:" + id + ":" + wokwiStrip(pin), true
		default:
			if k, ok := parts[id]; ok {
				if t := k.kind.pins[pin]; t != "" {
					return k.name + "." + t, true
				}
			}
		}
		return "", false
	}
	missed := map[string]bool{}
	type link struct{ a, b, colour string }
	var links []link
	for _, raw := range d.Connections {
		if len(raw) < 2 {
			continue
		}
		var from, to, colour string
		json.Unmarshal(raw[0], &from)
		json.Unmarshal(raw[1], &to)
		if len(raw) > 2 {
			json.Unmarshal(raw[2], &colour)
		}
		a, oka := end(from)
		b, okb := end(to)
		for _, x := range []struct {
			s  string
			ok bool
		}{{from, oka}, {to, okb}} {
			if id, _, _ := strings.Cut(x.s, ":"); !x.ok && !missed[x.s] {
				missed[x.s] = true
				if _, isPart := parts[id]; isPart || id == uno {
					notes = append(notes, x.s+" left out: Forma's part has no such pin")
				}
			}
		}
		if oka && okb {
			links = append(links, link{a, b, colour})
		}
	}

	// wires straight between terminals stay as they are; what goes through
	// a breadboard joins everything on its strip, one wire after another
	root := map[string]string{}
	var find func(string) string
	find = func(r string) string {
		if p, ok := root[r]; ok && p != r {
			p = find(p)
			root[r] = p
			return p
		}
		root[r] = r
		return r
	}
	strip := func(r string) bool { return strings.HasPrefix(r, "bb:") }
	colourOf := map[string]string{} // a terminal's colour on a strip
	for _, l := range links {
		if !strip(l.a) && !strip(l.b) {
			c.Connect(l.a, l.b)
			setColour(c, l.a, l.b, l.colour)
			continue
		}
		ra, rb := find(l.a), find(l.b)
		if ra != rb {
			root[ra] = rb
		}
		for _, r := range []string{l.a, l.b} {
			if !strip(r) && colourOf[r] == "" {
				colourOf[r] = l.colour
			}
		}
	}
	groups := map[string][]string{}
	for r := range root {
		if !strip(r) {
			groups[find(r)] = append(groups[find(r)], r)
		}
	}
	var keys []string
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		g := groups[k]
		sort.Slice(g, func(i, j int) bool {
			bi, bj := strings.HasPrefix(g[i], elec.Board+"."), strings.HasPrefix(g[j], elec.Board+".")
			if bi != bj {
				return bi
			}
			return g[i] < g[j]
		})
		for i := 1; i < len(g); i++ {
			// the board's or a supply's terminal first: a wire from it
			c.Connect(g[0], g[i])
			setColour(c, g[0], g[i], colourOf[g[i]])
		}
	}
	if len(boards) > 0 {
		notes = append(notes, "a part pushed into a breadboard joins it where it sits, which isn't read: wire such parts on the sheet")
	}
	return c, notes, nil
}

// setColour gives the wire between a and b a colour ("" leaves it).
func setColour(c *Circuit, a, b, colour string) {
	if colour == "" {
		return
	}
	for i := range c.Wires {
		if c.Wires[i].Has(a) && c.Wires[i].Other(a) == b {
			c.Wires[i].Colour = colour
		}
	}
}
