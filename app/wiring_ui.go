package app

// The wiring sheet (M21): the circuit drawn as pin-outs — the Uno and each
// part a box with its terminals down one side — and its wires, routed
// square between them. Drag a terminal onto another to wire them, a box by
// its title to move it, a selected wire's corner to move it; double-click
// a wire to give it a corner, right-click it for its colour; Delete takes
// the selected wire off. The checks show under the sheet as it is drawn,
// and while the study runs the LEDs glow, holding a button's box presses
// it, a click on a switch's turns it, and a terminal's tooltip gives its
// voltage.

import (
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strconv"
	"strings"

	imgui "github.com/AllenDang/cimgui-go/imgui"

	"forma/doc"
	"forma/elec"
	"forma/geom"
	"forma/ui"
)

const (
	shPitch  = 18.0 // between terminals
	shHead   = 26.0 // a box's title
	shPartW  = 132.0
	shBoardW = 150.0
	shStub   = 14.0 // a wire's straight run out of its terminal
	shGrid   = 10.0
	shGap    = shPitch // between the board's groups of pins
)

const icWiring = 0xf1e6 // plug

// wireSheet is the wiring sheet's state.
type wireSheet struct {
	open      bool
	zoom      float64
	off       geom.Vec2 // the sheet's origin from the canvas's corner (px)
	fit       bool
	drag      string // "", "pan", "wire", "part", "corner"
	from      string // the terminal a wire is drawn from, the box moved
	grab      geom.Vec2
	moved     bool
	sel       int // the selected wire + 1 (0: none)
	del       int // a wire to take out next frame + 1
	corner    int
	menu      int // the wire the context menu is for + 1
	rightOn   int // the wire under a right press + 1
	press     *elec.Button
	pos, size imgui.Vec2 // the canvas on screen
	mouse     geom.Vec2  // the cursor on the sheet
	notes     []string   // what a Wokwi project's import left out
	notesFor  int        // the circuit they are for
	user      bool       // panned or zoomed by hand: no fitting as the window resizes
	lastSize  imgui.Vec2
}

// shBox is a box on the sheet: the board or a part.
type shBox struct {
	name, model string
	at          geom.Vec2 // its top-left corner
	w, h        float64
	terms       []string
	ys          []float64 // each terminal's height from the top
	board       bool
}

func (b shBox) term(i int) (geom.Vec2, float64) {
	if b.board {
		return geom.V2(b.at.X+b.w, b.at.Y+b.ys[i]), 1
	}
	return geom.V2(b.at.X, b.at.Y+b.ys[i]), -1
}

func (b shBox) contains(p geom.Vec2) bool {
	return p.X >= b.at.X && p.X <= b.at.X+b.w && p.Y >= b.at.Y && p.Y <= b.at.Y+b.h
}

// boardOrder is the order the board's terminals are drawn in: the supplies,
// the digital pins, the analogue ones.
var boardOrder = []string{"5V", "3V3", "VIN", "GND",
	"D0", "D1", "D2", "D3", "D4", "D5", "D6", "D7", "D8", "D9", "D10", "D11", "D12", "D13",
	"A0", "A1", "A2", "A3", "A4", "A5"}

// sheetBoxes lays the circuit out: the board where it was put (else at the
// origin), each part where it was put, the others in columns to its right.
func sheetBoxes(c *doc.Circuit) []shBox {
	board := shBox{name: elec.Board, board: true, w: shBoardW, terms: boardOrder}
	if c.BoardAt != nil {
		board.at = *c.BoardAt
	}
	y := shHead + shPitch/2
	boardY := map[string]float64{}
	for i, t := range boardOrder {
		if i == 4 || i == 18 {
			y += shGap
		}
		board.ys = append(board.ys, y)
		boardY[t] = y
		y += shPitch
	}
	board.h = y - shPitch/2 + 8
	boxes := []shBox{board}

	// the parts not placed go down one column, in the order of the board
	// pins they are wired to (the supplies and ground left out), a part
	// wired only to other parts beside them, so few wires cross
	key := map[string]float64{}
	nets := map[string][]string{} // a part's terminals' nets
	for _, p := range c.Parts {
		key[p.Name] = math.Inf(1)
		for _, t := range elec.Terms(p.Model) {
			net := c.Net(p.Name + "." + t)
			supply := false
			for _, r := range net {
				switch r {
				case elec.Board + ".GND", elec.Board + ".5V", elec.Board + ".3V3", elec.Board + ".VIN":
					supply = true
				}
			}
			if !supply { // everything is beside everything on ground
				nets[p.Name] = append(nets[p.Name], net...)
			}
			for _, r := range net {
				if bp := boardPin(r); bp != "" {
					key[p.Name] = math.Min(key[p.Name], boardY[bp])
				}
			}
		}
	}
	for pass := 0; pass < 3; pass++ {
		for _, p := range c.Parts {
			for _, r := range nets[p.Name] {
				q, _, _ := elec.Split(r)
				if k, ok := key[q]; ok && q != p.Name && k+0.5 < key[p.Name] {
					key[p.Name] = k + 0.5
				}
			}
		}
	}
	order := make([]int, len(c.Parts))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return key[c.Parts[order[i]].Name] < key[c.Parts[order[j]].Name] })

	// the boxes are a whole number of pitches apart, half a pitch off the
	// board's, so no part's terminal is level with one of the board's (their
	// wires would run along each other)
	x0, cy := board.at.X+shBoardW+200, board.at.Y+shPitch/2
	parts := make([]shBox, len(c.Parts))
	for _, i := range order {
		p := c.Parts[i]
		b := shBox{name: p.Name, model: p.Model, w: shPartW, terms: elec.Terms(p.Model)}
		for k := range b.terms {
			b.ys = append(b.ys, shHead+shPitch*(float64(k)+0.5))
		}
		b.h = shHead + shPitch*float64(len(b.terms)) + 6
		if p.At != nil {
			b.at = *p.At
		} else {
			b.at = geom.V2(x0, cy)
			cy += b.h + 22 // h is 32 + n pitches: 3 pitches more
		}
		parts[i] = b
	}
	return append(boxes, parts...)
}

