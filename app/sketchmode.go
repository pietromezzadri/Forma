package app

import (
	"math"

	imgui "github.com/AllenDang/cimgui-go/imgui"
	"github.com/go-gl/mathgl/mgl32"

	"forma/expr"
	"forma/geom"
	"forma/render"
	"forma/sketch"
)

type sketchTool int

const (
	toolSelect sketchTool = iota
	toolLine
	toolRect
	toolCircle
	toolArc
	toolPoint
	toolDim
	toolSpline
)

var toolNames = [...]string{"Select", "Line", "Rectangle", "Circle", "Arc", "Point", "Dimension", "Spline"}

var toolHints = [...]string{
	"Click to select, drag points and curves to move them. Esc clears.",
	"Click to place points; the line chains on. Esc or right-click stops.",
	"Click two opposite corners.",
	"Click the centre, then a point on the circle.",
	"Click the centre, the start, then the end (counter-clockwise).",
	"Click to place a point.",
	"Click a line, circle or arc; or two points, a point and a line, or two lines.",
	"Click the start, two handles, then the end; it chains on smoothly. Esc stops.",
}

// snapPt is where a click lands after snapping.
type snapPt struct {
	p     geom.Vec2
	point int    // an existing point (coincident)
	on    int    // a curve it lies on
	mid   int    // a line whose midpoint it is
	hv    int    // 1 horizontal, 2 vertical (from the previous click)
	label string // what it snapped to, for the cursor
	// on the sketch's origin: a new point there is pinned to it, so what
	// is drawn round the origin stays centred on it whatever changes
	origin bool
}

type sketchMode struct {
	app    *App
	item   *SketchItem
	tool   sketchTool
	clicks []snapPt
	chain  int // line tool: the point the next line starts from
	first  int // line tool: the chain's first point
	cur    snapPt
	valid  bool

	dragEnt  int
	dragGrab geom.Vec2
	dragging bool
	// a drag's own cursor (slowed while Shift is held), where it began and
	// where the dragged curve's first point was then
	dragX, dragY        fineDrag
	dragStart, dragFrom geom.Vec2
	dragLabel           string
	// the spline tool: the last spline of a chain and the chain's first point
	prevSpline, splineFirst int

	dimFirst Ref

	editDim  int // constraint being edited (0: none)
	editBuf  string
	editPos  imgui.Vec2
	editOpen bool

	dimRects map[int][4]float32 // constraint ID → label rect (last frame)
	hoverDim int
}

func (a *App) enterSketch(item *SketchItem) {
	a.sk = &sketchMode{app: a, item: item, tool: toolLine, dimRects: map[int][4]float32{}}
	a.sel = a.sel[:0]
	a.hover = Ref{}
	item.Visible = true
	item.markDirty()
	item.S.Solve()
	pl := item.S.Plane
	target := pl.O
	if b := item.S.Bounds(); !b.Empty() {
		target = pl.At2(geom.Vec2{X: b.Center().X, Y: b.Center().Y})
	}
	a.cam.LookAlong(pl.Z.Neg(), pl.Y, target)
}

func (a *App) finishSketch() {
	if a.sk == nil {
		return
	}
	a.sk.item.markDirty()
	// the finished sketch stays picked: Extrude, Hole… work on it next
	a.browserSel = a.sk.item.ID
	a.sk = nil
	a.sel = a.sel[:0]
}

// planeFor turns a named plane or a selected planar face into a sketch plane.

// cursor2 is the cursor on the sketch plane, in sketch coordinates.
func (sm *sketchMode) cursor2(a *App) (geom.Vec2, bool) {
	p, ok := a.rayPlane(sm.item.S.Plane)
	if !ok {
		return geom.Vec2{}, false
	}
	l := sm.item.S.Plane.ToLocal(p)
	return geom.Vec2{X: l.X, Y: l.Y}, true
}

// screen distance in UI units between two sketch points
func (a *App) skScreenDist(s *sketch.Sketch, p, q geom.Vec2) float64 {
	sp, ok1 := a.toScreen(s.Plane.At2(p))
	sq, ok2 := a.toScreen(s.Plane.At2(q))
	if !ok1 || !ok2 {
		return math.Inf(1)
	}
	return math.Hypot(float64(sp.X-sq.X), float64(sp.Y-sq.Y))
}

