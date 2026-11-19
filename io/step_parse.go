package io

// A reader for ISO 10303-21 exchange files: the DATA section's entity
// instances, simple (#1=LINE(...)) or complex (#1=(A(...) B(...))), with
// their parameters as a tree of values.

import (
	"fmt"
	"strconv"
	"strings"
)

type p21Kind byte

const (
	p21Null  p21Kind = iota // $
	p21Deriv                // *
	p21Num
	p21Str
	p21Enum // .T.
	p21Ref  // #12
	p21List
	p21Typed // LENGTH_MEASURE(1.E-06)
	p21Bin
)

type p21Val struct {
	Kind p21Kind
	Num  float64
	Str  string // string, enumeration, typed value's type
	Ref  int
	List []p21Val // list items, or the typed value's one parameter
}

// p21Part is one TYPE(params) of an instance.
type p21Part struct {
	Type string
	Args []p21Val
}

type p21Ent struct {
	ID    int
	Parts []p21Part // one, or several for a complex instance
}

// Is reports whether the instance is (or includes) the type.
func (e *p21Ent) Is(t string) bool { return e.Part(t) != nil }

// Part is the instance's part of a type, or nil.
func (e *p21Ent) Part(t string) *p21Part {
	for i := range e.Parts {
		if e.Parts[i].Type == t {
			return &e.Parts[i]
		}
	}
	return nil
}

// Type is a simple instance's type (a complex one's first).
func (e *p21Ent) Type() string { return e.Parts[0].Type }

type p21File struct {
	Ents   map[int]*p21Ent
	Order  []int // in file order
	Schema string
}

func parseP21(src string) (*p21File, error) {
	p := &p21Parser{s: stripComments(src)}
	f := &p21File{Ents: map[int]*p21Ent{}}
	if !strings.HasPrefix(strings.TrimSpace(p.s), "ISO-10303-21") {
		return nil, fmt.Errorf("not a STEP file (no ISO-10303-21 header)")
	}
	if i := strings.Index(p.s, "FILE_SCHEMA"); i >= 0 {
		j := strings.Index(p.s[i:], ";")
		if j > 0 {
			f.Schema = p.s[i : i+j]
		}
	}
	d := indexKeyword(p.s, "DATA")
	if d < 0 {
		return nil, fmt.Errorf("no DATA section")
	}
	p.i = d + len("DATA")
	p.ws()
	if p.peek() == '(' { // DATA('name', ('schema')); in edition 3 files
		if _, err := p.value(); err != nil {
			return nil, err
		}
		p.ws()
	}
	if !p.eat(';') {
		return nil, p.errf("expected ; after DATA")
	}
	for {
		p.ws()
		if p.i >= len(p.s) {
			return nil, fmt.Errorf("DATA section has no ENDSEC")
		}
		if strings.HasPrefix(p.s[p.i:], "ENDSEC") {
			break
		}
		e, err := p.instance()
		if err != nil {
			return nil, err
		}
		if _, dup := f.Ents[e.ID]; dup {
			return nil, fmt.Errorf("entity #%d defined twice", e.ID)
		}
		f.Ents[e.ID] = e
		f.Order = append(f.Order, e.ID)
	}
	return f, nil
}

func stripComments(s string) string {
	if !strings.Contains(s, "/*") {
		return s
	}
	var b strings.Builder
	inStr := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\'' {
			inStr = !inStr
		}
		if !inStr && c == '/' && i+1 < len(s) && s[i+1] == '*' {
			j := strings.Index(s[i+2:], "*/")
			if j < 0 {
				break
			}
			i += j + 3
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// indexKeyword finds a section keyword standing on its own (DATA; not
// FILE_DATA or inside a string).
func indexKeyword(s, kw string) int {
	from := 0
	for {
		i := strings.Index(s[from:], kw)
		if i < 0 {
			return -1
		}
		i += from
		before := i == 0 || !isIdent(s[i-1])
		after := i+len(kw) >= len(s) || !isIdent(s[i+len(kw)])
		if before && after {
			return i
		}
		from = i + len(kw)
	}
}

func isIdent(c byte) bool {
	return c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
}

type p21Parser struct {
	s string
	i int
}

func (p *p21Parser) errf(f string, a ...any) error {
	line := strings.Count(p.s[:min(p.i, len(p.s))], "\n") + 1
	return fmt.Errorf("STEP line %d: %s", line, fmt.Sprintf(f, a...))
}

func (p *p21Parser) ws() {
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\r', '\n':
			p.i++
		default:
			return
		}
	}
}

func (p *p21Parser) peek() byte {
	if p.i < len(p.s) {
		return p.s[p.i]
	}
	return 0
}

func (p *p21Parser) eat(c byte) bool {
	p.ws()
	if p.peek() == c {
		p.i++
		return true
	}
	return false
}

func (p *p21Parser) ident() string {
	p.ws()
	j := p.i
	for p.i < len(p.s) && isIdent(p.s[p.i]) {
		p.i++
	}
	return strings.ToUpper(p.s[j:p.i])
}

