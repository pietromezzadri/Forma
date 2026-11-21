package app

import (
	"math"

	imgui "github.com/AllenDang/cimgui-go/imgui"
	"github.com/AllenDang/cimgui-go/imguizmo"
	"github.com/go-gl/mathgl/mgl32"

	"forma/doc"
	"forma/geom"
	"forma/render"
	"forma/sketch"
	"forma/ui"
)

// Colours of the viewport (linear-ish RGBA; faces are lit in the shader).
var (
	colBgTop    = mgl32.Vec4{0.74, 0.78, 0.84, 1}
	colBgBottom = mgl32.Vec4{0.95, 0.96, 0.97, 1}
	colBody     = mgl32.Vec4{0.70, 0.73, 0.78, 1}
	colEdge     = mgl32.Vec4{0.07, 0.08, 0.10, 1}
	colHover    = mgl32.Vec4{1.00, 0.62, 0.18, 1}
	colSelect   = mgl32.Vec4{0.98, 0.46, 0.10, 1}
	colFree     = mgl32.Vec4{0.10, 0.36, 0.92, 1} // under-constrained sketch geometry
	colFixed    = mgl32.Vec4{0.04, 0.50, 0.20, 1} // fully constrained
	colConstr   = mgl32.Vec4{0.85, 0.45, 0.10, 0.9}
	colRegion   = mgl32.Vec4{0.20, 0.45, 0.95, 0.10}
	colRegionHi = mgl32.Vec4{0.20, 0.45, 0.95, 0.30}
)

// viewport is the 3D view's window state.
type viewport struct {
	pos, size imgui.Vec2 // image rect in UI units
	px, py    int        // framebuffer pixels
	scale     float32    // framebuffer pixels per UI unit
	hovered   bool
	mouse     imgui.Vec2 // cursor, UI units from the image's corner
	view      render.View
	viewProj  mgl32.Mat4
	slots     slots
	depth     float32 // depth under the cursor (1 = nothing)
	cursor3   geom.Vec3
	hasCursor bool

	dragBtn   int // 0 none, 1 orbit, 2 pan, 3 left (tools / box)
	dragFrom  imgui.Vec2
	dragMoved bool
	pivot     geom.Vec3
	leftDown  imgui.Vec2
	leftHeld  bool
	wholeBody bool // a double click picked a whole body: its release keeps that

	cubeHot   bool
	gizmoHot  bool // the cursor is on the move gizmo
	gizmoDrag bool
	handle    handleSet

	// a capture places the cursor here (UI units in the image), nil = the mouse
	virtual *imgui.Vec2
}

func (a *App) aspect() float64 {
	if a.vp.py == 0 {
		return 1
	}
	return float64(a.vp.px) / float64(a.vp.py)
}

// computeView fills vp.view from the camera.
func (a *App) computeView() {
	v := &a.vp.view
	v.W, v.H = a.vp.px, a.vp.py
	v.View = a.cam.ViewMatrix()
	v.Proj = a.cam.Proj(a.aspect(), a.doc.Bounds())
	v.Ortho = a.cam.Ortho
	v.Eye = a.cam.Eye()
	v.KeyDir = mgl32.Vec3{0.35, 0.75, 0.55}.Normalize()
	v.Section = a.section.on
	c := a.section.clip(v.Eye)
	v.Clip = mgl32.Vec4{c[0], c[1], c[2], c[3]}
	a.vp.viewProj = v.Proj.Mul4(v.View)
}

// toScreen projects a world point to UI coordinates (absolute), and whether
// it is in front of the camera.
func (a *App) toScreen(p geom.Vec3) (imgui.Vec2, bool) {
	c := a.vp.viewProj.Mul4x1(mgl32.Vec4{float32(p.X), float32(p.Y), float32(p.Z), 1})
	if c[3] <= 1e-6 {
		return imgui.Vec2{}, false
	}
	x := (c[0]/c[3]*0.5 + 0.5) * a.vp.size.X
	y := (1 - (c[1]/c[3]*0.5 + 0.5)) * a.vp.size.Y
	return imgui.Vec2{X: a.vp.pos.X + x, Y: a.vp.pos.Y + y}, true
}

// ndc of a UI point in the image.
func (a *App) ndc(m imgui.Vec2) (float64, float64) {
	return float64(m.X/a.vp.size.X*2 - 1), float64(1 - m.Y/a.vp.size.Y*2)
}

