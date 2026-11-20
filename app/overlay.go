package app

import (
	"fmt"
	"math"

	imgui "github.com/AllenDang/cimgui-go/imgui"

	"forma/geom"
	"forma/sketch"
	"forma/ui"
)

func u32(r, g, b, a float32) uint32 {
	return imgui.ColorConvertFloat4ToU32(imgui.Vec4{X: r, Y: g, Z: b, W: a})
}

func vadd(a, b imgui.Vec2) imgui.Vec2           { return imgui.Vec2{X: a.X + b.X, Y: a.Y + b.Y} }
func vsub(a, b imgui.Vec2) imgui.Vec2           { return imgui.Vec2{X: a.X - b.X, Y: a.Y - b.Y} }
func vscale(a imgui.Vec2, s float32) imgui.Vec2 { return imgui.Vec2{X: a.X * s, Y: a.Y * s} }
func vlen(a imgui.Vec2) float32                 { return float32(math.Hypot(float64(a.X), float64(a.Y))) }
func vnorm(a imgui.Vec2) imgui.Vec2 {
	l := vlen(a)
	if l < 1e-6 {
		return imgui.Vec2{X: 1}
	}
	return vscale(a, 1/l)
}
func vperp(a imgui.Vec2) imgui.Vec2 { return imgui.Vec2{X: -a.Y, Y: a.X} }

// drawOverlays draws what ImGui draws over the 3D view: the sketch's
// dimensions and constraint badges, the snap cursor, the tool hint.
func (a *App) drawOverlays(dl *imgui.DrawList) {
	vp := &a.vp
	dl.PushClipRect(vp.pos, vadd(vp.pos, vp.size))
	defer dl.PopClipRect()
	if a.sk != nil {
		a.drawSketchOverlay(dl)
	}
	a.vp.handle.update(a)
	a.vp.handle.draw(a, dl)
	// a little legend of the mouse, bottom left
	hint := "Right drag: orbit · Middle drag: pan · Wheel: zoom · Shift+middle: orbit"
	ts := imgui.CalcTextSize(hint)
	p := imgui.Vec2{X: vp.pos.X + 10, Y: vp.pos.Y + vp.size.Y - ts.Y - 10}
	dl.AddTextVec2(p, u32(0.25, 0.27, 0.32, 0.8), hint)
}

func (a *App) skScreen(s *sketch.Sketch, p geom.Vec2) imgui.Vec2 {
	q, _ := a.toScreen(s.Plane.At2(p))
	return q
}