// snap works out where the cursor lands.
func (sm *sketchMode) snap(a *App) (snapPt, bool) {
	s := sm.item.S
	c, ok := sm.cursor2(a)
	if !ok {
		return snapPt{}, false
	}
	sp := snapPt{p: c}
	h := a.hover
	if h.Item == sm.item.ID {
		switch h.Kind {
		case KindSkPoint:
			if e := s.Ent(h.Local); e != nil {
				return snapPt{p: s.P(h.Local), point: h.Local, label: "point"}, true
			}
		case KindSkCurve:
			e := s.Ent(h.Local)
			if e == nil {
				break
			}
			switch e.Kind {
			case sketch.Line:
				a0, a1 := s.P(e.Pts[0]), s.P(e.Pts[1])
				m := a0.Lerp(a1, 0.5)
				if a.skScreenDist(s, m, c) < 9 {
					return snapPt{p: m, mid: e.ID, label: "midpoint"}, true
				}
				d := a1.Sub(a0)
				t := math.Max(0, math.Min(1, c.Sub(a0).Dot(d)/math.Max(d.Dot(d), 1e-12)))
				return snapPt{p: a0.Add(d.Scale(t)), on: e.ID, label: "on line"}, true
			case sketch.Circle, sketch.Arc:
				cc := s.P(e.Pts[0])
				r := s.Radius(e)
				dir := c.Sub(cc).Norm()
				if dir.Len() == 0 {
					dir = geom.Vec2{X: 1}
				}
				return snapPt{p: cc.Add(dir.Scale(r)), on: e.ID, label: "on curve"}, true
			}
		}
	}
	// centres of circles and arcs near the cursor
	for _, e := range s.Ents {
		if e.Kind == sketch.Circle || e.Kind == sketch.Arc {
			cc := s.P(e.Pts[0])
			if a.skScreenDist(s, cc, c) < 9 {
				return snapPt{p: cc, point: e.Pts[0], label: "centre"}, true
			}
		}
	}
	// the origin: an existing point there, else a new one pinned to it
	if a.skScreenDist(s, geom.Vec2{}, c) < 9 {
		for _, e := range s.Ents {
			if e.Kind == sketch.Point && s.P(e.ID).Len() < 1e-9 {
				return snapPt{point: e.ID, label: "origin"}, true
			}
		}
		return snapPt{origin: true, label: "origin"}, true
	}
	// horizontal / vertical from the previous click
	if n := len(sm.clicks); n > 0 && (sm.tool == toolLine) {
		prev := sm.clicks[n-1].p
		d := c.Sub(prev)
		lim := math.Tan(2.5 * math.Pi / 180)
		if math.Abs(d.Y) < lim*math.Abs(d.X) {
			sp.p.Y, sp.hv, sp.label = prev.Y, 1, "horizontal"
		} else if math.Abs(d.X) < lim*math.Abs(d.Y) {
			sp.p.X, sp.hv, sp.label = prev.X, 2, "vertical"
		}
	}
	if k := a.dragKeys(); k.snap {
		sub := a.increment()
		if k.fine {
			sub /= 10
		}
		gx, gy := math.Round(sp.p.X/sub)*sub, math.Round(sp.p.Y/sub)*sub
		if sp.hv != 1 {
			sp.p.Y = gy
		}
		if sp.hv != 2 {
			sp.p.X = gx
		}
	}
	return sp, true
}

// pointFor returns the point a snap means: the snapped-to point, or a new
// one (with its on-curve or midpoint constraint added after the curve that
// uses it exists).
func (sm *sketchMode) pointFor(sp snapPt) int {
	if sp.point != 0 {
		return sp.point
	}
	return sm.item.S.AddPoint(sp.p).ID
}

func (sm *sketchMode) snapConstraints(id int, sp snapPt) {
	s := sm.item.S
	if sp.point != 0 {
		return
	}
	if sp.origin {
		s.AddConstraint(sketch.Fix, []int{id}, 0)
	}
	if sp.on != 0 {
		if e := s.Ent(sp.on); e != nil {
			if e.Kind == sketch.Line {
				s.AddConstraint(sketch.PointOnLine, []int{id, sp.on}, 0)
			} else {
				s.AddConstraint(sketch.PointOnCircle, []int{id, sp.on}, 0)
			}
		}
	}
	if sp.mid != 0 {
		s.AddConstraint(sketch.Midpoint, []int{id, sp.mid}, 0)
	}
}

