// Package expr evaluates the expressions typed into value fields:
// arithmetic, parentheses, units, a few functions and named parameters.
// Lengths come out in mm, angles in degrees.
package expr

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// Units map a suffix to its factor (lengths to mm, angles to degrees).
var Units = map[string]float64{
	"mm": 1, "cm": 10, "m": 1000, "in": 25.4, "ft": 304.8, "um": 1e-3,
	"deg": 1, "rad": 180 / math.Pi, "°": 1,
}

var funcs = map[string]func(float64) float64{
	"sqrt": math.Sqrt, "abs": math.Abs,
	"sin":   func(d float64) float64 { return math.Sin(d * math.Pi / 180) },
	"cos":   func(d float64) float64 { return math.Cos(d * math.Pi / 180) },
	"tan":   func(d float64) float64 { return math.Tan(d * math.Pi / 180) },
	"asin":  func(x float64) float64 { return math.Asin(x) * 180 / math.Pi },
	"acos":  func(x float64) float64 { return math.Acos(x) * 180 / math.Pi },
	"atan":  func(x float64) float64 { return math.Atan(x) * 180 / math.Pi },
	"round": math.Round, "floor": math.Floor, "ceil": math.Ceil,
}

// Eval evaluates s with the given named values (may be nil).
func Eval(s string, vars map[string]float64) (float64, error) {
	p := &parser{src: []rune(s), vars: vars}
	v, err := p.sum()
	if err != nil {
		return 0, err
	}
	p.space()
	if p.i < len(p.src) {
		return 0, fmt.Errorf("unexpected %q", string(p.src[p.i:]))
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("not a number")
	}
	return v, nil
}

type parser struct {
	src  []rune
	i    int
	vars map[string]float64
}

func (p *parser) space() {
	for p.i < len(p.src) && unicode.IsSpace(p.src[p.i]) {
		p.i++
	}
}

func (p *parser) peek() rune {
	p.space()
	if p.i < len(p.src) {
		return p.src[p.i]
	}
	return 0
}

func (p *parser) sum() (float64, error) {
	v, err := p.product()
	for err == nil {
		switch p.peek() {
		case '+':
			p.i++
			var w float64
			w, err = p.product()
			v += w
		case '-':
			p.i++
			var w float64
			w, err = p.product()
			v -= w
		default:
			return v, nil
		}
	}
	return v, err
}

func (p *parser) product() (float64, error) {
	v, err := p.power()
	for err == nil {
		switch p.peek() {
		case '*':
			p.i++
			var w float64
			w, err = p.power()
			v *= w
		case '/':
			p.i++
			var w float64
			w, err = p.power()
			if err == nil && w == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			v /= w
		default:
			return v, nil
		}
	}
	return v, err
}

func (p *parser) power() (float64, error) {
	v, err := p.unary()
	if err != nil {
		return v, err
	}
	if p.peek() == '^' {
		p.i++
		w, err := p.power()
		return math.Pow(v, w), err
	}
	return v, nil
}

func (p *parser) unary() (float64, error) {
	switch p.peek() {
	case '-':
		p.i++
		v, err := p.unary()
		return -v, err
	case '+':
		p.i++
		return p.unary()
	}
	return p.atom()
}

func (p *parser) atom() (float64, error) {
	c := p.peek()
	switch {
	case c == '(':
		p.i++
		v, err := p.sum()
		if err != nil {
			return 0, err
		}
		if p.peek() != ')' {
			return 0, fmt.Errorf("missing )")
		}
		p.i++
		return p.unit(v), nil
	case unicode.IsDigit(c) || c == '.':
		start := p.i
		for p.i < len(p.src) && (unicode.IsDigit(p.src[p.i]) || p.src[p.i] == '.' ||
			((p.src[p.i] == 'e' || p.src[p.i] == 'E') && p.i+1 < len(p.src) && (unicode.IsDigit(p.src[p.i+1]) || p.src[p.i+1] == '-'))) {
			if p.src[p.i] == 'e' || p.src[p.i] == 'E' {
				p.i++
			}
			p.i++
		}
		v, err := strconv.ParseFloat(string(p.src[start:p.i]), 64)
		if err != nil {
			return 0, fmt.Errorf("bad number %q", string(p.src[start:p.i]))
		}
		return p.unit(v), nil
	case unicode.IsLetter(c) || c == '_':
		name := p.ident()
		if f, ok := funcs[name]; ok && p.peek() == '(' {
			p.i++
			v, err := p.sum()
			if err != nil {
				return 0, err
			}
			if p.peek() != ')' {
				return 0, fmt.Errorf("missing ) after %s", name)
			}
			p.i++
			return p.unit(f(v)), nil
		}
		if name == "pi" {
			return p.unit(math.Pi), nil
		}
		if v, ok := p.vars[name]; ok {
			return p.unit(v), nil
		}
		return 0, fmt.Errorf("unknown name %q", name)
	case c == 0:
		return 0, fmt.Errorf("expression ends early")
	}
	return 0, fmt.Errorf("unexpected %q", string(c))
}

func (p *parser) ident() string {
	start := p.i
	for p.i < len(p.src) && (unicode.IsLetter(p.src[p.i]) || unicode.IsDigit(p.src[p.i]) || p.src[p.i] == '_') {
		p.i++
	}
	return string(p.src[start:p.i])
}

// unit applies a unit suffix right after a value, if there is one.
func (p *parser) unit(v float64) float64 {
	save := p.i
	p.space()
	if p.i < len(p.src) && p.src[p.i] == '°' {
		p.i++
		return v
	}
	if p.i < len(p.src) && unicode.IsLetter(p.src[p.i]) {
		name := p.ident()
		if f, ok := Units[strings.ToLower(name)]; ok {
			return v * f
		}
	}
	p.i = save
	return v
}