func (a *App) drawSketchOverlay(dl *imgui.DrawList) {
	sm := a.sk
	s := sm.item.S
	vp := &a.vp
	ink := u32(0.10, 0.12, 0.16, 1)
	dimCol := u32(0.12, 0.20, 0.45, 1)

	// constraint badges (under the dimensions, so labels stay readable)
	for _, c := range s.Cons {
		if c.Kind.Dimensional() || c.Internal {
			continue
		}
		a.drawBadges(dl, s, c)
	}

	// dimensions
	mouse := vadd(vp.pos, vp.mouse)
	sm.hoverDim = 0
	rects := map[int][4]float32{}
	for _, c := range s.Cons {
		if !c.Kind.Dimensional() || c.Internal {
			continue
		}
		label, ok := a.drawDim(dl, s, c, dimCol)
		if !ok {
			continue
		}
		text := dimText(c)
		ts := imgui.CalcTextSize(text)
		pad := imgui.Vec2{X: 5, Y: 2}
		mn := vsub(vsub(label, vscale(ts, 0.5)), pad)
		mx := vadd(vadd(label, vscale(ts, 0.5)), pad)
		hot := mouse.X >= mn.X && mouse.X <= mx.X && mouse.Y >= mn.Y && mouse.Y <= mx.Y && vp.hovered
		if hot {
			sm.hoverDim = c.ID
		}
		sel := a.isSelected(Ref{Kind: KindDim, Item: sm.item.ID, Local: c.ID})
		bg := u32(1, 1, 1, 0.92)
		if sel {
			bg = accentU32(0.95)
		}
		dl.AddRectFilledV(mn, mx, bg, 3, 0)
		border := u32(0.12, 0.20, 0.45, 0.6)
		if hot {
			border = accentU32(1)
		}
		dl.AddRectV(mn, mx, border, 3, 1.2, 0)
		dl.AddTextVec2(vsub(label, vscale(ts, 0.5)), ink, text)
		rects[c.ID] = [4]float32{mn.X, mn.Y, mx.X, mx.Y}
	}
	sm.dimRects = rects
	sm.drawXform(a, dl)

	// what a drag is setting, by the cursor
	if sm.dragging && sm.dragLabel != "" {
		p := vadd(a.vp.pos, a.vp.mouse)
		ts := imgui.CalcTextSize(sm.dragLabel)
		mn := vadd(p, imgui.Vec2{X: 16, Y: 12})
		dl.AddRectFilledV(mn, vadd(mn, imgui.Vec2{X: ts.X + 10, Y: ts.Y + 4}), u32(1, 1, 1, 0.92), 4, 0)
		dl.AddTextVec2(vadd(mn, imgui.Vec2{X: 5, Y: 2}), u32(0.1, 0.12, 0.16, 1), sm.dragLabel)
	}

	// the snap cursor
	if sm.valid && sm.tool != toolSelect && !sm.editOpen {
		p := a.skScreen(s, sm.cur.p)
		col := accentU32(1)
		dl.AddCircleV(p, 5, col, 16, 1.6)
		if sm.cur.label != "" {
			dl.AddTextVec2(vadd(p, imgui.Vec2{X: 10, Y: 6}), u32(0.55, 0.30, 0.05, 1), sm.cur.label)
		}
		if sm.tool == toolLine || sm.tool == toolRect || sm.tool == toolCircle {
			if len(sm.clicks) > 0 {
				var txt string
				q := sm.clicks[0].p
				if sm.tool == toolLine {
					q = s.P(sm.chain)
				}
				switch sm.tool {
				case toolRect:
					txt = fmt.Sprintf("%s × %s", fmtLen(math.Abs(sm.cur.p.X-q.X)), fmtLen(math.Abs(sm.cur.p.Y-q.Y)))
				case toolCircle:
					txt = "Ø" + fmtLen(2*q.Dist(sm.cur.p))
				default:
					txt = fmtLen(q.Dist(sm.cur.p))
				}
				dl.AddTextVec2(vadd(p, imgui.Vec2{X: 10, Y: -18}), ink, txt)
			}
		}
	}

	// the hint and constraint status, top left
	st := s.Status
	status := fmt.Sprintf("%d degrees of freedom", st.DOF)
	stCol := u32(0.10, 0.36, 0.92, 1)
	if st.DOF == 0 && len(s.Ents) > 0 {
		status, stCol = "Fully constrained", u32(0.04, 0.50, 0.20, 1)
	}
	if !st.Converged && len(s.Cons) > 0 {
		status, stCol = "Can't solve", u32(0.8, 0.1, 0.1, 1)
	}
	head := fmt.Sprintf("%s  ·  %s", sm.item.Name, toolNames[sm.tool])
	x, y := vp.pos.X+12, vp.pos.Y+10
	w := imgui.CalcTextSize(toolHints[sm.tool]).X + 20
	dl.AddRectFilledV(imgui.Vec2{X: x - 6, Y: y - 4}, imgui.Vec2{X: x + w, Y: y + 58}, u32(1, 1, 1, 0.78), 5, 0)
	dl.AddTextVec2(imgui.Vec2{X: x, Y: y}, ink, head)
	dl.AddTextVec2(imgui.Vec2{X: x, Y: y + 18}, u32(0.3, 0.32, 0.36, 1), toolHints[sm.tool])
	dl.AddTextVec2(imgui.Vec2{X: x, Y: y + 36}, stCol, status)

	a.dimEditor()
}