// changed solves the sketch and records an undo step (regenerating what
// follows from it).
func (sm *sketchMode) changed() {
	sm.item.S.Solve()
	sm.item.markDirty()
	sm.app.doc.Commit()
}

func (sm *sketchMode) cancel() {
	sm.clicks = sm.clicks[:0]
	sm.chain, sm.first = 0, 0
	sm.prevSpline, sm.splineFirst = 0, 0
	sm.dimFirst = Ref{}
}

func (sm *sketchMode) setTool(t sketchTool) {
	sm.cancel()
	sm.tool = t
}

func (sm *sketchMode) press(a *App) {
	if sm.editOpen {
		return
	}
	s := sm.item.S
	if sm.hoverDim != 0 {
		a.toggleSelect(Ref{Kind: KindDim, Item: sm.item.ID, Local: sm.hoverDim}, imgui.CurrentIO().KeyCtrl())
		return
	}
	sp, ok := sm.snap(a)
	if !ok {
		return
	}
	switch sm.tool {
	case toolSelect:
		h := a.hover
		if h.Item == sm.item.ID && (h.Kind == KindSkPoint || h.Kind == KindSkCurve) {
			c, _ := sm.cursor2(a)
			sm.dragEnt, sm.dragGrab, sm.dragging = h.Local, c, false
		}
	case toolLine:
		if len(sm.clicks) == 0 {
			id := sm.pointFor(sp)
			sm.snapConstraints(id, sp)
			sm.chain, sm.first = id, id
			sm.clicks = append(sm.clicks, sp)
			sm.changed()
			return
		}
		if s.P(sm.chain).Dist(sp.p) < 1e-9 {
			return
		}
		end := sm.pointFor(sp)
		l := s.AddLine(sm.chain, end)
		sm.snapConstraints(end, sp)
		switch sp.hv {
		case 1:
			s.AddConstraint(sketch.Horizontal, []int{l.ID}, 0)
		case 2:
			s.AddConstraint(sketch.Vertical, []int{l.ID}, 0)
		}
		sm.changed()
		if end == sm.first {
			sm.cancel() // closed the loop
			return
		}
		sm.chain = end
		sm.clicks = append(sm.clicks, sp)
	case toolRect:
		if len(sm.clicks) == 0 {
			sm.clicks = append(sm.clicks, sp)
			return
		}
		p0, p2 := sm.clicks[0], sp
		if math.Abs(p0.p.X-p2.p.X) < 1e-9 || math.Abs(p0.p.Y-p2.p.Y) < 1e-9 {
			return
		}
		a0 := sm.pointFor(p0)
		a1 := s.AddPoint(geom.Vec2{X: p2.p.X, Y: p0.p.Y}).ID
		a2 := sm.pointFor(p2)
		a3 := s.AddPoint(geom.Vec2{X: p0.p.X, Y: p2.p.Y}).ID
		l0 := s.AddLine(a0, a1)
		l1 := s.AddLine(a1, a2)
		l2 := s.AddLine(a2, a3)
		l3 := s.AddLine(a3, a0)
		sm.snapConstraints(a0, p0)
		sm.snapConstraints(a2, p2)
		s.AddConstraint(sketch.Horizontal, []int{l0.ID}, 0)
		s.AddConstraint(sketch.Horizontal, []int{l2.ID}, 0)
		s.AddConstraint(sketch.Vertical, []int{l1.ID}, 0)
		s.AddConstraint(sketch.Vertical, []int{l3.ID}, 0)
		sm.changed()
		sm.cancel()
	case toolCircle:
		if len(sm.clicks) == 0 {
			sm.clicks = append(sm.clicks, sp)
			return
		}
		r := sm.clicks[0].p.Dist(sp.p)
		if r < 1e-9 {
			return
		}
		c := sm.pointFor(sm.clicks[0])
		s.AddCircle(c, r)
		sm.snapConstraints(c, sm.clicks[0])
		sm.changed()
		sm.cancel()
	case toolArc:
		sm.clicks = append(sm.clicks, sp)
		if len(sm.clicks) < 3 {
			if len(sm.clicks) == 2 && sm.clicks[1].p.Dist(sm.clicks[0].p) < 1e-9 {
				sm.clicks = sm.clicks[:1]
			}
			return
		}
		c0, st, en := sm.clicks[0], sm.clicks[1], sm.clicks[2]
		r := st.p.Dist(c0.p)
		dir := en.p.Sub(c0.p).Norm()
		en.p = c0.p.Add(dir.Scale(r))
		if en.point != 0 && math.Abs(s.P(en.point).Dist(c0.p)-r) > 1e-6 {
			en.point = 0
		}
		c := sm.pointFor(c0)
		pst := sm.pointFor(st)
		pen := sm.pointFor(en)
		s.AddArc(c, pst, pen)
		sm.snapConstraints(c, c0)
		sm.snapConstraints(pst, st)
		sm.snapConstraints(pen, en)
		sm.changed()
		sm.cancel()
	case toolPoint:
		id := sm.pointFor(sp)
		sm.snapConstraints(id, sp)
		sm.changed()
	case toolSpline:
		sm.splineClick(sp)
	case toolDim:
		sm.dimClick(a)
	}
}

