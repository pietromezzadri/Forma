package elec

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"forma/avr"
)

// Wiring by wires (M21): every part has terminals ("SIG", "+", "A"), the
// board's are its pins and supplies ("D9", "A0", "5V", "GND"), and wires
// join two of them ("uno.D9", "servo.SIG"). Terminals joined by wires make
// nets. Each step the nets are solved as a small DC circuit — the board's
// output pins (their duty over the step, through their output
// resistance), its pull-ups, its 5 V and 3.3 V, the batteries, resistors,
// pots, closed switches and buttons, and LEDs (a forward voltage over a
// small resistance, conducting or not) — by nodal analysis, the board's
// ground the reference. What comes out: the levels the board's input pins
// read and the voltages on its analogue ones, the current through each
// LED, whether a servo has power. Servos, motor drivers and motors keep
// their lumped models, fed by what their terminals are wired to: a servo's
// signal from the board pin on its SIG net, a driver's inputs from a pin's
// duty or a fixed level, its supply the battery on its VM net and its
// motor the one across OUT1 and OUT2.

// Board is the name the board goes by in wires.
const Board = "uno"

// BoardTerms are the board's terminals.
var BoardTerms = func() []string {
	var t []string
	for i := 0; i <= 13; i++ {
		t = append(t, fmt.Sprintf("D%d", i))
	}
	for i := 0; i <= 5; i++ {
		t = append(t, fmt.Sprintf("A%d", i))
	}
	return append(t, "5V", "3V3", "GND", "VIN")
}()

// Wire joins two terminals, each "part.terminal".
type Wire struct{ A, B string }

// Spec is a part of a circuit: its model, name and figures.
type Spec struct {
	Model, Name string
	Params      map[string]float64
}

// Electrical constants of the board and parts.
const (
	pinOhms    = 25.0    // an output pin's resistance
	pullupOhms = 35000.0 // the pull-up's
	railOhms   = 0.05    // the 5 V and 3.3 V rails'
	closedOhms = 0.05    // a closed switch's or button's
	ledOhms    = 15.0    // an LED's, conducting
	ledFull    = 0.020   // the current an LED is fully lit at
	potOhms    = 10000.0 // a pot's, end to end, when its figure is missing
	leak       = 1e-12   // siemens from every net to ground, so floating nets solve
	logicHigh  = 2.5     // volts an input reads as high above
	servoVolts = 4.0     // a servo needs at least this across V+ and GND
)

// Split is a terminal reference's part and terminal.
func Split(ref string) (part, term string, ok bool) {
	i := strings.LastIndex(ref, ".")
	if i <= 0 || i == len(ref)-1 {
		return "", "", false
	}
	return ref[:i], ref[i+1:], true
}

// Terms are a part's terminals by its model (the board's for Board).
func Terms(model string) []string {
	if model == Board {
		return BoardTerms
	}
	if m := Library[model]; m != nil {
		return m.Terms
	}
	return nil
}

// Build makes a simulation of a board running its sketch, with the parts
// wired by wires; it returns the parts by name.
func Build(b *avr.Uno, specs []Spec, wires []Wire) (*Sim, map[string]Part, error) {
	nw, parts, byName, err := buildNetwork(specs, wires)
	if err != nil {
		return nil, nil, err
	}
	nw.board = b
	s := New(b)
	s.Add(nw)
	for _, p := range parts {
		s.Add(p)
	}
	return s, byName, nil
}