// boardPin is a board pin's name if ref is one ("D9", "A0"), else "".
func boardPin(ref string) string {
	p, t, ok := elec.Split(ref)
	if !ok || p != elec.Board || len(t) < 2 || (t[0] != 'D' && t[0] != 'A') {
		return ""
	}
	return t
}

// wireRoots names each wire's net (by its first terminal in order).
func wireRoots(c *doc.Circuit) []string {
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
	for _, w := range c.Wires {
		ra, rb := find(w.A), find(w.B)
		if ra != rb {
			if rb < ra {
				ra, rb = rb, ra
			}
			root[rb] = ra
		}
	}
	out := make([]string, len(c.Wires))
	for i, w := range c.Wires {
		out[i] = find(w.A)
	}
	return out
}

// shLane is the room between lanes, the vertical runs of wires.
const shLane = 8.0

// sheetRoutes routes every wire: those with corners through them, the
// others square, each net's vertical run in a lane of its own — clear of
// other nets' runs beside it and of the boxes — so wires don't lie on one
// another (wires of one net may share a lane: they meet anyway).
func sheetRoutes(c *doc.Circuit, boxes []shBox, terms map[string]shTerm) [][]geom.Vec2 {
	roots := wireRoots(c)
	routes := make([][]geom.Vec2, len(c.Wires))
	type run struct {
		lo, hi float64
		net    string
	}
	lanes := map[int][]run{} // by x / shLane
	horiz := map[int][]run{} // the runs out of the terminals, by y
	clearAlong := func(y, x0, x1 float64, net string) bool {
		lo, hi := math.Min(x0, x1), math.Max(x0, x1)
		for dy := -1; dy <= 1; dy++ {
			for _, r := range horiz[int(math.Round(y))+dy] {
				if r.net != net && r.lo <= hi+2 && lo <= r.hi+2 {
					return false
				}
			}
		}
		return true
	}
	free := func(x, lo, hi float64, net string, ta, tb shTerm) bool {
		if !clearAlong(ta.at.Y, ta.at.X, x, net) || !clearAlong(tb.at.Y, tb.at.X, x, net) {
			return false
		}
		for _, r := range lanes[int(math.Round(x/shLane))] {
			if r.net != net && r.lo <= hi+4 && lo <= r.hi+4 {
				return false
			}
		}
		for _, b := range boxes {
			if x > b.at.X-2 && x < b.at.X+b.w+2 && hi > b.at.Y && lo < b.at.Y+b.h {
				return false
			}
		}
		return true
	}
	type job struct {
		i                int
		lo, hi           float64
		from, step, stop float64
	}
	var jobs []job
	for i, w := range c.Wires {
		ta, oka := terms[w.A]
		tb, okb := terms[w.B]
		if !oka || !okb {
			continue
		}
		if len(w.Path) > 0 {
			routes[i] = shRoute(ta, tb, w.Path, math.NaN())
			continue
		}
		a1, b1 := ta.at.X+ta.dir*shStub, tb.at.X+tb.dir*shStub
		lo, hi := math.Min(ta.at.Y, tb.at.Y), math.Max(ta.at.Y, tb.at.Y)
		switch {
		case ta.dir == tb.dir && ta.dir < 0:
			x := math.Floor(math.Min(a1, b1)/shLane) * shLane
			jobs = append(jobs, job{i, lo, hi, x, -shLane, x - 60*shLane})
		case ta.dir == tb.dir:
			x := math.Ceil(math.Max(a1, b1)/shLane) * shLane
			jobs = append(jobs, job{i, lo, hi, x, shLane, x + 60*shLane})
		case (b1-a1)*ta.dir >= 0:
			// from the side facing left (the parts') toward the board
			l, r := math.Min(a1, b1), math.Max(a1, b1)
			x := math.Floor(r/shLane) * shLane
			jobs = append(jobs, job{i, lo, hi, x, -shLane, math.Ceil(l/shLane) * shLane})
		default:
			routes[i] = shRoute(ta, tb, nil, math.NaN())
		}
	}
	// the shortest runs first, nearest their parts
	sort.SliceStable(jobs, func(x, y int) bool { return jobs[x].hi-jobs[x].lo < jobs[y].hi-jobs[y].lo })
	for _, j := range jobs {
		net := roots[j.i]
		w := c.Wires[j.i]
		ta, tb := terms[w.A], terms[w.B]
		x := j.from
		for {
			if free(x, j.lo, j.hi, net, ta, tb) {
				break
			}
			if (j.step < 0 && x+j.step < j.stop) || (j.step > 0 && x+j.step > j.stop) {
				x = j.from // nowhere clear: where it wanted to be
				break
			}
			x += j.step
		}
		k := int(math.Round(x / shLane))
		lanes[k] = append(lanes[k], run{j.lo, j.hi, net})
		for _, t := range []shTerm{ta, tb} {
			y := int(math.Round(t.at.Y))
			horiz[y] = append(horiz[y], run{math.Min(t.at.X, x), math.Max(t.at.X, x), net})
		}
		routes[j.i] = shRoute(ta, tb, nil, x)
	}
	return routes
}