// drawDim draws a dimension's lines and returns where its label goes.
func (a *App) drawDim(dl *imgui.DrawList, s *sketch.Sketch, c *sketch.Constraint, col uint32) (imgui.Vec2, bool) {
	arrow := func(tip, dir imgui.Vec2) {
		d := vnorm(dir)
		n := vperp(d)
		b := vsub(tip, vscale(d, 9))
		dl.AddTriangleFilled(tip, vadd(b, vscale(n, 3.5)), vsub(b, vscale(n, 3.5)), col)
	}
	switch c.Kind {
	case sketch.Distance:
		p := s.P(c.E[0])
		var q geom.Vec2
		e1 := s.Ent(c.E[1])
		if e1.Kind == sketch.Line {
			a0, a1 := s.P(e1.Pts[0]), s.P(e1.Pts[1])
			d := a1.Sub(a0)
			t := p.Sub(a0).Dot(d) / math.Max(d.Dot(d), 1e-12)
			q = a0.Add(d.Scale(t))
		} else {
			q = s.P(c.E[1])
		}
		A, B := a.skScreen(s, p), a.skScreen(s, q)
		dir := vsub(B, A)
		if vlen(dir) < 2 {
			return imgui.Vec2{}, false
		}
		n := vperp(vnorm(dir))
		if n.Y > 0 || (n.Y == 0 && n.X < 0) {
			n = vscale(n, -1)
		}
		off := float32(26)
		if e1.Kind == sketch.Line {
			off = 0
		}
		A2, B2 := vadd(A, vscale(n, off)), vadd(B, vscale(n, off))
		if off > 0 {
			dl.AddLineArgs(vadd(A, vscale(n, 4)), vadd(A2, vscale(n, 5)), col, 1)
			dl.AddLineArgs(vadd(B, vscale(n, 4)), vadd(B2, vscale(n, 5)), col, 1)
		}
		dl.AddLineArgs(A2, B2, col, 1.2)
		arrow(A2, vsub(A2, B2))
		arrow(B2, vsub(B2, A2))
		return vscale(vadd(A2, B2), 0.5), true
	case sketch.Radius, sketch.Diameter:
		e := s.Ent(c.E[0])
		cc := s.P(e.Pts[0])
		r := s.Radius(e)
		ang := math.Pi / 4
		if e.Kind == sketch.Arc {
			a0, sw := s.ArcAngles(e)
			ang = a0 + sw/2
		}
		u := geom.Vec2{X: math.Cos(ang), Y: math.Sin(ang)}
		C := a.skScreen(s, cc)
		P := a.skScreen(s, cc.Add(u.Scale(r)))
		if c.Kind == sketch.Diameter {
			Q := a.skScreen(s, cc.Sub(u.Scale(r)))
			dl.AddLineArgs(Q, P, col, 1.2)
			arrow(P, vsub(P, Q))
			arrow(Q, vsub(Q, P))
		} else {
			dl.AddLineArgs(C, P, col, 1.2)
			arrow(P, vsub(P, C))
		}
		d := vnorm(vsub(P, C))
		return vadd(P, vscale(d, 26)), true
	case sketch.Angle:
		l1, l2 := s.Ent(c.E[0]), s.Ent(c.E[1])
		a0, a1 := s.P(l1.Pts[0]), s.P(l1.Pts[1])
		b0, b1 := s.P(l2.Pts[0]), s.P(l2.Pts[1])
		d1, d2 := a1.Sub(a0), b1.Sub(b0)
		den := d1.Cross(d2)
		if math.Abs(den) < 1e-12 {
			return imgui.Vec2{}, false
		}
		t := b0.Sub(a0).Cross(d2) / den
		x := a0.Add(d1.Scale(t))
		X := a.skScreen(s, x)
		u1 := vnorm(vsub(a.skScreen(s, x.Add(d1.Norm())), X))
		u2 := vnorm(vsub(a.skScreen(s, x.Add(d2.Norm())), X))
		ang1 := math.Atan2(float64(u1.Y), float64(u1.X))
		ang2 := math.Atan2(float64(u2.Y), float64(u2.X))
		sweep := math.Remainder(ang2-ang1, 2*math.Pi)
		rad := float32(42)
		n := 24
		prev := vadd(X, vscale(u1, rad))
		for i := 1; i <= n; i++ {
			t := ang1 + sweep*float64(i)/float64(n)
			p := vadd(X, imgui.Vec2{X: rad * float32(math.Cos(t)), Y: rad * float32(math.Sin(t))})
			dl.AddLineArgs(prev, p, col, 1.2)
			prev = p
		}
		mid := ang1 + sweep/2
		return vadd(X, imgui.Vec2{X: (rad + 20) * float32(math.Cos(mid)), Y: (rad + 20) * float32(math.Sin(mid))}), true
	}
	return imgui.Vec2{}, false
}