func (sm *sketchMode) drag(a *App) {
	if sm.tool != toolSelect || sm.dragEnt == 0 {
		return
	}
	c, ok := sm.cursor2(a)
	if !ok {
		return
	}
	s := sm.item.S
	e := s.Ent(sm.dragEnt)
	if e == nil {
		return
	}
	if !sm.dragging {
		if a.skScreenDist(s, c, sm.dragGrab) < 4 {
			return
		}
		// start from where it was grabbed
		sm.dragX.reset()
		sm.dragY.reset()
		sm.dragX.follow(sm.dragGrab.X, false)
		sm.dragY.follow(sm.dragGrab.Y, false)
		sm.dragStart = sm.dragGrab
		if e.Kind != sketch.Point {
			sm.dragFrom = s.P(e.Pts[0])
		}
	}
	sm.dragging = true
	k := a.dragKeys()
	v := geom.Vec2{X: sm.dragX.follow(c.X, k.fine), Y: sm.dragY.follow(c.Y, k.fine)}
	snap2 := func(p geom.Vec2) geom.Vec2 { return geom.Vec2{X: a.snapLen(p.X, k), Y: a.snapLen(p.Y, k)} }
	switch e.Kind {
	case sketch.Point:
		t := snap2(v)
		s.Drag(e.ID, s.P(e.ID), t)
		p := s.P(e.ID)
		sm.dragLabel = fmtLen(p.X) + ", " + fmtLen(p.Y)
	case sketch.Circle:
		cc := s.P(e.Pts[0])
		dir := v.Sub(cc).Norm()
		if dir.Len() == 0 {
			dir = geom.Vec2{X: 1}
		}
		r := math.Max(a.snapLen(v.Dist(cc), k), a.increment()/10)
		s.Drag(e.ID, v, cc.Add(dir.Scale(r)))
		sm.dragLabel = "Ø " + fmtLen(2*s.Radius(e))
	default:
		// move the whole curve: its first point goes onto the grid
		to := snap2(sm.dragFrom.Add(v.Sub(sm.dragStart)))
		s.Drag(e.ID, s.P(e.Pts[0]), to)
		d := s.P(e.Pts[0]).Sub(sm.dragFrom)
		sm.dragLabel = "Δ " + fmtLen(d.X) + ", " + fmtLen(d.Y)
		if e.Kind == sketch.Line {
			sm.dragLabel += "   (" + fmtLen(s.P(e.Pts[0]).Dist(s.P(e.Pts[1]))) + " mm)"
		}
	}
	sm.dragGrab = c
	sm.item.markDirty()
}

// release ends a drag; it returns true when it used the click.
func (sm *sketchMode) release(a *App, moved bool) bool {
	was := sm.dragging
	sm.dragEnt, sm.dragging, sm.dragLabel = 0, false, ""
	if was {
		sm.app.doc.Commit()
		return true
	}
	if sm.tool != toolSelect {
		return true // tool clicks don't select
	}
	if sm.hoverDim != 0 {
		return true
	}
	return false
}