// shTerm is where a terminal is on the sheet and which way it faces.
type shTerm struct {
	at  geom.Vec2
	dir float64
}

func sheetTerms(boxes []shBox) map[string]shTerm {
	m := map[string]shTerm{}
	for _, b := range boxes {
		for i, t := range b.terms {
			p, d := b.term(i)
			m[b.name+"."+t] = shTerm{p, d}
		}
	}
	return m
}

// shRoute is a wire's path: straight out of each terminal, square between,
// through its corners.
func shRoute(a, b shTerm, corners []geom.Vec2, lane float64) []geom.Vec2 {
	a1 := geom.V2(a.at.X+a.dir*shStub, a.at.Y)
	b1 := geom.V2(b.at.X+b.dir*shStub, b.at.Y)
	var pts []geom.Vec2
	switch {
	case len(corners) > 0:
		pts = []geom.Vec2{a.at, a1}
		prev := a1
		for _, c := range corners {
			pts = append(pts, geom.V2(c.X, prev.Y), c)
			prev = c
		}
		pts = append(pts, geom.V2(b1.X, prev.Y), b1, b.at)
	case !math.IsNaN(lane):
		pts = []geom.Vec2{a.at, geom.V2(lane, a.at.Y), geom.V2(lane, b.at.Y), b.at}
	case a.dir == b.dir:
		x := math.Min(a1.X, b1.X)
		if a.dir > 0 {
			x = math.Max(a1.X, b1.X)
		}
		pts = []geom.Vec2{a.at, geom.V2(x, a.at.Y), geom.V2(x, b.at.Y), b.at}
	case (b1.X-a1.X)*a.dir >= 0:
		mx := math.Round((a1.X+b1.X)/2/shGrid) * shGrid
		pts = []geom.Vec2{a.at, geom.V2(mx, a.at.Y), geom.V2(mx, b.at.Y), b.at}
	default:
		my := (a.at.Y + b.at.Y) / 2
		pts = []geom.Vec2{a.at, a1, geom.V2(a1.X, my), geom.V2(b1.X, my), b1, b.at}
	}
	out := pts[:1]
	for _, p := range pts[1:] {
		if p.Sub(out[len(out)-1]).Len() > 1e-6 {
			out = append(out, p)
		}
	}
	return out
}

func routeLen(pts []geom.Vec2) float64 {
	l := 0.0
	for i := 1; i < len(pts); i++ {
		l += pts[i].Sub(pts[i-1]).Len()
	}
	return l
}

// wireColours are the names a wire's colour may take (Wokwi's among them).
var wireColours = map[string][3]float64{
	"red": {0.85, 0.12, 0.1}, "black": {0.08, 0.08, 0.09}, "blue": {0.15, 0.38, 0.88},
	"green": {0.1, 0.58, 0.22}, "limegreen": {0.3, 0.8, 0.2}, "yellow": {0.9, 0.75, 0.05},
	"gold": {0.85, 0.65, 0.1}, "orange": {0.95, 0.5, 0.08}, "white": {0.97, 0.97, 0.97},
	"purple": {0.55, 0.25, 0.75}, "violet": {0.6, 0.35, 0.85}, "magenta": {0.85, 0.2, 0.7},
	"pink": {0.95, 0.5, 0.7}, "cyan": {0.05, 0.65, 0.75}, "brown": {0.5, 0.3, 0.15},
	"gray": {0.55, 0.55, 0.57}, "grey": {0.55, 0.55, 0.57},
}

// colourMenu is the colours the wire menu offers, in its order.
var colourMenu = []string{"red", "black", "blue", "green", "yellow", "orange", "white", "purple", "gray", "brown"}

// parseColour reads a wire's colour ("red", "#3080ff").
func parseColour(s string) ([3]float64, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if c, ok := wireColours[s]; ok {
		return c, true
	}
	if len(s) == 7 && s[0] == '#' {
		if v, err := strconv.ParseUint(s[1:], 16, 32); err == nil {
			return [3]float64{float64(v>>16&255) / 255, float64(v>>8&255) / 255, float64(v&255) / 255}, true
		}
	}
	return [3]float64{}, false
}