// mouseRay is the camera ray through the cursor.
func (a *App) mouseRay() (geom.Vec3, geom.Vec3) {
	x, y := a.ndc(a.vp.mouse)
	return a.cam.Ray(x, y, a.aspect())
}

// unproject turns the cursor and a depth from the ID buffer into a world point.
func (a *App) unproject(m imgui.Vec2, depth float32) geom.Vec3 {
	x, y := a.ndc(m)
	inv := a.vp.viewProj.Inv()
	p := inv.Mul4x1(mgl32.Vec4{float32(x), float32(y), depth*2 - 1, 1})
	return geom.V3(float64(p[0]/p[3]), float64(p[1]/p[3]), float64(p[2]/p[3]))
}

// rayPlane intersects the cursor ray with a plane.
func (a *App) rayPlane(f geom.Frame) (geom.Vec3, bool) {
	o, d := a.mouseRay()
	den := d.Dot(f.Z)
	if math.Abs(den) < 1e-9 {
		return geom.Vec3{}, false
	}
	t := f.O.Sub(o).Dot(f.Z) / den
	return o.Add(d.Scale(t)), true
}

// gizmo space (Y up) to world (Z up)
var gizmoToWorld = mgl32.Mat4{1, 0, 0, 0, 0, 0, 1, 0, 0, -1, 0, 0, 0, 0, 0, 1}

func (a *App) viewportWindow() {
	imgui.PushStyleVarVec2(imgui.StyleVarWindowPadding, imgui.Vec2{})
	open := imgui.BeginV("Viewport", nil, imgui.WindowFlagsNoScrollbar|imgui.WindowFlagsNoScrollWithMouse)
	imgui.PopStyleVar()
	if !open {
		imgui.End()
		return
	}
	vp := &a.vp
	vp.pos = imgui.CursorScreenPos()
	vp.size = imgui.ContentRegionAvail()
	if vp.size.X < 8 || vp.size.Y < 8 {
		imgui.End()
		return
	}
	fbw, _ := a.win.GetFramebufferSize()
	ww, _ := a.win.GetSize()
	vp.scale = 1
	if ww > 0 {
		vp.scale = float32(fbw) / float32(ww)
	}
	vp.px, vp.py = int(vp.size.X*vp.scale), int(vp.size.Y*vp.scale)
	a.r.Resize(vp.px, vp.py)
	tex := imgui.NewTextureRefTextureID(imgui.TextureID(a.r.Color.Texture))
	imgui.ImageV(*tex, vp.size, imgui.Vec2{X: 0, Y: 1}, imgui.Vec2{X: 1, Y: 0})
	vp.hovered = imgui.IsItemHovered()
	if vp.virtual != nil {
		vp.mouse = *vp.virtual
		vp.hovered = true
	} else {
		m := imgui.MousePos()
		vp.mouse = imgui.Vec2{X: m.X - vp.pos.X, Y: m.Y - vp.pos.Y}
	}

	a.computeView()
	dl := imgui.WindowDrawList()
	a.viewCube(dl)
	a.computeView()
	a.moveGizmo(dl)
	if vp.virtual == nil && !vp.cubeHot && !vp.gizmoHot && !vp.gizmoDrag {
		a.viewportInput()
	}
	a.computeView()
	a.drawOverlays(dl)
	imgui.End()
	a.section.panel(a)
	a.modeBar()
}