func (sm *sketchMode) doubleClick(a *App) {
	if sm.hoverDim != 0 {
		sm.openEdit(sm.hoverDim)
	}
}

// dimClick builds a dimension from what is clicked.
func (sm *sketchMode) dimClick(a *App) {
	s := sm.item.S
	h := a.hover
	if h.Item != sm.item.ID || (h.Kind != KindSkPoint && h.Kind != KindSkCurve) {
		if sm.dimFirst.Kind == KindSkCurve {
			if e := s.Ent(sm.dimFirst.Local); e != nil && e.Kind == sketch.Line {
				sm.addDim(sketch.Distance, []int{e.Pts[0], e.Pts[1]})
			}
		}
		sm.dimFirst = Ref{}
		return
	}
	e := s.Ent(h.Local)
	if e == nil {
		return
	}
	if sm.dimFirst.IsZero() {
		switch e.Kind {
		case sketch.Circle:
			sm.addDim(sketch.Diameter, []int{e.ID})
		case sketch.Arc:
			sm.addDim(sketch.Radius, []int{e.ID})
		default:
			sm.dimFirst = h
		}
		return
	}
	f := s.Ent(sm.dimFirst.Local)
	sm.dimFirst = Ref{}
	if f == nil {
		return
	}
	switch {
	case f.ID == e.ID && e.Kind == sketch.Line:
		sm.addDim(sketch.Distance, []int{e.Pts[0], e.Pts[1]})
	case f.Kind == sketch.Point && e.Kind == sketch.Point:
		sm.addDim(sketch.Distance, []int{f.ID, e.ID})
	case f.Kind == sketch.Point && e.Kind == sketch.Line:
		sm.addDim(sketch.Distance, []int{f.ID, e.ID})
	case f.Kind == sketch.Line && e.Kind == sketch.Point:
		sm.addDim(sketch.Distance, []int{e.ID, f.ID})
	case f.Kind == sketch.Line && e.Kind == sketch.Line:
		sm.addDim(sketch.Angle, []int{f.ID, e.ID})
	default:
		a.notify("No dimension between a %s and a %s", f.Kind, e.Kind)
	}
}

func (sm *sketchMode) addDim(k sketch.ConKind, ids []int) {
	c, err := sm.item.S.AddConstraint(k, ids, math.NaN())
	if err != nil {
		sm.err(err)
		return
	}
	sm.changed()
	sm.openEdit(c.ID)
}

func (sm *sketchMode) err(err error) { sm.app.notify("%v", err) }

func (sm *sketchMode) openEdit(id int) {
	c := sm.item.S.Con(id)
	if c == nil {
		return
	}
	sm.editDim = id
	sm.editOpen = true
	if c.Expr != "" {
		sm.editBuf = c.Expr
	} else if c.Kind == sketch.Angle {
		sm.editBuf = fmtLen(math.Abs(c.Value) * 180 / math.Pi)
	} else {
		sm.editBuf = fmtLen(math.Abs(c.Value))
	}
}

// applyEdit evaluates the typed value and sets the dimension.
func (sm *sketchMode) applyEdit(a *App) {
	c := sm.item.S.Con(sm.editDim)
	sm.editOpen = false
	if c == nil {
		return
	}
	v, err := expr.Eval(sm.editBuf, a.doc.D.Vars())
	if err != nil {
		a.notify("%v", err)
		return
	}
	if c.Kind == sketch.Angle {
		v = v * math.Pi / 180
		if c.Value < 0 {
			v = -v
		}
	} else if c.Kind == sketch.HDistance || c.Kind == sketch.VDistance {
		if c.Value < 0 {
			v = -v
		}
	}
	if err := sm.item.S.SetValue(c.ID, v, sm.editBuf); err != nil {
		a.notify("%v", err)
		return
	}
	sm.item.markDirty()
	a.doc.Commit()
}