// netColours is the colour each wire takes when it was given none: red on
// a supply's net, black on ground's, one of a few others by net.
func netColours(c *doc.Circuit) [][3]float64 {
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
	for _, w := range c.Wires {
		ra, rb := find(w.A), find(w.B)
		if ra != rb {
			if rb < ra {
				ra, rb = rb, ra
			}
			root[rb] = ra
		}
	}
	kind := map[string]string{}
	for _, p := range c.Parts {
		if m := elec.Library[p.Model]; m != nil {
			kind[p.Name] = m.Kind
		}
	}
	role := map[string]string{} // net root → "supply" / "ground"
	for r := range root {
		part, t, _ := elec.Split(r)
		switch {
		case part == elec.Board && (t == "5V" || t == "3V3" || t == "VIN"), kind[part] == "battery" && t == "+":
			if role[find(r)] != "ground" {
				role[find(r)] = "supply"
			}
		case part == elec.Board && t == "GND", kind[part] == "battery" && t == "-":
			role[find(r)] = "ground"
		}
	}
	palette := [][3]float64{{0.1, 0.58, 0.22}, {0.15, 0.38, 0.88}, {0.95, 0.5, 0.08}, {0.55, 0.25, 0.75}, {0.8, 0.62, 0.02}, {0.05, 0.6, 0.7}, {0.5, 0.3, 0.15}}
	out := make([][3]float64, len(c.Wires))
	for i, w := range c.Wires {
		if col, ok := parseColour(w.Colour); ok {
			out[i] = col
			continue
		}
		r := find(w.A)
		switch role[r] {
		case "supply":
			out[i] = wireColours["red"]
		case "ground":
			out[i] = wireColours["black"]
		default:
			h := fnv.New32a()
			h.Write([]byte(r))
			out[i] = palette[h.Sum32()%uint32(len(palette))]
		}
	}
	return out
}

func (sh *wireSheet) toScreen(p geom.Vec2) imgui.Vec2 {
	return imgui.Vec2{X: sh.pos.X + float32(sh.off.X+p.X*sh.zoom), Y: sh.pos.Y + float32(sh.off.Y+p.Y*sh.zoom)}
}

func (sh *wireSheet) toSheet(s imgui.Vec2) geom.Vec2 {
	return geom.V2((float64(s.X-sh.pos.X)-sh.off.X)/sh.zoom, (float64(s.Y-sh.pos.Y)-sh.off.Y)/sh.zoom)
}

func snapSheet(p geom.Vec2) geom.Vec2 {
	return geom.V2(math.Round(p.X/shGrid)*shGrid, math.Round(p.Y/shGrid)*shGrid)
}

// fitSheet frames every box.
func (sh *wireSheet) fitSheet(boxes []shBox) {
	lo, hi := geom.V2(math.Inf(1), math.Inf(1)), geom.V2(math.Inf(-1), math.Inf(-1))
	for _, b := range boxes {
		lo = geom.V2(math.Min(lo.X, b.at.X-40), math.Min(lo.Y, b.at.Y-20))
		hi = geom.V2(math.Max(hi.X, b.at.X+b.w+40), math.Max(hi.Y, b.at.Y+b.h+20))
	}
	w, h := hi.X-lo.X, hi.Y-lo.Y
	z := math.Min(float64(sh.size.X)/w, float64(sh.size.Y)/h)
	sh.zoom = math.Max(0.3, math.Min(z, 1.6))
	sh.off = geom.V2(float64(sh.size.X)/2-(lo.X+w/2)*sh.zoom, float64(sh.size.Y)/2-(lo.Y+h/2)*sh.zoom)
}

// sheetNetwork is the running study's network for the circuit (nil if none).
func (a *App) sheetNetwork(f *doc.Feature) (*doc.Study, *elec.Network) {
	st := a.elec.study
	if st == nil || st.F != f || st.Sim == nil {
		return nil, nil
	}
	for _, p := range st.Sim.Parts {
		if nw, ok := p.(*elec.Network); ok {
			return st, nw
		}
	}
	return st, nil
}

// wiringWindow is the wiring sheet's window.
func (a *App) wiringWindow() {
	sh := &a.elec.sheet
	if !sh.open {
		return
	}
	vp := imgui.MainViewport()
	imgui.SetNextWindowPosV(imgui.Vec2{X: vp.Pos().X + 285, Y: vp.Pos().Y + 110}, imgui.CondAppearing, imgui.Vec2{})
	imgui.SetNextWindowSizeV(imgui.Vec2{X: min(1000, vp.Size().X-720), Y: min(680, vp.Size().Y-150)}, imgui.CondAppearing)
	open := true
	if imgui.BeginV(ui.Icon(icWiring, "Wiring")+"###wiring", &open, imgui.WindowFlagsNoSavedSettings|imgui.WindowFlagsNoScrollbar|imgui.WindowFlagsNoScrollWithMouse) {
		a.wiringBody()
	}
	imgui.End()
	if !open {
		sh.open = false
	}
}

