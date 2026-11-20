package app

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	imgui "github.com/AllenDang/cimgui-go/imgui"

	"forma/brep"
	"forma/doc"
	"forma/geom"
	"forma/sketch"
)

// The transform box round a sketch selection (Select tool): drag inside it
// to move the selection, a side handle to stretch it along one axis, a
// corner handle to scale it evenly about the opposite corner. Snapped like
// every drag (Ctrl free, Shift fine). Dimensions and fixed points within
// the selection take the new sizes, so the solver keeps them.

type skXform struct {
	on     bool
	handle int // 0 inside (move), 1–4 corners, 5–8 sides
	lo, hi geom.Vec2
	start  geom.Vec2
	pts    map[int]geom.Vec2 // the moved points where they were
	radii  map[int]float64   // circles' radius parameters where they were
	dx, dy fineDrag
	label  string
}

// selected is the sketch entities selected (curves and points of this
// sketch).
func (sm *sketchMode) selected(a *App) []int {
	var out []int
	for _, r := range a.sel {
		if r.Item == sm.item.ID && (r.Kind == KindSkCurve || r.Kind == KindSkPoint) {
			if sm.item.S.Ent(r.Local) != nil {
				out = append(out, r.Local)
			}
		}
	}
	return out
}

// selBox is the selection's extent in sketch coordinates; ok only when it
// spans something (a lone point is dragged as it is).
func (sm *sketchMode) selBox(a *App) (lo, hi geom.Vec2, ok bool) {
	s := sm.item.S
	ids := sm.selected(a)
	if len(ids) == 0 {
		return
	}
	lo = geom.Vec2{X: math.Inf(1), Y: math.Inf(1)}
	hi = geom.Vec2{X: math.Inf(-1), Y: math.Inf(-1)}
	for _, id := range ids {
		for _, p := range s.Polyline(s.Ent(id), 0.05) {
			lo.X, lo.Y = math.Min(lo.X, p.X), math.Min(lo.Y, p.Y)
			hi.X, hi.Y = math.Max(hi.X, p.X), math.Max(hi.Y, p.Y)
		}
	}
	ok = hi.X-lo.X > 1e-9 || hi.Y-lo.Y > 1e-9
	return
}

// handlePoints are the box's handles in sketch coordinates: corners 1–4
// (lo-lo, hi-lo, hi-hi, lo-hi), then the sides' middles 5–8 (bottom, right,
// top, left).
func handlePoints(lo, hi geom.Vec2) [9]geom.Vec2 {
	m := lo.Lerp(hi, 0.5)
	return [9]geom.Vec2{m, lo, {X: hi.X, Y: lo.Y}, hi, {X: lo.X, Y: hi.Y},
		{X: m.X, Y: lo.Y}, {X: hi.X, Y: m.Y}, {X: m.X, Y: hi.Y}, {X: lo.X, Y: m.Y}}
}

// xformHandleAt is the handle under the mouse (0 inside the box but on no
// handle, −1 outside).
func (sm *sketchMode) xformHandleAt(a *App) int {
	lo, hi, ok := sm.selBox(a)
	if !ok {
		return -1
	}
	s := sm.item.S
	m := vadd(a.vp.pos, a.vp.mouse)
	hp := handlePoints(lo, hi)
	for i := 1; i < 9; i++ {
		if vlen(vsub(a.skScreen(s, hp[i]), m)) < 8 {
			return i
		}
	}
	a0, a1 := a.skScreen(s, lo), a.skScreen(s, hi)
	minX, maxX := math.Min(float64(a0.X), float64(a1.X)), math.Max(float64(a0.X), float64(a1.X))
	minY, maxY := math.Min(float64(a0.Y), float64(a1.Y)), math.Max(float64(a0.Y), float64(a1.Y))
	if float64(m.X) > minX && float64(m.X) < maxX && float64(m.Y) > minY && float64(m.Y) < maxY {
		return 0
	}
	return -1
}