func (p *p21Parser) instance() (*p21Ent, error) {
	if !p.eat('#') {
		return nil, p.errf("expected an entity instance")
	}
	id, err := strconv.Atoi(p.ident())
	if err != nil {
		return nil, p.errf("bad entity number")
	}
	if !p.eat('=') {
		return nil, p.errf("expected = after #%d", id)
	}
	e := &p21Ent{ID: id}
	p.ws()
	if p.peek() == '(' { // complex instance
		p.i++
		for {
			p.ws()
			if p.eat(')') {
				break
			}
			part, err := p.part()
			if err != nil {
				return nil, err
			}
			e.Parts = append(e.Parts, part)
		}
	} else {
		part, err := p.part()
		if err != nil {
			return nil, err
		}
		e.Parts = []p21Part{part}
	}
	if len(e.Parts) == 0 {
		return nil, p.errf("#%d has no type", id)
	}
	if !p.eat(';') {
		return nil, p.errf("expected ; after #%d", id)
	}
	return e, nil
}

func (p *p21Parser) part() (p21Part, error) {
	t := p.ident()
	if t == "" {
		return p21Part{}, p.errf("expected an entity type")
	}
	if !p.eat('(') {
		return p21Part{}, p.errf("expected ( after %s", t)
	}
	args, err := p.list()
	return p21Part{Type: t, Args: args}, err
}

// list reads values up to the closing parenthesis (the opening one read).
func (p *p21Parser) list() ([]p21Val, error) {
	var out []p21Val
	p.ws()
	if p.eat(')') {
		return out, nil
	}
	for {
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		if p.eat(',') {
			continue
		}
		if p.eat(')') {
			return out, nil
		}
		return nil, p.errf("expected , or )")
	}
}

func (p *p21Parser) value() (p21Val, error) {
	p.ws()
	c := p.peek()
	switch {
	case c == '$':
		p.i++
		return p21Val{Kind: p21Null}, nil
	case c == '*':
		p.i++
		return p21Val{Kind: p21Deriv}, nil
	case c == '#':
		p.i++
		id, err := strconv.Atoi(p.ident())
		if err != nil {
			return p21Val{}, p.errf("bad reference")
		}
		return p21Val{Kind: p21Ref, Ref: id}, nil
	case c == '(':
		p.i++
		l, err := p.list()
		return p21Val{Kind: p21List, List: l}, err
	case c == '\'':
		s, err := p.str()
		return p21Val{Kind: p21Str, Str: s}, err
	case c == '"':
		j := strings.IndexByte(p.s[p.i+1:], '"')
		if j < 0 {
			return p21Val{}, p.errf("unterminated binary")
		}
		v := p21Val{Kind: p21Bin, Str: p.s[p.i+1 : p.i+1+j]}
		p.i += j + 2
		return v, nil
	case c == '.':
		j := strings.IndexByte(p.s[p.i+1:], '.')
		if j < 0 {
			return p21Val{}, p.errf("unterminated enumeration")
		}
		v := p21Val{Kind: p21Enum, Str: strings.ToUpper(p.s[p.i+1 : p.i+1+j])}
		p.i += j + 2
		return v, nil
	case c == '-' || c == '+' || c >= '0' && c <= '9':
		j := p.i
		p.i++
		for p.i < len(p.s) {
			d := p.s[p.i]
			if d >= '0' && d <= '9' || d == '.' || d == 'E' || d == 'e' ||
				(d == '-' || d == '+') && (p.s[p.i-1] == 'E' || p.s[p.i-1] == 'e') {
				p.i++
				continue
			}
			break
		}
		x, err := strconv.ParseFloat(p.s[j:p.i], 64)
		if err != nil {
			return p21Val{}, p.errf("bad number %q", p.s[j:p.i])
		}
		return p21Val{Kind: p21Num, Num: x}, nil
	case isIdent(c):
		t := p.ident()
		if !p.eat('(') {
			return p21Val{}, p.errf("expected ( after %s", t)
		}
		l, err := p.list()
		return p21Val{Kind: p21Typed, Str: t, List: l}, err
	}
	return p21Val{}, p.errf("unexpected %q", c)
}

func (p *p21Parser) str() (string, error) {
	p.i++ // opening quote
	var b strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == '\'' {
			if p.i+1 < len(p.s) && p.s[p.i+1] == '\'' {
				b.WriteByte('\'')
				p.i += 2
				continue
			}
			p.i++
			return decodeP21(b.String()), nil
		}
		b.WriteByte(c)
		p.i++
	}
	return "", p.errf("unterminated string")
}

// decodeP21 turns \X2\…\X0\, \X4\…\X0\, \X\hh and \\ into text.
func decodeP21(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		rest := s[i:]
		switch {
		case strings.HasPrefix(rest, "\\\\"):
			b.WriteByte('\\')
			i++
		case strings.HasPrefix(rest, "\\X2\\") || strings.HasPrefix(rest, "\\X4\\"):
			w := 4
			if rest[2] == '4' {
				w = 8
			}
			end := strings.Index(rest[4:], "\\X0\\")
			if end < 0 {
				b.WriteString(rest)
				return b.String()
			}
			hex := rest[4 : 4+end]
			for k := 0; k+w <= len(hex); k += w {
				if r, err := strconv.ParseUint(hex[k:k+w], 16, 32); err == nil {
					b.WriteRune(rune(r))
				}
			}
			i += 4 + end + 3
		case strings.HasPrefix(rest, "\\X\\") && len(rest) >= 5:
			if r, err := strconv.ParseUint(rest[3:5], 16, 8); err == nil {
				b.WriteRune(rune(r))
			}
			i += 4
		default:
			b.WriteByte('\\')
		}
	}
	return b.String()
}