// buildNetwork makes the parts and the network joining them.
func buildNetwork(specs []Spec, wires []Wire) (*Network, []Part, map[string]Part, error) {
	model := map[string]string{Board: Board}
	var names []string
	for _, sp := range specs {
		if sp.Name == "" {
			return nil, nil, nil, fmt.Errorf("a %s has no name", sp.Model)
		}
		if sp.Name == Board {
			return nil, nil, nil, fmt.Errorf("a part can't be called %q (the board's name)", Board)
		}
		if _, ok := model[sp.Name]; ok {
			return nil, nil, nil, fmt.Errorf("two parts are called %q", sp.Name)
		}
		if Library[sp.Model] == nil {
			return nil, nil, nil, fmt.Errorf("no part %q in the library", sp.Model)
		}
		model[sp.Name] = sp.Model
		names = append(names, sp.Name)
	}
	nw := &Network{netOf: map[string]int{}}
	// every terminal a net of its own, then wires join them
	parent := []int{}
	add := func(ref string) {
		nw.netOf[ref] = len(parent)
		parent = append(parent, len(parent))
	}
	for _, t := range BoardTerms {
		add(Board + "." + t)
	}
	for _, n := range names {
		for _, t := range Terms(model[n]) {
			add(n + "." + t)
		}
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	nw.degree = map[string]int{}
	for _, w := range wires {
		for _, ref := range []string{w.A, w.B} {
			if _, ok := nw.netOf[ref]; !ok {
				p, t, ok := Split(ref)
				if !ok {
					return nil, nil, nil, fmt.Errorf("a wire end %q isn't part.terminal", ref)
				}
				if _, known := model[p]; !known {
					return nil, nil, nil, fmt.Errorf("a wire goes to %s: no part %q", ref, p)
				}
				return nil, nil, nil, fmt.Errorf("a wire goes to %s: %s has no terminal %q (it has %s)", ref, p, t, strings.Join(Terms(model[p]), ", "))
			}
			nw.degree[ref]++
		}
		a, b := find(nw.netOf[w.A]), find(nw.netOf[w.B])
		parent[a] = b
	}
	// number the nets
	id := map[int]int{}
	for ref, i := range nw.netOf {
		r := find(i)
		if _, ok := id[r]; !ok {
			id[r] = len(id)
		}
		nw.netOf[ref] = id[r]
	}
	nw.n = len(id)
	nw.gnd = nw.netOf[Board+".GND"]
	nw.v5, nw.v33 = nw.netOf[Board+".5V"], nw.netOf[Board+".3V3"]
	nw.members = make([][]string, nw.n)
	for ref, i := range nw.netOf {
		nw.members[i] = append(nw.members[i], ref)
	}
	for i := range nw.members {
		sort.Strings(nw.members[i])
	}
	// the board's pins on nets with anything else on them
	for i := 0; i <= 19; i++ {
		name := avr.UnoName(avr.D(i))
		ref := Board + "." + name
		if len(nw.members[nw.netOf[ref]]) > 1 {
			nw.pins = append(nw.pins, netPin{avr.D(i), nw.netOf[ref]})
		}
	}
	// the parts, bound to their nets
	var parts []Part
	byName := map[string]Part{}
	net := func(name, term string) int { return nw.netOf[name+"."+term] }
	for _, sp := range specs {
		m := Library[sp.Model]
		pin := map[string]avr.Pin{}
		for _, r := range m.Pins {
			pin[r] = NoPin
		}
		p := map[string]float64{}
		for _, q := range m.Params {
			p[q.Name] = q.Default
			if v, ok := sp.Params[q.Name]; ok {
				p[q.Name] = v
			}
		}
		part := m.make(sp.Name, pin, p)
		n := sp.Name
		switch v := part.(type) {
		case *Battery:
			nw.bats = append(nw.bats, netBat{v, net(n, "+"), net(n, "-")})
		case *Resistor:
			nw.res = append(nw.res, netRes{net(n, "1"), net(n, "2"), func() float64 { return v.Ohms }, func(a float64) { v.Amps = a }})
		case *Switch:
			nw.res = append(nw.res, netRes{net(n, "1"), net(n, "2"), func() float64 {
				if v.On {
					return closedOhms
				}
				return 0
			}, nil})
		case *Button:
			v.net = true
			nw.res = append(nw.res, netRes{net(n, "1"), net(n, "2"), func() float64 {
				if v.Pressed {
					return closedOhms
				}
				return 0
			}, nil})
		case *Pot:
			v.net = true
			if v.Ohms <= 0 {
				v.Ohms = potOhms
			}
			share := func(f func(x float64) float64) func() float64 {
				return func() float64 { return math.Max(1, v.Ohms*f(math.Max(0, math.Min(1, v.Value)))) }
			}
			nw.res = append(nw.res,
				netRes{net(n, "1"), net(n, "W"), share(func(x float64) float64 { return x }), nil},
				netRes{net(n, "W"), net(n, "2"), share(func(x float64) float64 { return 1 - x }), nil})
		case *LED:
			v.net = true
			nw.leds = append(nw.leds, &netLED{led: v, a: net(n, "A"), k: net(n, "K")})
		case *Servo:
			v.Signal = nw.pinOn(net(n, "SIG"))
			nw.servos = append(nw.servos, netServo{v, net(n, "V+"), net(n, "GND")})
		case *Driver:
			v.in1, v.in2, v.en = nw.level(net(n, "IN1"), 0), nw.level(net(n, "IN2"), 0), nw.level(net(n, "EN"), 1)
			nw.drivers = append(nw.drivers, netDriver{v, n})
		}
		parts = append(parts, part)
		byName[n] = part
	}
	// drivers' supplies and motors, by what their terminals share a net with
	for _, d := range nw.drivers {
		vm, o1, o2 := net(d.name, "VM"), net(d.name, "OUT1"), net(d.name, "OUT2")
		for _, sp := range specs {
			switch q := byName[sp.Name].(type) {
			case *Battery:
				if net(sp.Name, "+") == vm {
					d.d.Supply = q
				}
			case *Motor:
				a, b := net(sp.Name, "1"), net(sp.Name, "2")
				if a == o1 && b == o2 || a == o2 && b == o1 {
					d.d.Motor, d.d.Reverse = q, a == o2
				}
			}
		}
	}
	return nw, parts, byName, nil
}

// Network solves the nets each step (the first of the simulation's parts).
type Network struct {
	board   *avr.Uno
	n       int
	gnd     int
	v5, v33 int
	netOf   map[string]int // "part.terminal" → net
	members [][]string     // each net's terminals
	degree  map[string]int // wires at each terminal
	pins    []netPin
	bats    []netBat
	res     []netRes
	leds    []*netLED
	servos  []netServo
	drivers []netDriver
	V       []float64 // each net's voltage, last solved
	anch    []bool    // nets with a path to ground
}

type netPin struct {
	pin avr.Pin
	net int
}

type netBat struct {
	b        *Battery
	pos, neg int
}

type netRes struct {
	a, b int
	ohms func() float64 // 0: open
	amps func(float64)  // told the current a → b (nil: not wanted)
}

type netLED struct {
	led  *LED
	a, k int
	on   bool
}

type netServo struct {
	s      *Servo
	vp, gn int
}

type netDriver struct {
	d    *Driver
	name string
}

// pinOn is the board pin on a net, NoPin if none.
func (nw *Network) pinOn(net int) avr.Pin {
	for _, p := range nw.pins {
		if p.net == net {
			return p.pin
		}
	}
	return NoPin
}

// level reads a driver input's net: a board pin's duty, else its voltage
// as a logic level, else (nothing on it) none.
func (nw *Network) level(net int, none float64) func(*Sim) float64 {
	pin := nw.pinOn(net)
	alone := len(nw.members[net]) == 1
	return func(s *Sim) float64 {
		switch {
		case pin != NoPin && s.Board.Output(pin):
			return s.Duty(pin)
		case alone:
			return none
		case nw.V != nil && nw.anch[net]:
			return b2f(nw.V[net] > logicHigh)
		}
		return none
	}
}

func (nw *Network) Label() string { return "wiring" }

// Wire solves the nets once before the board runs, so its inputs read
// right from the first instruction.
func (nw *Network) Wire(s *Sim) {
	for _, p := range nw.pins {
		s.Watch(p.pin)
	}
	nw.solve(s, true)
}

// Step solves the nets with the step's duties.
func (nw *Network) Step(s *Sim, _ float64) { nw.solve(s, false) }

// Volts is what an analogue input reads (its net's voltage).
func (nw *Network) Volts(ch int) (float64, bool) {
	ref := Board + "." + fmt.Sprintf("A%d", ch)
	i, ok := nw.netOf[ref]
	if !ok || nw.V == nil || len(nw.members[i]) == 1 {
		return 0, false
	}
	return nw.V[i], true
}

// VoltsAt is the voltage on a terminal's net, last solved (false: the
// terminal is wired to nothing, or nothing is solved yet).
func (nw *Network) VoltsAt(ref string) (float64, bool) {
	i, ok := nw.netOf[ref]
	if !ok || nw.V == nil || i >= len(nw.V) || len(nw.members[i]) == 1 {
		return 0, false
	}
	return nw.V[i], true
}

// NetOf is the net a terminal is on (-1: none).
func (nw *Network) NetOf(ref string) int {
	if i, ok := nw.netOf[ref]; ok {
		return i
	}
	return -1
}

// solve finds every net's voltage (start: the pins' levels, not a step's
// duty), then tells the parts and the board.
func (nw *Network) solve(s *Sim, start bool) {
	duty := func(p avr.Pin) float64 {
		if start {
			return b2f(s.Board.Level(p))
		}
		return s.Duty(p)
	}
	type cond struct {
		a, b int
		g    float64
		r    *netRes
	}
	var gs []cond
	for i := range nw.res {
		r := &nw.res[i]
		if o := r.ohms(); o > 0 {
			gs = append(gs, cond{r.a, r.b, 1 / o, r})
		}
	}
	type src struct {
		a, b int // from b to a (a the + side)
		v, g float64
	}
	// the sources, the board's output pins at the level given
	sources := func(level func(avr.Pin) float64) []src {
		var srcs []src
		for _, p := range nw.pins {
			switch {
			case s.Board.Output(p.pin):
				srcs = append(srcs, src{p.net, nw.gnd, 5 * level(p.pin), 1 / pinOhms})
			case s.Board.Pullup(p.pin):
				srcs = append(srcs, src{p.net, nw.gnd, 5, 1 / pullupOhms})
			}
		}
		if len(nw.members[nw.v5]) > 1 {
			srcs = append(srcs, src{nw.v5, nw.gnd, 5, 1 / railOhms})
		}
		if len(nw.members[nw.v33]) > 1 {
			srcs = append(srcs, src{nw.v33, nw.gnd, 3.3, 1 / railOhms})
		}
		for _, b := range nw.bats {
			srcs = append(srcs, src{b.pos, b.neg, b.b.Out(), 1 / math.Max(b.b.R, 0.01)})
		}
		return srcs
	}
	vfOf := func(l *netLED) float64 {
		if l.led.Vf > 0 {
			return l.led.Vf
		}
		return 2
	}
	// nodal analysis, the LEDs switched on and off until they settle; the
	// voltages and the LEDs' currents
	nodal := func(srcs []src) ([]float64, []float64) {
		on := make([]bool, len(nw.leds))
		for i, l := range nw.leds {
			on[i] = l.on
		}
		var V []float64
		n := nw.n
		for iter := 0; iter < 12; iter++ {
			G := make([][]float64, n)
			for i := range G {
				G[i] = make([]float64, n+1)
				G[i][i] = leak
			}
			conduct := func(a, b int, g float64) {
				G[a][a] += g
				G[b][b] += g
				G[a][b] -= g
				G[b][a] -= g
			}
			// an element whose current from a to b is g (Va − Vb − v)
			emf := func(a, b int, g, v float64) {
				conduct(a, b, g)
				G[a][n] += g * v
				G[b][n] -= g * v
			}
			for _, x := range srcs {
				emf(x.a, x.b, x.g, x.v)
			}
			for _, x := range gs {
				conduct(x.a, x.b, x.g)
			}
			for i, l := range nw.leds {
				if on[i] {
					emf(l.a, l.k, 1/ledOhms, vfOf(l))
				}
			}
			for j := 0; j <= n; j++ {
				G[nw.gnd][j] = 0
			}
			G[nw.gnd][nw.gnd] = 1
			V = solveLinear(G)
			changed := false
			for i, l := range nw.leds {
				d := V[l.a] - V[l.k]
				if on[i] && d < vfOf(l)-1e-9 || !on[i] && d > vfOf(l)+1e-9 {
					on[i] = !on[i]
					changed = true
				}
			}
			if !changed {
				break
			}
		}
		amps := make([]float64, len(nw.leds))
		for i, l := range nw.leds {
			if on[i] {
				amps[i] = math.Max(0, (V[l.a]-V[l.k]-vfOf(l))/ledOhms)
			}
			if amps[i] < 1e-7 { // a leak, not a current
				amps[i] = 0
			}
			l.on = on[i]
		}
		return V, amps
	}
	srcs := sources(duty)
	V, amps := nodal(srcs)
	nw.V = V
	// a PWM pin's LED: its current pin high and pin low, mixed by the duty
	// (the voltage's average before a diode's drop would light it far too
	// little)
	pwm := false
	for _, p := range nw.pins {
		if d := duty(p.pin); s.Board.Output(p.pin) && d > 1e-6 && d < 1-1e-6 {
			pwm = true
		}
	}
	if pwm && len(nw.leds) > 0 {
		_, hi := nodal(sources(func(p avr.Pin) float64 { return math.Ceil(duty(p) - 1e-6) }))
		_, lo := nodal(sources(func(p avr.Pin) float64 { return math.Floor(duty(p) + 1e-6) }))
		// the nets joined by passive parts (not through a source)
		comp := make([]int, nw.n)
		for i := range comp {
			comp[i] = i
		}
		var root func(int) int
		root = func(i int) int {
			for comp[i] != i {
				i = comp[i]
			}
			return i
		}
		rail := func(i int) bool { return i == nw.gnd || i == nw.v5 || i == nw.v33 }
		join := func(a, b int) {
			if !rail(a) && !rail(b) {
				comp[root(a)] = root(b)
			}
		}
		for _, x := range gs {
			join(x.a, x.b)
		}
		for _, l := range nw.leds {
			join(l.a, l.k)
		}
		for i := range comp {
			comp[i] = root(i)
		}
		for i, l := range nw.leds {
			sum, k := 0.0, 0
			for _, p := range nw.pins {
				if s.Board.Output(p.pin) && comp[p.net] == comp[l.a] {
					sum += duty(p.pin)
					k++
				}
			}
			if k > 0 {
				amps[i] = lo[i] + (hi[i]-lo[i])*sum/float64(k)
			}
		}
	}
	// the nets with a path to ground (through anything that conducts)
	adj := make([][]int, nw.n)
	link := func(a, b int) { adj[a] = append(adj[a], b); adj[b] = append(adj[b], a) }
	for _, x := range srcs {
		link(x.a, x.b)
	}
	for _, x := range gs {
		link(x.a, x.b)
	}
	for _, l := range nw.leds {
		if l.on {
			link(l.a, l.k)
		}
	}
	nw.anch = make([]bool, nw.n)
	queue := []int{nw.gnd}
	nw.anch[nw.gnd] = true
	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		for _, j := range adj[i] {
			if !nw.anch[j] {
				nw.anch[j] = true
				queue = append(queue, j)
			}
		}
	}
	// what the parts see
	for i, l := range nw.leds {
		l.led.Amps = amps[i]
		l.led.Brightness = math.Min(1, amps[i]/ledFull)
	}
	for _, x := range gs {
		if x.r.amps != nil {
			x.r.amps((V[x.a] - V[x.b]) * x.g)
		}
	}
	for _, sv := range nw.servos {
		sv.s.Unpowered = !nw.anch[sv.vp] || V[sv.vp]-V[sv.gn] < servoVolts
	}
	// the board's inputs read their nets
	for _, p := range nw.pins {
		if s.Board.Output(p.pin) {
			continue
		}
		if nw.anch[p.net] {
			s.Board.Drive(p.pin, V[p.net] > logicHigh)
		} else {
			s.Board.Release(p.pin)
		}
	}
}