// xformPress starts a transform when the press is on a handle, or inside
// the box on a selected curve of a selection of more than one.
func (sm *sketchMode) xformPress(a *App) bool {
	h := sm.xformHandleAt(a)
	if h < 0 {
		return false
	}
	ids := sm.selected(a)
	if h == 0 {
		hv := a.hover
		onSel := false
		for _, id := range ids {
			onSel = onSel || (hv.Item == sm.item.ID && hv.Local == id)
		}
		if !onSel || len(ids) < 2 {
			return false // an ordinary drag of what is under the mouse
		}
	}
	c, ok := sm.cursor2(a)
	if !ok {
		return false
	}
	s := sm.item.S
	lo, hi, _ := sm.selBox(a)
	sm.xf = skXform{on: true, handle: h, lo: lo, hi: hi, start: c, pts: map[int]geom.Vec2{}, radii: map[int]float64{}}
	sm.xf.dx.follow(c.X, false)
	sm.xf.dy.follow(c.Y, false)
	for _, id := range ids {
		e := s.Ent(id)
		for _, p := range s.PointsOf(id) {
			sm.xf.pts[p] = s.P(p)
		}
		if e.Kind == sketch.Circle {
			sm.xf.radii[e.Par] = s.Params[e.Par]
		}
	}
	return true
}

// xformDrag applies the transform the mouse has dragged to.
func (sm *sketchMode) xformDrag(a *App) {
	c, ok := sm.cursor2(a)
	if !ok {
		return
	}
	k := a.dragKeys()
	x := &sm.xf
	v := geom.Vec2{X: x.dx.follow(c.X, k.fine), Y: x.dy.follow(c.Y, k.fine)}
	d := v.Sub(x.start)
	lo, hi := x.lo, x.hi
	// the map: p ↦ anchor + (p − anchor)·(sx, sy) + t
	anchor := geom.Vec2{}
	sx, sy, rs := 1.0, 1.0, 1.0
	var t geom.Vec2
	snapRatio := func(r float64) float64 {
		if !k.snap {
			return r
		}
		st := 0.05
		if k.fine {
			st = 0.005
		}
		return math.Max(st, math.Round(r/st)*st)
	}
	switch h := x.handle; {
	case h == 0:
		t = geom.Vec2{X: a.snapLen(d.X, k), Y: a.snapLen(d.Y, k)}
		x.label = "Δ " + fmtLen(t.X) + ", " + fmtLen(t.Y)
	case h <= 4:
		// corner: even scale about the opposite corner
		hp := handlePoints(lo, hi)
		opp := hp[(h+1)%4+1]
		anchor = opp
		from := hp[h].Sub(opp)
		to := hp[h].Add(d).Sub(opp)
		r := to.Dot(from) / math.Max(from.Dot(from), 1e-12)
		r = snapRatio(r)
		sx, sy, rs = r, r, r
		x.label = "× " + strconv.FormatFloat(r, 'f', -1, 64)
	default:
		// side: stretch along its axis about the opposite side
		w, ht := hi.X-lo.X, hi.Y-lo.Y
		switch h {
		case 5: // bottom: about the top
			anchor = geom.Vec2{Y: hi.Y}
			sy = snapLen0(a, ht-d.Y, k) / math.Max(ht, 1e-12)
		case 7: // top: about the bottom
			anchor = geom.Vec2{Y: lo.Y}
			sy = snapLen0(a, ht+d.Y, k) / math.Max(ht, 1e-12)
		case 6: // right: about the left
			anchor = geom.Vec2{X: lo.X}
			sx = snapLen0(a, w+d.X, k) / math.Max(w, 1e-12)
		case 8: // left: about the right
			anchor = geom.Vec2{X: hi.X}
			sx = snapLen0(a, w-d.X, k) / math.Max(w, 1e-12)
		}
		rs = math.Sqrt(math.Abs(sx * sy))
		if h == 6 || h == 8 {
			x.label = "W " + fmtLen(w*sx)
		} else {
			x.label = "H " + fmtLen(ht*sy)
		}
	}
	s := sm.item.S
	for id, p := range x.pts {
		q := geom.Vec2{X: anchor.X + (p.X-anchor.X)*sx + t.X, Y: anchor.Y + (p.Y-anchor.Y)*sy + t.Y}
		if x.handle >= 5 && (x.handle == 5 || x.handle == 7) {
			q.X = p.X
		}
		if x.handle == 6 || x.handle == 8 {
			q.Y = p.Y
		}
		s.SetP(id, q)
	}
	for par, r := range x.radii {
		s.Params[par] = math.Max(r*rs, 1e-6)
	}
	sm.keepConstraints()
	s.Solve()
	sm.dragLabel = x.label
	sm.dragging = true
	sm.item.markDirty()
}

// snapLen0 snaps a length that must stay above zero.
func snapLen0(a *App, v float64, k dragKeys) float64 {
	return math.Max(a.snapLen(v, k), a.increment()/10)
}