func (a *App) wiringBody() {
	sh := &a.elec.sheet
	f := a.elecCircuit()
	if f == nil {
		imgui.TextWrapped("No circuit yet: add one in the Electronics window.")
		return
	}
	c := f.Circuit
	if i := sh.del - 1; i >= 0 {
		sh.del = 0
		if i < len(c.Wires) {
			c.Wires = append(c.Wires[:i], c.Wires[i+1:]...)
			a.elecEdit()
		}
	}
	boxes := sheetBoxes(c)
	terms := sheetTerms(boxes)

	if imgui.Button("Fit") {
		sh.fit = true
	}
	a.markItem("btn:wiring.fit")
	imgui.SameLine()
	if imgui.Button("Lay out again") {
		c.BoardAt = nil
		for i := range c.Parts {
			c.Parts[i].At = nil
		}
		for i := range c.Wires {
			c.Wires[i].Path = nil
		}
		sh.fit = true
		a.doc.Commit()
		return
	}
	imgui.SetItemTooltip("Put every box back in its column and every wire back on its square route")
	imgui.SameLine()
	imgui.TextDisabled("Drag any terminal onto another, a part's or the board's, to wire them; drag a box by its title; double-click a wire for a corner, right-click for its colour; Delete takes the selected wire off.")

	issues := c.Issues()
	issuesH := float32(26 + 20*min(len(issues), 4))
	if sh.notesFor == f.ID && len(sh.notes) > 0 {
		issuesH += 22
	}
	avail := imgui.ContentRegionAvail()
	sh.pos = imgui.CursorScreenPos()
	sh.size = imgui.Vec2{X: avail.X, Y: max(120, avail.Y-issuesH)}
	if sh.zoom == 0 || sh.fit || !sh.user && sh.size != sh.lastSize {
		sh.fitSheet(boxes)
		if sh.fit {
			sh.user = false
		}
		sh.fit = false
	}
	sh.lastSize = sh.size
	imgui.InvisibleButtonV("##wsheet", sh.size, imgui.ButtonFlagsMouseButtonLeft|imgui.ButtonFlagsMouseButtonRight|imgui.ButtonFlagsMouseButtonMiddle)
	hovered := imgui.IsItemHovered()
	sh.mouse = sh.toSheet(imgui.MousePos())
	st, nw := a.sheetNetwork(f)

	// what is under the cursor
	px := 1 / sh.zoom
	hotTerm := ""
	best := 8 * px
	for ref, t := range terms {
		if d := t.at.Sub(sh.mouse).Len(); d < best {
			hotTerm, best = ref, d
		}
	}
	routes := sheetRoutes(c, boxes, terms)
	hotWire := 0
	if hotTerm == "" {
		best = 5 * px
		for i, r := range routes {
			for k := 1; k < len(r); k++ {
				if d := segDist(sh.mouse, r[k-1], r[k]); d < best {
					hotWire, best = i+1, d
				}
			}
		}
	}
	hotCorner := -1
	if sh.sel > 0 && sh.sel <= len(c.Wires) {
		for k, p := range c.Wires[sh.sel-1].Path {
			if p.Sub(sh.mouse).Len() < 6*px {
				hotCorner = k
			}
		}
	}
	hotBox := -1
	if hotTerm == "" && hotCorner < 0 {
		for i := len(boxes) - 1; i >= 0; i-- {
			if boxes[i].contains(sh.mouse) {
				hotBox = i
				break
			}
		}
	}

	a.wiringInput(f, boxes, terms, routes, hovered, hotTerm, hotWire, hotCorner, hotBox)
	if sh.sel > len(c.Wires) {
		sh.sel = 0
	}

	// the sheet
	dl := imgui.WindowDrawList()
	end := imgui.Vec2{X: sh.pos.X + sh.size.X, Y: sh.pos.Y + sh.size.Y}
	dl.PushClipRect(sh.pos, end)
	dl.AddRectFilled(sh.pos, end, rgba(0.93, 0.93, 0.9, 1))
	if step := shGrid * 5 * sh.zoom; step >= 12 {
		o := sh.toScreen(geom.V2(0, 0))
		x0 := float64(o.X) - math.Ceil((float64(o.X-sh.pos.X))/step)*step
		for x := x0; x < float64(end.X); x += step {
			dl.AddLine(imgui.Vec2{X: float32(x), Y: sh.pos.Y}, imgui.Vec2{X: float32(x), Y: end.Y}, rgba(0, 0, 0, 0.05))
		}
		y0 := float64(o.Y) - math.Ceil((float64(o.Y-sh.pos.Y))/step)*step
		for y := y0; y < float64(end.Y); y += step {
			dl.AddLine(imgui.Vec2{X: sh.pos.X, Y: float32(y)}, imgui.Vec2{X: end.X, Y: float32(y)}, rgba(0, 0, 0, 0.05))
		}
	}
	font := imgui.CurrentFont()
	fs := float32(math.Max(9, math.Min(18, 14*sh.zoom)))
	text := func(p geom.Vec2, col uint32, s string, size float32) {
		dl.AddTextFontPtr(font, size, sh.toScreen(p), col, s)
	}
	textW := func(s string, size float32) float64 {
		return float64(font.CalcTextSizeA(size, math.MaxFloat32, 0, s).X) / sh.zoom
	}

	// the net under the cursor, lit along its terminals
	hotNet := map[string]bool{}
	if hotTerm != "" {
		for _, r := range c.Net(hotTerm) {
			hotNet[r] = true
		}
	} else if hotWire > 0 {
		for _, r := range c.Net(c.Wires[hotWire-1].A) {
			hotNet[r] = true
		}
	}

	// boxes
	for _, b := range boxes {
		p0, p1 := sh.toScreen(b.at), sh.toScreen(geom.V2(b.at.X+b.w, b.at.Y+b.h))
		hd := sh.toScreen(geom.V2(b.at.X+b.w, b.at.Y+shHead))
		r := float32(4 * sh.zoom)
		dl.AddRectFilledV(imgui.Vec2{X: p0.X + 3, Y: p0.Y + 3}, imgui.Vec2{X: p1.X + 3, Y: p1.Y + 3}, rgba(0, 0, 0, 0.18), r, 0)
		title := b.name
		sub := ""
		head, body, ink := rgba(0.0, 0.42, 0.52, 1), rgba(0.0, 0.5, 0.6, 1), rgba(1, 1, 1, 1)
		if b.board {
			title, sub = "Arduino Uno", "uno"
		} else {
			head, body, ink = rgba(0.28, 0.3, 0.34, 1), rgba(1, 1, 1, 1), rgba(0.1, 0.1, 0.12, 1)
			if m := elec.Library[b.model]; m != nil {
				sub = m.Label
			} else {
				sub = b.model + " (not in the library)"
				head = rgba(0.7, 0.15, 0.1, 1)
			}
		}
		dl.AddRectFilledV(p0, p1, body, r, 0)
		dl.AddRectFilledV(p0, hd, head, r, imgui.DrawFlagsRoundCornersTop)
		dl.AddRectV(p0, p1, rgba(0, 0, 0, 0.45), r, 1, 0)
		text(geom.V2(b.at.X+6, b.at.Y+3), rgba(1, 1, 1, 1), title, fs)
		if sh.zoom > 0.55 {
			text(geom.V2(b.at.X+6, b.at.Y+3+float64(fs)/sh.zoom*0.95), rgba(1, 1, 1, 0.6), clip(sub, 24), fs*0.72)
		}
		// live state
		if st != nil {
			switch q := st.Parts[b.name].(type) {
			case *elec.LED:
				v := ledLook(q.Brightness)
				c := sh.toScreen(geom.V2(b.at.X+b.w-14, b.at.Y+shHead/2))
				if v > 0.02 {
					dl.AddCircleFilled(c, float32(13*sh.zoom), rgba(1, 0.3, 0.1, 0.35*v))
				}
				dl.AddCircleFilled(c, float32(7*sh.zoom), rgba(0.35+0.65*v, 0.08+0.25*v, 0.06, 1))
			case *elec.Button:
				col := rgba(0.6, 0.6, 0.6, 1)
				if q.Pressed {
					col = rgba(0.95, 0.55, 0.1, 1)
				}
				dl.AddCircleFilled(sh.toScreen(geom.V2(b.at.X+b.w-14, b.at.Y+shHead/2)), float32(6*sh.zoom), col)
			case *elec.Switch:
				s := "off"
				if q.On {
					s = "on"
				}
				text(geom.V2(b.at.X+b.w-6-textW(s, fs*0.8), b.at.Y+6), rgba(1, 0.8, 0.3, 1), s, fs*0.8)
			}
		}
		for i, t := range b.terms {
			at, dir := b.term(i)
			ref := b.name + "." + t
			col := rgba(0.75, 0.6, 0.15, 1)
			rad := float32(3.5 * sh.zoom)
			if hotNet[ref] {
				col = rgba(0.95, 0.5, 0.05, 1)
			}
			if ref == hotTerm || ref == sh.from && sh.drag == "wire" {
				rad *= 1.6
			}
			dl.AddCircleFilled(sh.toScreen(at), rad, col)
			dl.AddCircleV(sh.toScreen(at), rad, rgba(0, 0, 0, 0.5), 0, 1)
			if sh.zoom > 0.45 {
				lx := at.X + 7
				if dir > 0 {
					lx = at.X - 7 - textW(t, fs*0.85)
				}
				text(geom.V2(lx, at.Y-float64(fs)*0.85/2/sh.zoom), ink, t, fs*0.85)
			}
			a.btnRects["term:"+ref] = sh.toScreen(at)
		}
		a.btnRects["box:"+b.name] = sh.toScreen(geom.V2(b.at.X+b.w/2, b.at.Y+shHead/2))
	}

	// wires
	cols := netColours(c)
	th := float32(math.Max(1.5, 2.6*sh.zoom))
	for i, r := range routes {
		if i >= len(c.Wires) || i >= len(cols) {
			break
		}
		if len(r) < 2 {
			continue
		}
		pts := make([]imgui.Vec2, len(r))
		for k, p := range r {
			pts[k] = sh.toScreen(p)
		}
		col := cols[i]
		if i+1 == sh.sel || i+1 == hotWire {
			dl.AddPolylineV(&pts[0], int32(len(pts)), rgba(0.2, 0.55, 1, 0.45), th+5, 0)
		}
		if col[0]+col[1]+col[2] > 2.6 { // a white wire: an outline
			dl.AddPolylineV(&pts[0], int32(len(pts)), rgba(0.4, 0.4, 0.4, 1), th+1.5, 0)
		}
		dl.AddPolylineV(&pts[0], int32(len(pts)), rgba(col[0], col[1], col[2], 1), th, 0)
		mid := len(r) / 2
		w := c.Wires[i]
		a.btnRects["wire:"+w.A+"~"+w.B] = sh.toScreen(r[mid-1].Add(r[mid]).Scale(0.5))
		if i+1 == sh.sel {
			for k, p := range w.Path {
				hc := rgba(1, 1, 1, 1)
				if k == hotCorner {
					hc = rgba(1, 0.6, 0.1, 1)
				}
				dl.AddRectFilled(imgui.Vec2{X: sh.toScreen(p).X - 4, Y: sh.toScreen(p).Y - 4}, imgui.Vec2{X: sh.toScreen(p).X + 4, Y: sh.toScreen(p).Y + 4}, hc)
				dl.AddRect(imgui.Vec2{X: sh.toScreen(p).X - 4, Y: sh.toScreen(p).Y - 4}, imgui.Vec2{X: sh.toScreen(p).X + 4, Y: sh.toScreen(p).Y + 4}, rgba(0, 0, 0, 0.8))
			}
		}
	}
	// the wire being drawn
	if sh.drag == "wire" {
		if t, ok := terms[sh.from]; ok {
			to := imgui.MousePos()
			if ht, ok := terms[hotTerm]; ok && hotTerm != sh.from {
				to = sh.toScreen(ht.at)
			}
			pts := []imgui.Vec2{sh.toScreen(t.at), to}
			dl.AddPolylineV(&pts[0], 2, rgba(0.2, 0.5, 1, 0.9), th, 0)
		}
	}
	if len(c.Parts) == 0 {
		text(geom.V2(boxes[0].at.X+boxes[0].w+40, boxes[0].at.Y+20), rgba(0, 0, 0, 0.45), "Add parts in the Electronics window, then wire them here.", fs)
	}
	dl.PopClipRect()

	// a terminal's tooltip: its net and, while the study runs, its voltage
	if hovered && sh.drag == "" && hotTerm != "" {
		imgui.BeginTooltip()
		imgui.TextUnformatted(hotTerm)
		if others := c.Net(hotTerm); len(others) > 1 {
			var o []string
			for _, r := range others {
				if r != hotTerm {
					o = append(o, r)
				}
			}
			imgui.TextDisabled("wired to " + strings.Join(o, ", "))
		} else {
			imgui.TextDisabled("not wired")
		}
		if nw != nil {
			if v, ok := nw.VoltsAt(hotTerm); ok {
				imgui.Text(fmt.Sprintf("%.2f V", v))
			}
		}
		imgui.EndTooltip()
	}

	// the wire menu
	if imgui.BeginPopup("##wiremenu") {
		if i := sh.menu - 1; i >= 0 && i < len(c.Wires) {
			w := &c.Wires[i]
			imgui.TextDisabled(w.A + " – " + w.B)
			for _, n := range colourMenu {
				col := wireColours[n]
				imgui.ColorButtonV("##c"+n, imgui.Vec4{X: float32(col[0]), Y: float32(col[1]), Z: float32(col[2]), W: 1}, imgui.ColorEditFlagsNoTooltip, imgui.Vec2{X: 14, Y: 14})
				imgui.SameLine()
				if imgui.SelectableBoolV(n, w.Colour == n, 0, imgui.Vec2{}) {
					w.Colour = n
					a.doc.Commit()
				}
			}
			if imgui.SelectableBoolV("by what it carries", w.Colour == "", 0, imgui.Vec2{}) {
				w.Colour = ""
				a.doc.Commit()
			}
			imgui.Separator()
			if len(w.Path) > 0 && imgui.SelectableBool("Straighten") {
				w.Path = nil
				a.doc.Commit()
			}
			del := imgui.SelectableBool("Delete wire")
			a.markItem("menu:delete wire")
			if del {
				sh.del, sh.sel = i+1, 0
			}
		}
		imgui.EndPopup()
	}

	// the checks
	imgui.SetCursorScreenPos(imgui.Vec2{X: sh.pos.X, Y: sh.pos.Y + sh.size.Y + 4})
	if imgui.BeginChildStrV("##issues", imgui.Vec2{X: 0, Y: 0}, 0, 0) {
		if sh.notesFor == f.ID && len(sh.notes) > 0 {
			if imgui.TreeNodeExStrV(ui.Icon(0xf05a, fmt.Sprintf("From Wokwi: %d %s left out", len(sh.notes), plural(len(sh.notes), "thing", "things")))+"###wnotes", 0) {
				for _, n := range sh.notes {
					imgui.BulletText(n)
				}
				imgui.TreePop()
			}
		}
		if len(issues) == 0 {
			imgui.TextColored(imgui.Vec4{X: 0.45, Y: 0.8, Z: 0.45, W: 1}, ui.Icon(0xf00c, "No problems in the wiring"))
		}
		for _, is := range issues {
			if is.Err {
				imgui.TextColored(colErr, ui.Icon(0xf06a, is.Text))
			} else {
				imgui.TextColored(colWarn, ui.Icon(0xf071, is.Text))
			}
		}
	}
	imgui.EndChild()
}