// solveLinear solves the augmented system (each row n coefficients and
// the right-hand side) by Gaussian elimination with partial pivoting.
func solveLinear(G [][]float64) []float64 {
	n := len(G)
	for c := 0; c < n; c++ {
		p := c
		for r := c + 1; r < n; r++ {
			if math.Abs(G[r][c]) > math.Abs(G[p][c]) {
				p = r
			}
		}
		G[c], G[p] = G[p], G[c]
		if math.Abs(G[c][c]) < 1e-300 {
			continue
		}
		for r := c + 1; r < n; r++ {
			f := G[r][c] / G[c][c]
			if f == 0 {
				continue
			}
			for k := c; k <= n; k++ {
				G[r][k] -= f * G[c][k]
			}
		}
	}
	x := make([]float64, n)
	for r := n - 1; r >= 0; r-- {
		s := G[r][n]
		for k := r + 1; k < n; k++ {
			s -= G[r][k] * x[k]
		}
		if math.Abs(G[r][r]) > 1e-300 {
			x[r] = s / G[r][r]
		}
	}
	return x
}

// Issue is something wrong with a circuit's wiring: Err when it can't run
// as wired, else a warning.
type Issue struct {
	Text string
	Err  bool
}

// Check looks over a circuit's wiring for what is likely a mistake: a
// short, an LED with nothing to hold its current down, a motor or a
// servo's supply on a board pin, a part's terminal left unwired.
func Check(specs []Spec, wires []Wire) []Issue {
	nw, _, _, err := buildNetwork(specs, wires)
	if err != nil {
		return []Issue{{err.Error(), true}}
	}
	var out []Issue
	warn := func(f string, a ...any) { out = append(out, Issue{fmt.Sprintf(f, a...), false}) }
	bad := func(f string, a ...any) { out = append(out, Issue{fmt.Sprintf(f, a...), true}) }
	net := func(n, t string) int { return nw.netOf[n+"."+t] }
	has := func(i int, ref string) bool {
		for _, m := range nw.members[i] {
			if m == ref {
				return true
			}
		}
		return false
	}
	// sources of current on a net: the board's pins and rails, batteries
	stiff := func(i int) string {
		for _, m := range nw.members[i] {
			p, t, _ := Split(m)
			if p == Board && t != "VIN" {
				return m
			}
			if mm := Library[modelOf(specs, p)]; mm != nil && mm.Kind == "battery" {
				return m
			}
		}
		return ""
	}
	if nw.v5 == nw.gnd {
		bad("5V and GND are wired together: a short")
	}
	if nw.v33 == nw.gnd {
		bad("3V3 and GND are wired together: a short")
	}
	for i := range nw.members {
		var pins []string
		for _, m := range nw.members[i] {
			if p, t, _ := Split(m); p == Board && (t[0] == 'D' || t[0] == 'A') {
				pins = append(pins, t)
			}
		}
		if len(pins) > 1 {
			warn("%s are wired together: a short if more than one is an output", strings.Join(pins, ", "))
		}
		if len(pins) > 0 && (i == nw.gnd || i == nw.v5 || i == nw.v33) {
			warn("%s is wired straight to %s: a short if it is an output", strings.Join(pins, ", "), map[int]string{nw.gnd: "GND", nw.v5: "5V", nw.v33: "3V3"}[i])
		}
	}
	for _, sp := range specs {
		m := Library[sp.Model]
		n := sp.Name
		for _, t := range m.Terms {
			if nw.degree[n+"."+t] == 0 && !(m.Kind == "driver" && t == "EN") {
				warn("%s: %s isn't wired", n, t)
			}
		}
		switch m.Kind {
		case "battery":
			if net(n, "+") == net(n, "-") {
				bad("%s: + and − are wired together: a short", n)
			}
		case "led":
			a, k := stiff(net(n, "A")), stiff(net(n, "K"))
			if a != "" && k != "" {
				warn("%s has no resistor in series: %s to %s will burn it (and the pin)", n, a, k)
			}
		case "motor":
			for _, t := range m.Terms {
				for _, ref := range nw.members[net(n, t)] {
					if p, bt, _ := Split(ref); p == Board && (bt[0] == 'D' || bt[0] == 'A') {
						warn("%s is on board pin %s: a pin gives 40 mA at most — use a motor driver", n, bt)
					}
				}
			}
		case "servo":
			vp := net(n, "V+")
			for _, ref := range nw.members[vp] {
				if p, bt, _ := Split(ref); p == Board && (bt[0] == 'D' || bt[0] == 'A') {
					warn("%s: its V+ is on board pin %s, not a supply", n, bt)
				}
			}
			if !has(net(n, "GND"), Board+".GND") && nw.degree[n+".GND"] > 0 {
				warn("%s: its GND isn't the board's GND, so its signal has no reference", n)
			}
		case "driver":
			d := false
			for _, q := range specs {
				if Library[q.Model].Kind == "battery" && net(q.Name, "+") == net(n, "VM") {
					d = true
				}
			}
			if !d && nw.degree[n+".VM"] > 0 {
				warn("%s: no battery's + on VM, so its motor gets nothing", n)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Err && !out[j].Err })
	return out
}

func modelOf(specs []Spec, name string) string {
	for _, s := range specs {
		if s.Name == name {
			return s.Model
		}
	}
	return ""
}