// viewCube draws ImGuizmo's view cube in the top right corner and takes the
// camera from it when it is used.
func (a *App) viewCube(dl *imgui.DrawList) {
	vp := &a.vp
	size := float32(110)
	pos := imgui.Vec2{X: vp.pos.X + vp.size.X - size - 8, Y: vp.pos.Y + 8}
	m := a.vp.view.View.Mul4(gizmoToWorld)
	before := m
	imguizmo.SetDrawlistV(dl)
	imguizmo.ViewManipulateFloat(&m[0], float32(a.cam.Dist), pos, imgui.Vec2{X: size, Y: size}, 0x00000000)
	mp := imgui.MousePos()
	vp.cubeHot = mp.X >= pos.X && mp.X <= pos.X+size && mp.Y >= pos.Y && mp.Y <= pos.Y+size || imguizmo.IsUsingViewManipulate()
	if m == before {
		return
	}
	view := m.Mul4(gizmoToWorld.Transpose())
	inv := view.Inv()
	eye := geom.V3(float64(inv[12]), float64(inv[13]), float64(inv[14]))
	fwd := geom.V3(float64(-inv[8]), float64(-inv[9]), float64(-inv[10])).Norm()
	up := geom.V3(float64(inv[4]), float64(inv[5]), float64(inv[6]))
	c := a.cam
	c.anim = nil
	c.Pitch = math.Asin(math.Max(-1, math.Min(1, fwd.Z)))
	if math.Abs(fwd.Z) > 0.9999 {
		if fwd.Z < 0 {
			c.Yaw = math.Atan2(up.Y, up.X)
		} else {
			c.Yaw = math.Atan2(-up.Y, -up.X)
		}
	} else {
		c.Yaw = math.Atan2(fwd.Y, fwd.X)
	}
	c.Target = eye.Add(fwd.Scale(c.Dist))
}

// viewportInput handles the mouse over the view: orbit (right drag, or
// Shift + middle), pan (middle drag), zoom (wheel, toward the cursor), and
// left clicks for selection or the sketch tool.
func (a *App) viewportInput() {
	vp := &a.vp
	io := imgui.CurrentIO()
	if vp.hovered {
		if w := io.MouseWheel(); w != 0 {
			at := a.cam.Target
			if vp.hasCursor {
				at = vp.cursor3
			} else if p, ok := a.rayPlane(geom.FrameZ(a.cam.Target, a.cam.Eye().Sub(a.cam.Target), geom.XAxis)); ok {
				at = p
			}
			a.cam.Zoom(math.Pow(0.85, float64(w)), at)
		}
		startDrag := func(btn int) {
			vp.dragBtn, vp.dragFrom, vp.dragMoved = btn, vp.mouse, false
			vp.pivot = a.cam.Target
			if vp.hasCursor {
				vp.pivot = vp.cursor3
			}
		}
		if imgui.IsMouseClickedBool(imgui.MouseButtonRight) {
			startDrag(1)
		}
		if imgui.IsMouseClickedBool(imgui.MouseButtonMiddle) {
			if io.KeyShift() {
				startDrag(1)
			} else {
				startDrag(2)
			}
		}
		if imgui.IsMouseClickedBool(imgui.MouseButtonLeft) {
			vp.leftDown, vp.leftHeld = vp.mouse, true
			a.leftPress()
		}
	}
	if vp.dragBtn != 0 {
		d := io.MouseDelta()
		if d.X != 0 || d.Y != 0 {
			vp.dragMoved = true
		}
		switch vp.dragBtn {
		case 1:
			a.cam.Orbit(-float64(d.X)*0.006, -float64(d.Y)*0.006, vp.pivot)
		case 2:
			a.cam.Pan(float64(d.X*vp.scale), float64(d.Y*vp.scale), vp.py)
		}
		if !imgui.IsMouseDown(imgui.MouseButtonRight) && !imgui.IsMouseDown(imgui.MouseButtonMiddle) {
			vp.dragBtn = 0
		}
	}
	if vp.leftHeld {
		if imgui.IsMouseDown(imgui.MouseButtonLeft) {
			a.leftDrag()
		} else {
			vp.leftHeld = false
			moved := math.Hypot(float64(vp.mouse.X-vp.leftDown.X), float64(vp.mouse.Y-vp.leftDown.Y)) > 4
			a.leftRelease(moved)
		}
	}
	if vp.hovered && imgui.IsMouseDoubleClicked(imgui.MouseButtonLeft) {
		a.doubleClick()
	}
}

func (a *App) leftPress() {
	if a.vp.handle.press() {
		return
	}
	if a.sk != nil {
		a.sk.press(a)
	}
}

func (a *App) leftDrag() {
	if a.vp.handle.dragging {
		a.vp.handle.drag(a)
		return
	}
	if a.sk != nil {
		a.sk.drag(a)
	}
}