// keepConstraints makes the constraints wholly on moved geometry agree
// with where it now is: a fixed point fixed at its new place, a dimension
// measuring its new size (unless it is worked out from parameters).
func (sm *sketchMode) keepConstraints() {
	s := sm.item.S
	moved := func(id int) bool {
		e := s.Ent(id)
		if e == nil {
			return false
		}
		for _, p := range s.PointsOf(id) {
			if _, ok := sm.xf.pts[p]; !ok {
				return false
			}
		}
		return true
	}
	for _, c := range s.Cons {
		all := true
		for _, id := range c.E {
			all = all && moved(id)
		}
		if !all {
			continue
		}
		switch {
		case c.Kind == sketch.Fix:
			p := s.P(c.E[0])
			c.Value, c.Value2 = p.X, p.Y
		case c.Kind.Dimensional() && c.Kind != sketch.Angle:
			if c.Expr != "" {
				if _, err := strconv.ParseFloat(strings.TrimSpace(c.Expr), 64); err != nil {
					continue // a parameter's expression: left to pull it back
				}
			}
			v := s.Measure(c)
			if c.Kind == sketch.Distance || c.Kind == sketch.Radius || c.Kind == sketch.Diameter {
				v = math.Abs(v)
			}
			c.Value = v
			if c.Expr != "" {
				c.Expr = fmtLen(math.Abs(v))
			}
		}
	}
}

// drawXform draws the selection's box and handles.
func (sm *sketchMode) drawXform(a *App, dl *imgui.DrawList) {
	lo, hi, ok := sm.selBox(a)
	if !ok || sm.tool != toolSelect {
		return
	}
	if sm.xf.on {
		lo, hi = sm.selBoxNow(a)
	}
	s := sm.item.S
	col := accentU32(0.9)
	hp := handlePoints(lo, hi)
	corners := []geom.Vec2{hp[1], hp[2], hp[3], hp[4]}
	for i := range corners {
		p, q := a.skScreen(s, corners[i]), a.skScreen(s, corners[(i+1)%4])
		// dashed
		d := vsub(q, p)
		l := vlen(d)
		n := int(l / 6)
		for k := 0; k < n; k += 2 {
			dl.AddLineArgs(vadd(p, vscale(d, float32(k)/float32(n))), vadd(p, vscale(d, float32(k+1)/float32(n))), col, 1.2)
		}
	}
	hot := sm.xformHandleAt(a)
	if sm.xf.on {
		hot = sm.xf.handle
	}
	for i := 1; i < 9; i++ {
		p := a.skScreen(s, hp[i])
		r := float32(4)
		if i == hot {
			r = 6
			dl.AddRectFilled(vsub(p, imgui.Vec2{X: r, Y: r}), vadd(p, imgui.Vec2{X: r, Y: r}), col)
		} else {
			dl.AddRectFilled(vsub(p, imgui.Vec2{X: r, Y: r}), vadd(p, imgui.Vec2{X: r, Y: r}), u32(1, 1, 1, 1))
		}
		dl.AddRect(vsub(p, imgui.Vec2{X: r, Y: r}), vadd(p, imgui.Vec2{X: r, Y: r}), col)
	}
}

// selBoxNow is the box round the selection as it is now (during a drag).
func (sm *sketchMode) selBoxNow(a *App) (geom.Vec2, geom.Vec2) {
	lo, hi, _ := sm.selBox(a)
	return lo, hi
}

func (x skXform) String() string { return fmt.Sprintf("xform %d", x.handle) }

// projectClick projects the body edge under the mouse into the sketch, or
// every edge of the face under it.
func (sm *sketchMode) projectClick(a *App) {
	h := a.hover
	if sm.item.F == nil {
		return
	}
	var edges []*brep.Edge
	switch h.Kind {
	case KindEdge:
		if _, e := a.refEdge(h); e != nil {
			edges = append(edges, e)
		}
	case KindFace:
		if _, f := a.refFace(h); f != nil {
			for _, l := range f.Loops {
				for _, he := range l.HalfEdges() {
					edges = append(edges, he.Edge)
				}
			}
		}
	default:
		a.notify("Click an edge or a face of a body to project it")
		return
	}
	n := 0
	for _, e := range edges {
		if err := doc.ProjectEdge(sm.item.F, e, e.Curve.Eval((e.T0+e.T1)/2)); err == nil {
			n++
		}
	}
	if n > 0 {
		sm.changed()
	}
}
