package doc

import (
	"os"
	"strings"
	"testing"
)

// A Wokwi project with a breadboard: the parts it knows come in named by
// their ids, the Uno's pins as the board's, the breadboard's ground rail
// joining the LED, the button, the pot and the board's ground, colours
// kept; the LCD and the board's RESET pin are listed as left out.
func TestReadWokwi(t *testing.T) {
	diagram, err := os.ReadFile("testdata/wokwi_button/diagram.json")
	if err != nil {
		t.Fatal(err)
	}
	c, notes, err := ReadWokwi(diagram, "void setup(){}")
	if err != nil {
		t.Fatal(err)
	}
	models := map[string]string{}
	for _, p := range c.Parts {
		models[p.Name] = p.Model
	}
	want := map[string]string{"led1": "led", "r1": "resistor", "btn1": "button", "servo1": "sg90", "pot1": "pot"}
	for n, m := range want {
		if models[n] != m {
			t.Fatalf("%s is %q, want %q (%v)", n, models[n], m, models)
		}
	}
	if len(c.Parts) != len(want) {
		t.Fatalf("parts %v", models)
	}
	for _, p := range c.Parts {
		if p.Name == "r1" && p.Params["Resistance"] != 1000 {
			t.Fatalf("r1 is %v Ω", p.Params["Resistance"])
		}
	}
	gnd := strings.Join(c.Net("uno.GND"), " ")
	for _, r := range []string{"led1.K", "btn1.2", "pot1.1", "servo1.GND"} {
		if !strings.Contains(gnd, r) {
			t.Fatalf("%s not on ground: %s", r, gnd)
		}
	}
	for ref, pin := range map[string]string{"r1.1": "D13", "btn1.1": "D2", "servo1.SIG": "D9", "pot1.W": "A0"} {
		p, term, _ := strings.Cut(ref, ".")
		if got := c.BoardPin(p, term); got != pin {
			t.Fatalf("%s on %q, want %s", ref, got, pin)
		}
	}
	if n := strings.Join(c.Net("pot1.2"), " "); !strings.Contains(n, "uno.5V") {
		t.Fatalf("pot1.2 through the plus rail: %s", n)
	}
	if n := c.Net("led1.A"); len(n) != 2 || n[0] != "led1.A" || n[1] != "r1.2" {
		t.Fatalf("led1.A: %v", n)
	}
	for _, w := range c.Wires {
		if w.Has("servo1.V+") && w.Colour != "red" || w.Has("led1.K") && w.Colour != "black" {
			t.Fatalf("wire %+v lost its colour", w)
		}
	}
	all := strings.Join(notes, "\n")
	for _, frag := range []string{"lcd1 (lcd1602) left out", "uno:RESET left out", "breadboard"} {
		if !strings.Contains(all, frag) {
			t.Fatalf("notes don't say %q:\n%s", frag, all)
		}
	}
	for _, is := range c.Issues() {
		if is.Err {
			t.Fatalf("the circuit has an error: %s", is.Text)
		}
	}
	if c.Source == "" {
		t.Fatal("the sketch was dropped")
	}
}