func (a *App) leftRelease(moved bool) {
	if a.vp.handle.release(a) {
		return
	}
	if a.sk != nil && a.sk.release(a, moved) {
		return
	}
	if moved {
		return
	}
	if a.vp.wholeBody {
		a.vp.wholeBody = false
		return
	}
	io := imgui.CurrentIO()
	add := io.KeyCtrl() || io.KeyShift()
	a.toggleSelect(a.hover, add)
	if a.sk == nil && !add {
		// a click on a body picks the body too, so its gizmo comes up; a click
		// on nothing lets it go
		switch a.hover.Kind {
		case KindFace, KindEdge, KindVertex:
			if a.doc.Body(a.hover.Item) != nil {
				a.browserSel = a.hover.Item
			}
		case KindNone:
			if a.doc.Body(a.browserSel) != nil {
				a.browserSel = 0
			}
		}
	}
}

func (a *App) doubleClick() {
	if a.sk != nil {
		a.sk.doubleClick(a)
		return
	}
	// a double click on a body selects the whole of it: every face
	switch a.hover.Kind {
	case KindFace, KindEdge, KindVertex:
		if b := a.doc.Body(a.hover.Item); b != nil {
			a.sel = a.sel[:0]
			for _, f := range b.Body.Faces() {
				a.sel = append(a.sel, Ref{Kind: KindFace, Item: b.ID, Local: f.ID})
			}
			a.browserSel = b.ID
			a.vp.wholeBody = true
		}
	}
}

// renderScene draws the frame into the viewport target and the ID buffer,
// then reads what is under the cursor.
func (a *App) renderScene() {
	vp := &a.vp
	if vp.px == 0 || vp.py == 0 {
		return
	}
	a.computeView()
	v := vp.view
	r := a.r
	px := vp.scale
	wpp := float32(a.cam.WorldPerPixel(vp.py))

	r.BeginColor(colBgTop, colBgBottom)
	inSketch := a.sk != nil
	for _, b := range a.doc.Bodies {
		if !b.Visible {
			continue
		}
		g := b.upload()
		off := a.explodeOffset(doc.ExplodeGroup(b.Comp, b.ID))
		if g == nil {
			continue
		}
		col := colBody
		if inSketch {
			col[3] = 0.25
		}
		hoverFace := uint32(0)
		if a.hover.Kind == KindFace && a.hover.Item == b.ID {
			hoverFace = uint32(a.hover.Local)
		}
		r.DrawFaces(v, g.faces, render.Paint{Offset: off, Color: col, HoverColor: mgl32.Vec4{1, 0.65, 0.25, 0.45}, Hover: hoverFace}, false)
		for _, s := range a.sel {
			if s.Kind == KindFace && s.Item == b.ID {
				r.DrawFaceRange(v, g.faces, uint32(s.Local), render.Paint{Offset: off, Color: mgl32.Vec4{0.98, 0.55, 0.2, 1}})
			}
		}
		hoverEdge := uint32(0)
		if a.hover.Kind == KindEdge && a.hover.Item == b.ID {
			hoverEdge = uint32(a.hover.Local)
		}
		ec := colEdge
		if inSketch {
			ec[3] = 0.35
		}
		r.DrawLines(v, g.edges, render.Paint{Offset: off, Color: ec, HoverColor: colHover, Hover: hoverEdge, Width: 1.6 * px, DepthBias: 1e-5}, false)
		for _, s := range a.sel {
			if s.Kind == KindEdge && s.Item == b.ID {
				r.DrawLineRange(v, g.edges, uint32(s.Local), render.Paint{Offset: off, Color: colSelect, Width: 3.2 * px, DepthBias: 2e-5})
			}
		}
		for _, ref := range append([]Ref{a.hover}, a.sel...) {
			if ref.Kind == KindVertex && ref.Item == b.ID {
				c := colSelect
				if ref == a.hover {
					c = colHover
				}
				r.DrawPoint(v, g.verts, uint32(ref.Local), render.Paint{Offset: off, Color: c, Width: 9 * px, DepthBias: 3e-5})
			}
		}
	}

	a.drawMeshes(v, inSketch)
	a.drawClashes(v)

	if a.showGrid {
		r.DrawGrid(v, a.gridSpec(wpp))
	}

	for _, s := range a.doc.Sketches {
		if !s.Visible && (a.sk == nil || a.sk.item != s) {
			continue
		}
		a.drawSketch(s, v, wpp)
	}
	if a.sk != nil {
		a.sk.drawPreview(a, v)
	}
	r.EndColor()

	// the ID buffer
	vp.slots.reset()
	r.BeginPick()
	// bodies are picked outside sketches, and in one while projecting
	if !inSketch || a.sk.tool == toolProject {
		for _, b := range a.doc.Bodies {
			if !b.Visible || b.gpu == nil {
				continue
			}
			slot := vp.slots.slot(b.ID)
			off := a.explodeOffset(doc.ExplodeGroup(b.Comp, b.ID))
			r.DrawFaces(v, b.gpu.faces, render.Paint{Offset: off, IDBase: idBase(KindFace, slot)}, true)
			r.DrawLines(v, b.gpu.edges, render.Paint{Offset: off, IDBase: idBase(KindEdge, slot), Width: 7 * px, DepthBias: 1e-5}, true)
			r.DrawPoints(v, b.gpu.verts, render.Paint{Offset: off, IDBase: idBase(KindVertex, slot), Width: 12 * px, DepthBias: 2e-5}, true)
		}
	}
	for _, s := range a.doc.Sketches {
		if s.gpu == nil || (inSketch && a.sk.item != s) || (!inSketch && !s.Visible) || (inSketch && a.sk.tool == toolProject) {
			continue
		}
		slot := vp.slots.slot(s.ID)
		r.DrawFlat(v, s.gpu.regions, render.Paint{IDBase: idBase(KindSkRegion, slot)}, true)
		r.DrawLines(v, s.gpu.curves, render.Paint{IDBase: idBase(KindSkCurve, slot), Width: 8 * px, DepthBias: 3e-5}, true)
		r.DrawLines(v, s.gpu.constr, render.Paint{IDBase: idBase(KindSkCurve, slot), Width: 8 * px, DepthBias: 3e-5}, true)
		r.DrawPoints(v, s.gpu.points, render.Paint{IDBase: idBase(KindSkPoint, slot), Width: 13 * px, DepthBias: 4e-5}, true)
	}
	a.readPick()
	r.EndPick()
}