// applyConstraint constrains the selection.
func (a *App) applyConstraint(k sketch.ConKind) {
	sm := a.sk
	if sm == nil {
		return
	}
	s := sm.item.S
	ids := a.selectedSketchEnts(sm.item.ID)
	if len(ids) == 0 {
		a.notify("Select sketch geometry first")
		return
	}
	kinds := make([]sketch.Kind, len(ids))
	for i, id := range ids {
		kinds[i] = s.Ent(id).Kind
	}
	try := func(k sketch.ConKind, ids []int) bool {
		if _, err := s.AddConstraint(k, ids, 0); err != nil {
			a.notify("%s: %v", k, err)
			return false
		}
		return true
	}
	done := false
	switch k {
	case sketch.Horizontal, sketch.Vertical:
		if len(ids) == 2 && kinds[0] == sketch.Point && kinds[1] == sketch.Point {
			done = try(k, ids)
		} else {
			for i, id := range ids {
				if kinds[i] == sketch.Line {
					done = try(k, []int{id}) || done
				}
			}
		}
	case sketch.Fix:
		for i, id := range ids {
			if kinds[i] == sketch.Point {
				done = try(k, []int{id}) || done
			} else {
				for _, p := range s.Ent(id).Pts {
					done = try(k, []int{p}) || done
				}
			}
		}
	case sketch.Coincident:
		if len(ids) == 2 {
			p, q := ids[0], ids[1]
			switch {
			case kinds[0] == sketch.Point && kinds[1] == sketch.Point:
				done = try(sketch.Coincident, ids)
			case kinds[0] == sketch.Point && kinds[1] == sketch.Line:
				done = try(sketch.PointOnLine, ids)
			case kinds[1] == sketch.Point && kinds[0] == sketch.Line:
				done = try(sketch.PointOnLine, []int{q, p})
			case kinds[0] == sketch.Point:
				done = try(sketch.PointOnCircle, ids)
			case kinds[1] == sketch.Point:
				done = try(sketch.PointOnCircle, []int{q, p})
			}
		}
	case sketch.Midpoint:
		if len(ids) == 2 {
			if kinds[0] == sketch.Line {
				ids[0], ids[1] = ids[1], ids[0]
			}
			done = try(k, ids)
		}
	default:
		// pairs: chain them (each with the first)
		if len(ids) >= 2 {
			for _, id := range ids[1:] {
				pair := []int{ids[0], id}
				if k == sketch.Tangent && s.Ent(id).Kind == sketch.Line {
					pair = []int{id, ids[0]}
				}
				done = try(k, pair) || done
			}
		}
	}
	if !done && len(ids) > 0 {
		if s.Ent(ids[0]) != nil && !done {
			a.notify("%s needs %s", k, needs(k))
		}
	}
	sm.changed()
}

func needs(k sketch.ConKind) string {
	switch k {
	case sketch.Coincident:
		return "two points, or a point and a curve"
	case sketch.Horizontal, sketch.Vertical:
		return "lines, or two points"
	case sketch.Parallel, sketch.Perpendicular:
		return "two lines"
	case sketch.Tangent:
		return "a line and an arc or circle, or two arcs / circles"
	case sketch.Equal:
		return "two lines or two arcs / circles"
	case sketch.Concentric:
		return "two arcs / circles"
	case sketch.Midpoint:
		return "a point and a line"
	}
	return "other geometry"
}

// deleteSelection removes the selected sketch geometry and dimensions.
func (a *App) deleteSelection() {
	if a.sk != nil {
		s := a.sk.item.S
		for _, r := range a.sel {
			if r.Item != a.sk.item.ID {
				continue
			}
			switch r.Kind {
			case KindDim:
				s.DeleteConstraint(r.Local)
			case KindSkPoint, KindSkCurve:
				if s.Ent(r.Local) != nil {
					s.Delete(r.Local)
				}
			}
		}
		a.sel = a.sel[:0]
		a.sk.changed()
		return
	}
	if a.browserSel != 0 {
		a.doc.Remove(a.browserSel)
		a.browserSel = 0
		a.sel = a.sel[:0]
		a.hover = Ref{}
	}
}