// a spot along a curve for badges
func (a *App) curveSpot(s *sketch.Sketch, id int, t float64) (geom.Vec2, geom.Vec2, bool) {
	e := s.Ent(id)
	if e == nil {
		return geom.Vec2{}, geom.Vec2{}, false
	}
	switch e.Kind {
	case sketch.Line:
		p0, p1 := s.P(e.Pts[0]), s.P(e.Pts[1])
		return p0.Lerp(p1, t), p1.Sub(p0).Norm().Perp(), true
	case sketch.Circle:
		c := s.P(e.Pts[0])
		ang := math.Pi/2 + t
		u := geom.Vec2{X: math.Cos(ang), Y: math.Sin(ang)}
		return c.Add(u.Scale(s.Radius(e))), u, true
	case sketch.Arc:
		a0, sw := s.ArcAngles(e)
		ang := a0 + sw*t
		u := geom.Vec2{X: math.Cos(ang), Y: math.Sin(ang)}
		return s.P(e.Pts[0]).Add(u.Scale(s.Radius(e))), u, true
	case sketch.Spline:
		p, d := s.SplineAt(e, t)
		return p, d.Perp(), true
	case sketch.Point:
		return s.P(id), geom.Vec2{X: 0.7, Y: 0.7}, true
	}
	return geom.Vec2{}, geom.Vec2{}, false
}

func (a *App) drawBadges(dl *imgui.DrawList, s *sketch.Sketch, c *sketch.Constraint) {
	var text string
	switch c.Kind {
	case sketch.Horizontal:
		text = "H"
	case sketch.Vertical:
		text = "V"
	case sketch.Parallel:
		text = "//"
	case sketch.Perpendicular:
		text = "90°"
	case sketch.Tangent:
		text = "T"
	case sketch.Equal:
		text = "="
	case sketch.Concentric:
		text = "O"
	case sketch.Midpoint:
		text = "M"
	case sketch.Fix:
		text = ui.Icon(ui.FaLock, "")
		if !ui.Icons {
			text = "F"
		}
	default:
		return
	}
	targets := c.E
	if (c.Kind == sketch.Horizontal || c.Kind == sketch.Vertical) && len(c.E) == 2 {
		targets = c.E[:1]
	}
	t := 0.5
	if len(targets) > 1 || c.Kind == sketch.Midpoint {
		t = 0.3
	}
	for _, id := range targets {
		p, n, ok := a.curveSpot(s, id, t)
		if !ok {
			continue
		}
		P := a.skScreen(s, p)
		N := vnorm(vsub(a.skScreen(s, p.Add(n)), P))
		at := vadd(P, vscale(N, 14))
		ts := imgui.CalcTextSize(text)
		mn := vsub(at, imgui.Vec2{X: ts.X/2 + 4, Y: ts.Y/2 + 1})
		mx := vadd(at, imgui.Vec2{X: ts.X/2 + 4, Y: ts.Y/2 + 1})
		dl.AddRectFilledV(mn, mx, u32(0.97, 0.97, 0.99, 0.9), 3, 0)
		dl.AddRectV(mn, mx, u32(0.3, 0.35, 0.45, 0.5), 3, 1, 0)
		dl.AddTextVec2(vsub(at, vscale(ts, 0.5)), u32(0.2, 0.25, 0.35, 1), text)
	}
}

// dimEditor is the little box for typing a dimension's value.
func (a *App) dimEditor() {
	sm := a.sk
	if !sm.editOpen {
		return
	}
	pos := a.vp.pos
	if r, ok := sm.dimRects[sm.editDim]; ok {
		pos = imgui.Vec2{X: r[0], Y: r[3] + 4}
	} else {
		pos = vadd(a.vp.pos, a.vp.mouse)
	}
	imgui.SetNextWindowPosV(pos, imgui.CondAppearing, imgui.Vec2{})
	flags := imgui.WindowFlagsNoDecoration | imgui.WindowFlagsAlwaysAutoResize | imgui.WindowFlagsNoSavedSettings
	if imgui.BeginV("##dimedit", nil, flags) {
		if imgui.IsWindowAppearing() {
			imgui.SetKeyboardFocusHere()
		}
		imgui.SetNextItemWidth(140)
		if imgui.InputTextWithHint("##v", "value or expression", &sm.editBuf, imgui.InputTextFlagsEnterReturnsTrue|imgui.InputTextFlagsAutoSelectAll, nil) {
			sm.applyEdit(a)
		}
		imgui.SameLine()
		imgui.TextDisabled("mm · in · deg · 20+5")
		if imgui.IsKeyPressedBool(imgui.KeyEscape) {
			sm.editOpen = false
		}
		if !imgui.IsWindowFocusedV(imgui.FocusedFlagsRootAndChildWindows) && !imgui.IsWindowAppearing() {
			sm.editOpen = false
		}
	}
	imgui.End()
}