// readPick finds the best thing in a small square round the cursor.
func (a *App) readPick() {
	vp := &a.vp
	if !vp.hovered {
		a.hover = Ref{}
		vp.hasCursor = false
		return
	}
	x, y := int(vp.mouse.X*vp.scale), int(vp.mouse.Y*vp.scale)
	rad := int(5 * vp.scale)
	ids, n, depth := a.r.Pick.Read(x, y, rad)
	vp.depth = depth
	vp.hasCursor = depth < 1
	if vp.hasCursor {
		vp.cursor3 = a.unproject(vp.mouse, depth)
	}
	best, bestPri, bestD := uint32(0), 0, math.Inf(1)
	for i, id := range ids {
		if id == 0 {
			continue
		}
		k, _, _ := decodeID(id)
		pri := pickPriority(k)
		dx, dy := float64(i%n-rad), float64(i/n-rad)
		d := dx*dx + dy*dy
		// faces and profiles only count right under the cursor
		if pri == 1 && d > 0 {
			continue
		}
		if pri > bestPri || (pri == bestPri && d < bestD) {
			best, bestPri, bestD = id, pri, d
		}
	}
	if best == 0 {
		a.hover = Ref{}
		return
	}
	k, slot, local := decodeID(best)
	a.hover = Ref{Kind: k, Item: vp.slots.item(slot), Local: local}
}

// gridSpec sizes the grid to the zoom: minor lines ~24 px apart.
func (a *App) gridSpec(wpp float32) render.Grid {
	step := niceStep(float64(wpp) * 24 * float64(a.vp.scale))
	plane := geom.World
	if a.sk != nil {
		plane = a.sk.item.S.Plane
	}
	fade := float32(a.cam.Dist * 8)
	if a.cam.Ortho {
		fade = float32(a.cam.Dist * 40)
	}
	return render.Grid{
		Plane: plane, Step: float32(step), Fade: fade,
		Minor:      mgl32.Vec4{0.1, 0.12, 0.16, 0.14},
		Major:      mgl32.Vec4{0.1, 0.12, 0.16, 0.30},
		AxisXColor: mgl32.Vec4{0.85, 0.18, 0.18, 0.85},
		AxisYColor: mgl32.Vec4{0.15, 0.62, 0.22, 0.85},
	}
}

