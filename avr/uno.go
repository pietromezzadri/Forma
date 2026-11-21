package avr

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Uno is an Arduino Uno (or Nano): the chip with the board's pin names.
type Uno struct{ *Chip }

// NewUno makes an Uno running prog.
func NewUno(prog []uint16) *Uno { return &Uno{NewChip(prog)} }

// LoadUno makes an Uno running a compiled sketch (.hex).
func LoadUno(path string) (*Uno, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	prog, err := ParseHex(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return NewUno(prog), nil
}

// D is the Uno's digital pin n (D0..D13; 14..19 are A0..A5).
func D(n int) Pin {
	switch {
	case n < 8:
		return P(PortD, n)
	case n < 14:
		return P(PortB, n-8)
	default:
		return P(PortC, n-14)
	}
}

// A is analogue pin n (A0..A5) as a digital pin.
func A(n int) Pin { return P(PortC, n) }

// UnoName is a pin's name on the board (D3, A0, …).
func UnoName(p Pin) string {
	switch p.Port() {
	case PortD:
		return fmt.Sprintf("D%d", p.Bit())
	case PortB:
		if p.Bit() < 6 {
			return fmt.Sprintf("D%d", 8+p.Bit())
		}
	case PortC:
		if p.Bit() < 6 {
			return fmt.Sprintf("A%d", p.Bit())
		}
	}
	return p.String()
}

// PWMPins are the pins analogWrite drives with a timer: D3, D5, D6, D9,
// D10, D11.
var PWMPins = []int{3, 5, 6, 9, 10, 11}

// ParseUnoPin reads a pin's name: D0..D13, A0..A5, or a bare number
// (0..19, as Arduino numbers them).
func ParseUnoPin(s string) (Pin, error) {
	t := strings.ToUpper(strings.TrimSpace(s))
	base, max := 0, 19
	switch {
	case strings.HasPrefix(t, "D"):
		t, max = t[1:], 13
	case strings.HasPrefix(t, "A"):
		t, base, max = t[1:], 14, 5
	}
	n, err := strconv.Atoi(t)
	if err != nil || n < 0 || n > max {
		return 0, fmt.Errorf("no pin %q on an Uno (D0..D13, A0..A5)", s)
	}
	return D(base + n), nil
}

// AnalogChannel is the ADC channel of an analogue pin (A0..A5).
func AnalogChannel(p Pin) (int, bool) {
	if p.Port() == PortC && p.Bit() < 6 {
		return p.Bit(), true
	}
	return 0, false
}