// clip shortens s to n runes.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "."
}

// wiringInput: the wheel zooms about the cursor, right or middle drag pans,
// left drags wire, move boxes and corners.
func (a *App) wiringInput(f *doc.Feature, boxes []shBox, terms map[string]shTerm, routes [][]geom.Vec2,
	hovered bool, hotTerm string, hotWire, hotCorner, hotBox int) {
	sh := &a.elec.sheet
	c := f.Circuit
	io := imgui.CurrentIO()
	st, _ := a.sheetNetwork(f)
	if hovered {
		if w := io.MouseWheel(); w != 0 {
			at := sh.mouse
			sh.user = true
			sh.zoom = math.Max(0.25, math.Min(sh.zoom*math.Pow(1.15, float64(w)), 4))
			m := imgui.MousePos()
			sh.off = geom.V2(float64(m.X-sh.pos.X)-at.X*sh.zoom, float64(m.Y-sh.pos.Y)-at.Y*sh.zoom)
		}
		if imgui.IsMouseClickedBool(imgui.MouseButtonMiddle) || imgui.IsMouseClickedBool(imgui.MouseButtonRight) {
			sh.drag, sh.moved, sh.rightOn = "pan", false, 0
			if imgui.IsMouseClickedBool(imgui.MouseButtonRight) {
				sh.rightOn = hotWire
			}
		}
		if imgui.IsMouseDoubleClicked(imgui.MouseButtonLeft) && hotWire > 0 && hotTerm == "" {
			// a corner where the wire was clicked, between the corners it
			// keeps the route shortest between
			i := hotWire - 1
			w := &c.Wires[i]
			p := snapSheet(sh.mouse)
			bestK, bestL := 0, math.Inf(1)
			for k := 0; k <= len(w.Path); k++ {
				path := append(append(append([]geom.Vec2{}, w.Path[:k]...), p), w.Path[k:]...)
				if l := routeLen(shRoute(terms[w.A], terms[w.B], path, math.NaN())); l < bestL {
					bestK, bestL = k, l
				}
			}
			w.Path = append(append(append([]geom.Vec2{}, w.Path[:bestK]...), p), w.Path[bestK:]...)
			sh.sel, sh.drag = hotWire, ""
			a.doc.Commit()
			return
		}
		if imgui.IsMouseClickedBool(imgui.MouseButtonLeft) {
			sh.moved = false
			switch {
			case hotTerm != "":
				sh.drag, sh.from = "wire", hotTerm
			case hotCorner >= 0:
				sh.drag, sh.corner = "corner", hotCorner
			case hotBox >= 0:
				b := boxes[hotBox]
				sh.drag, sh.from, sh.grab = "part", b.name, b.at.Sub(sh.mouse)
				if st != nil {
					if q, ok := st.Parts[b.name].(*elec.Button); ok {
						q.Pressed, sh.press = true, q
					}
				}
			case hotWire > 0:
				sh.sel = hotWire
			default:
				sh.sel = 0
			}
		}
	}
	if sh.sel > 0 && (imgui.IsWindowFocused() || hovered) && (imgui.IsKeyPressedBool(imgui.KeyDelete) || imgui.IsKeyPressedBool(imgui.KeyBackspace)) && sh.sel <= len(c.Wires) {
		// taken out at the start of the next frame: this one is drawn from
		// routes worked out with the wire still there
		sh.del, sh.sel = sh.sel, 0
		return
	}
	switch sh.drag {
	case "pan":
		d := io.MouseDelta()
		if d.X != 0 || d.Y != 0 {
			sh.moved = true
			sh.user = true
		}
		sh.off = sh.off.Add(geom.V2(float64(d.X), float64(d.Y)))
		if !imgui.IsMouseDown(imgui.MouseButtonMiddle) && !imgui.IsMouseDown(imgui.MouseButtonRight) {
			if !sh.moved && sh.rightOn > 0 {
				sh.menu, sh.sel = sh.rightOn, sh.rightOn
				imgui.OpenPopupStr("##wiremenu")
			}
			sh.drag = ""
		}
	case "wire":
		if !imgui.IsMouseDown(imgui.MouseButtonLeft) {
			sh.drag = ""
			if hotTerm != "" && hotTerm != sh.from && c.Connect(sh.from, hotTerm) {
				sh.sel = len(c.Wires)
				a.elecEdit()
			}
		}
	case "part":
		if imgui.IsMouseDown(imgui.MouseButtonLeft) {
			if !sh.moved && sh.mouse.Add(sh.grab).Sub(boxAt(boxes, sh.from)).Len()*sh.zoom > 3 {
				sh.moved = true
			}
			if sh.moved {
				p := snapSheet(sh.mouse.Add(sh.grab))
				// the parts laid out stay where they were drawn
				for i := range c.Parts {
					if c.Parts[i].At == nil {
						at := boxAt(boxes, c.Parts[i].Name)
						c.Parts[i].At = &at
					}
					if c.Parts[i].Name == sh.from {
						c.Parts[i].At = &p
					}
				}
				if sh.from == elec.Board {
					c.BoardAt = &p
				}
			}
			return
		}
		sh.drag = ""
		if sh.press != nil {
			sh.press.Pressed, sh.press = false, nil
		}
		if sh.moved {
			a.doc.Commit()
		} else if st != nil {
			if q, ok := st.Parts[sh.from].(*elec.Switch); ok {
				q.On = !q.On
			}
		}
	case "corner":
		w := sh.sel - 1
		if w < 0 || w >= len(c.Wires) || sh.corner >= len(c.Wires[w].Path) {
			sh.drag = ""
			return
		}
		if imgui.IsMouseDown(imgui.MouseButtonLeft) {
			c.Wires[w].Path[sh.corner] = snapSheet(sh.mouse)
			sh.moved = true
			return
		}
		sh.drag = ""
		if sh.moved {
			a.doc.Commit()
		}
	}
}

func boxAt(boxes []shBox, name string) geom.Vec2 {
	for _, b := range boxes {
		if b.name == name {
			return b.at
		}
	}
	return geom.Vec2{}
}