// drawSketch draws a sketch's profiles, curves (blue free, green fixed,
// dashed orange construction) and points.
func (a *App) drawSketch(s *SketchItem, v render.View, wpp float32) {
	g := s.upload(float64(wpp) * 0.5)
	r := a.r
	px := a.vp.scale
	active := a.sk != nil && a.sk.item == s
	st := s.S.Status
	hoverRegion := uint32(0)
	if a.hover.Kind == KindSkRegion && a.hover.Item == s.ID {
		hoverRegion = uint32(a.hover.Local)
	}
	r.DrawFlat(v, g.regions, render.Paint{Color: colRegion, HoverColor: colRegionHi, Hover: hoverRegion}, false)
	for _, ref := range a.sel {
		if ref.Kind == KindSkRegion && ref.Item == s.ID {
			r.DrawFlat(v, g.regions, render.Paint{Color: mgl32.Vec4{}, HoverColor: mgl32.Vec4{0.98, 0.55, 0.2, 0.35}, Hover: uint32(ref.Local)}, false)
		}
	}
	width := 2.0 * px
	if !active {
		width = 1.6 * px
	}
	hoverCurve := uint32(0)
	if a.hover.Kind == KindSkCurve && a.hover.Item == s.ID {
		hoverCurve = uint32(a.hover.Local)
	}
	// curves: draw all in the free colour, then the fixed ones over them
	r.DrawLines(v, g.curves, render.Paint{Color: colFree, HoverColor: colHover, Hover: hoverCurve, Width: width, DepthBias: 2e-5}, false)
	for _, e := range s.S.Ents {
		if e.Kind != sketch.Point && !e.Construction && st.Fixed[e.ID] && uint32(e.ID) != hoverCurve {
			r.DrawLineRange(v, g.curves, uint32(e.ID), render.Paint{Color: colFixed, Width: width, DepthBias: 2.5e-5})
		}
		// projected from a body: purple
		if e.Kind != sketch.Point && !e.Construction && e.Proj != "" && uint32(e.ID) != hoverCurve {
			r.DrawLineRange(v, g.curves, uint32(e.ID), render.Paint{Color: mgl32.Vec4{0.62, 0.32, 0.85, 1}, Width: width, DepthBias: 2.6e-5})
		}
	}
	r.DrawLines(v, g.constr, render.Paint{Color: colConstr, HoverColor: colHover, Hover: hoverCurve, Width: 1.4 * px, Dash: wpp * 10 * px, DepthBias: 2e-5}, false)
	for _, ref := range a.sel {
		if ref.Kind == KindSkCurve && ref.Item == s.ID {
			r.DrawLineRange(v, g.curves, uint32(ref.Local), render.Paint{Color: colSelect, Width: width + 1.5*px, DepthBias: 3e-5})
			r.DrawLineRange(v, g.constr, uint32(ref.Local), render.Paint{Color: colSelect, Width: width + 1*px, DepthBias: 3e-5})
		}
	}
	if !active && a.hover.Item != s.ID {
		return
	}
	hoverPt := uint32(0)
	if a.hover.Kind == KindSkPoint && a.hover.Item == s.ID {
		hoverPt = uint32(a.hover.Local)
	}
	r.DrawPoints(v, g.points, render.Paint{Color: colFree, HoverColor: colHover, Hover: hoverPt, Width: 7 * px, DepthBias: 4e-5}, false)
	for _, e := range s.S.Ents {
		if e.Kind == sketch.Point && st.Fixed[e.ID] && uint32(e.ID) != hoverPt {
			r.DrawPoint(v, g.points, uint32(e.ID), render.Paint{Color: colFixed, Width: 7 * px, DepthBias: 4.5e-5})
		}
	}
	for _, ref := range a.sel {
		if ref.Kind == KindSkPoint && ref.Item == s.ID {
			r.DrawPoint(v, g.points, uint32(ref.Local), render.Paint{Color: colSelect, Width: 10 * px, DepthBias: 5e-5})
		}
	}
}

// accentU32 is the UI accent for overlays.
func accentU32(alpha float32) uint32 {
	c := ui.Accent
	c.W = alpha
	return imgui.ColorConvertFloat4ToU32(c)
}

// accentVec4 is the UI accent as a colour.
func accentVec4(alpha float32) imgui.Vec4 {
	c := ui.Accent
	c.W = alpha
	return c
}