// drawPreview draws the rubber band of the tool in progress.
func (sm *sketchMode) drawPreview(a *App, v render.View) {
	if sm.editOpen || !a.vp.hovered {
		sm.valid = false
		return
	}
	sp, ok := sm.snap(a)
	sm.cur, sm.valid = sp, ok
	if !ok || len(sm.clicks) == 0 {
		return
	}
	s := sm.item.S
	pl := s.Plane
	var pts [][]geom.Vec3
	c0 := sm.clicks[0].p
	switch sm.tool {
	case toolLine:
		pts = append(pts, []geom.Vec3{pl.At2(s.P(sm.chain)), pl.At2(sp.p)})
	case toolRect:
		q := []geom.Vec2{c0, {X: sp.p.X, Y: c0.Y}, sp.p, {X: c0.X, Y: sp.p.Y}, c0}
		var l []geom.Vec3
		for _, p := range q {
			l = append(l, pl.At2(p))
		}
		pts = append(pts, l)
	case toolCircle:
		pts = append(pts, circlePts(pl, c0, c0.Dist(sp.p), 0, 2*math.Pi))
	case toolSpline:
		pts = append(pts, splinePreview(pl, sm.clicks, sp.p)...)
	case toolArc:
		if len(sm.clicks) == 1 {
			pts = append(pts, []geom.Vec3{pl.At2(c0), pl.At2(sp.p)})
		} else {
			st := sm.clicks[1].p
			r := st.Dist(c0)
			a0 := st.Sub(c0).Angle()
			sw := geom.WrapAngle(sp.p.Sub(c0).Angle() - a0)
			pts = append(pts, circlePts(pl, c0, r, a0, sw))
		}
	}
	var pls []render.Polyline
	for _, l := range pts {
		pls = append(pls, render.Polyline{ID: 1, Pts: l})
	}
	ls := render.NewLineSet(pls)
	a.r.DrawLines(v, ls, render.Paint{Color: mgl32.Vec4{0.95, 0.45, 0.1, 1}, Width: 1.6 * a.vp.scale, DepthBias: 5e-5}, false)
	ls.Delete()
}

func circlePts(pl geom.Frame, c geom.Vec2, r, a0, sw float64) []geom.Vec3 {
	n := 96
	var out []geom.Vec3
	for i := 0; i <= n; i++ {
		t := a0 + sw*float64(i)/float64(n)
		out = append(out, pl.At2(c.Add(geom.Vec2{X: math.Cos(t), Y: math.Sin(t)}.Scale(r))))
	}
	return out
}

// splineClick takes the spline tool's clicks: start, two handles, end. A
// finished curve chains on: the next starts at its end with its first
// handle mirrored and held tangent, so it needs only a handle and an end.
func (sm *sketchMode) splineClick(sp snapPt) {
	s := sm.item.S
	if n := len(sm.clicks); n > 0 && sm.clicks[n-1].p.Dist(sp.p) < 1e-9 {
		return // a double click
	}
	sm.clicks = append(sm.clicks, sp)
	if len(sm.clicks) < 4 {
		return
	}
	c := sm.clicks
	p0 := sm.pointFor(c[0])
	if sm.splineFirst == 0 {
		sm.splineFirst = p0
	}
	h1 := s.AddPoint(c[1].p).ID
	h2 := s.AddPoint(c[2].p).ID
	p3 := sm.pointFor(c[3])
	e := s.AddSpline(p0, h1, h2, p3)
	sm.snapConstraints(p0, c[0])
	sm.snapConstraints(p3, c[3])
	if sm.prevSpline != 0 {
		s.AddConstraint(sketch.Tangent, []int{sm.prevSpline, e.ID}, 0)
	}
	sm.changed()
	if p3 == sm.splineFirst {
		sm.cancel() // closed the loop
		return
	}
	// chain on from the end, the next first handle mirrored through it
	end := s.P(p3)
	mirror := end.Add(end.Sub(s.P(h2)))
	sm.prevSpline = e.ID
	sm.clicks = append(sm.clicks[:0], snapPt{p: end, point: p3}, snapPt{p: mirror})
}

// splinePreview is the curve the spline tool would make with the cursor as
// its next click, and its control polygon.
func splinePreview(pl geom.Frame, clicks []snapPt, cur geom.Vec2) [][]geom.Vec3 {
	var c []geom.Vec3
	for _, k := range clicks {
		c = append(c, pl.At2(k.p))
	}
	c = append(c, pl.At2(cur))
	out := [][]geom.Vec3{c}
	if len(c) >= 3 {
		b := geom.Bezier(c...)
		var curve []geom.Vec3
		for i := 0; i <= 64; i++ {
			curve = append(curve, b.Eval(float64(i)/64))
		}
		out = append(out, curve)
	}
	return out
}
