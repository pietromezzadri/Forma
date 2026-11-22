package app

import (
	"strings"

	imgui "github.com/AllenDang/cimgui-go/imgui"

	"forma/brep"
	"forma/ui"
)

// The toolbar is a ribbon as in Fusion: tools with an icon over a short
// name, in groups captioned underneath (Sketch, Create, Modify, …). When
// the window is too narrow for it the tiles tighten round their names,
// then lose them (icons only, named by their tooltips), and when even that
// doesn't fit it scrolls sideways rather than cut tools off.

type ribbon struct {
	a       *App
	f       float32 // font size
	first   bool
	groups  int
	tight   bool // tiles only as wide as their names
	compact bool // icons only
}

func (r *ribbon) btnH() float32 { return r.f*2 + 12 }
func (r *ribbon) capH() float32 { return r.f + 2 }

// group starts a captioned group of tools.
func (r *ribbon) group() {
	if r.groups > 0 {
		gap := float32(12)
		if r.tight || r.compact {
			gap = 8
		}
		imgui.SameLineV(0, gap)
	}
	r.groups++
	imgui.BeginGroup()
	r.first = true
}

// end closes the group, writing its caption under it and a rule after it.
func (r *ribbon) end(caption string) {
	tw := imgui.CalcTextSize(caption).X
	// at least as wide as its caption (a lone icon is narrower)
	imgui.Dummy(imgui.Vec2{X: tw + 4, Y: r.capH()})
	imgui.EndGroup()
	mn, mx := imgui.ItemRectMin(), imgui.ItemRectMax()
	dl := imgui.WindowDrawList()
	col := imgui.ColorConvertFloat4ToU32(imgui.Vec4{X: 0.55, Y: 0.57, Z: 0.62, W: 1})
	dl.AddTextVec2(imgui.Vec2{X: (mn.X+mx.X)/2 - tw/2, Y: mx.Y - r.capH()}, col, caption)
	rule := imgui.ColorConvertFloat4ToU32(imgui.Vec4{X: 1, Y: 1, Z: 1, W: 0.10})
	dl.AddLineArgs(imgui.Vec2{X: mx.X + 6, Y: mn.Y + 2}, imgui.Vec2{X: mx.X + 6, Y: mx.Y - 2}, rule, 1)
}

func (r *ribbon) next() {
	if !r.first {
		imgui.SameLineV(0, 2)
	}
	r.first = false
}

// tile draws one ribbon tool: icon on top, name under; reports a click.
func (r *ribbon) tile(id, icon, name, tip string, enabled, lit bool) bool {
	r.next()
	w := imgui.CalcTextSize(name).X + 16
	if w < 50 {
		w = 50
	}
	switch {
	case r.compact:
		w = r.f + 18
	case r.tight:
		w = max(imgui.CalcTextSize(name).X+8, r.f+14)
	}
	imgui.BeginDisabledV(!enabled)
	clicked := imgui.InvisibleButton("##rb"+id, imgui.Vec2{X: w, Y: r.btnH()})
	hov, held := imgui.IsItemHovered(), imgui.IsItemActive()
	imgui.EndDisabled()
	mn, mx := imgui.ItemRectMin(), imgui.ItemRectMax()
	dl := imgui.WindowDrawList()
	switch {
	case lit:
		dl.AddRectFilledV(mn, mx, imgui.ColorConvertFloat4ToU32(imgui.Vec4{X: 0.88, Y: 0.62, Z: 0.28, W: 0.45}), 5, 0)
	case held && enabled:
		dl.AddRectFilledV(mn, mx, imgui.ColorU32Col(imgui.ColButtonActive), 5, 0)
	case hov && enabled:
		dl.AddRectFilledV(mn, mx, imgui.ColorU32Col(imgui.ColButtonHovered), 5, 0)
	}
	tc := imgui.ColorU32Col(imgui.ColText)
	if !enabled {
		tc = imgui.ColorU32Col(imgui.ColTextDisabled)
	}
	cx := (mn.X + mx.X) / 2
	iw := imgui.CalcTextSize(icon).X
	if r.compact {
		// the icon in the middle; a menu's arrow under it
		dl.AddTextVec2(imgui.Vec2{X: cx - iw/2, Y: mn.Y + 3 + r.f/2}, tc, icon)
		if arrow := dropArrow(); strings.HasSuffix(name, arrow) {
			a := strings.TrimSpace(arrow)
			aw := imgui.CalcTextSize(a).X
			dl.AddTextVec2(imgui.Vec2{X: cx - aw/2, Y: mn.Y + 5 + r.f*1.5}, tc, a)
		}
		if tip == "" {
			tip = strings.TrimSuffix(name, dropArrow())
		}
	} else {
		nw := imgui.CalcTextSize(name).X
		dl.AddTextVec2(imgui.Vec2{X: cx - iw/2, Y: mn.Y + 5}, tc, icon)
		dl.AddTextVec2(imgui.Vec2{X: cx - nw/2, Y: mn.Y + 7 + r.f}, tc, name)
	}
	if tip != "" && imgui.IsItemHoveredV(imgui.HoveredFlagsAllowWhenDisabled|imgui.HoveredFlagsDelayShort) {
		imgui.SetTooltip(tip)
	}
	return clicked && enabled
}

// cmd is a tile for a command.
func (r *ribbon) cmd(id string, icon rune, name string, lit bool) {
	a := r.a
	c := a.cmds.get(id)
	if c == nil {
		return
	}
	tip := c.Label
	if r.compact && name != "" && !strings.EqualFold(name, c.Label) {
		tip = name + ": " + c.Label
	}
	if c.Shortcut != "" {
		tip += "  (" + c.Shortcut + ")"
	}
	if r.tile(id, iconText(icon, name), name, tip, a.cmds.enabled(id), lit) {
		c.Run("")
	}
	a.markItem("btn:" + id)
}

// menu is a tile that opens a list of commands under it.
func (r *ribbon) menu(id string, icon rune, name, tip string, items [][2]string) {
	a := r.a
	any := false
	for _, it := range items {
		any = any || a.cmds.enabled(it[0])
	}
	if r.tile(id, iconText(icon, name), name+dropArrow(), tip, any, false) {
		imgui.OpenPopupStr("##menu" + id)
	}
	a.markItem("btn:" + id)
	mn, mx := imgui.ItemRectMin(), imgui.ItemRectMax()
	imgui.SetNextWindowPosV(imgui.Vec2{X: mn.X, Y: mx.Y + 2}, imgui.CondAlways, imgui.Vec2{})
	if imgui.BeginPopup("##menu" + id) {
		for _, it := range items {
			c := a.cmds.get(it[0])
			if c == nil {
				continue
			}
			label := it[1]
			if label == "" {
				label = c.Label
			}
			if imgui.MenuItemBoolV(ui.Icon(iconOr(c.Icon), label), c.Shortcut, false, a.cmds.enabled(it[0])) {
				c.Run("")
			}
			a.markItem("btn:" + it[0])
		}
		imgui.EndPopup()
	}
}

// small is a compact one-line button (constraints), two rows to a tile's height.
func (r *ribbon) small(id, name string) {
	a := r.a
	c := a.cmds.get(id)
	if c == nil {
		return
	}
	en := a.cmds.enabled(id)
	imgui.BeginDisabledV(!en)
	clicked := imgui.ButtonV(name+"##"+id, imgui.Vec2{X: 0, Y: r.btnH()/2 - 1})
	imgui.EndDisabled()
	tip := c.Label
	if c.Shortcut != "" {
		tip += "  (" + c.Shortcut + ")"
	}
	imgui.SetItemTooltip(tip)
	a.markItem("btn:" + id)
	if clicked && en {
		c.Run("")
	}
}

func iconText(icon rune, name string) string {
	if icon == 0 || !ui.Icons {
		if len(name) > 0 {
			return name[:1]
		}
		return " "
	}
	return string(icon)
}

func (a *App) toolbar() {
	r := &ribbon{a: a, f: imgui.FontSize()}
	pad := imgui.CurrentStyle().WindowPadding().Y
	h := r.btnH() + r.capH() + 2*pad + 4
	// which ribbon, and how wide it came out last time (full and compact)
	kind := "part"
	switch {
	case a.sk != nil:
		kind = "sketch"
	case a.dw != nil:
		kind = "drawing"
	}
	if a.ribbonW == nil {
		a.ribbonW = map[string]float32{}
	}
	avail := imgui.MainViewport().WorkSize().X - 2*imgui.CurrentStyle().WindowPadding().X - 4
	// the first of full, tight and compact known to fit (one not yet
	// measured is drawn to measure it)
	levels := []string{kind, kind + "/tight", kind + "/compact"}
	level := 0
	for level < 2 {
		w, ok := a.ribbonW[levels[level]]
		if !ok || w <= avail {
			break
		}
		level++
	}
	r.tight, r.compact = level == 1, level == 2
	scroll := level == 2 && a.ribbonW[levels[2]] > avail
	flags := imgui.WindowFlagsNoScrollbar | imgui.WindowFlagsNoSavedSettings | imgui.WindowFlagsNoTitleBar
	if scroll {
		flags = imgui.WindowFlagsHorizontalScrollbar | imgui.WindowFlagsNoSavedSettings | imgui.WindowFlagsNoTitleBar
		h += imgui.CurrentStyle().ScrollbarSize()
	}
	imgui.PushStyleVarVec2(imgui.StyleVarItemSpacing, imgui.Vec2{X: 4, Y: 2})
	if imgui.InternalBeginViewportSideBar("##toolbar", imgui.MainViewport(), imgui.DirUp, h, flags) {
		x0 := imgui.CursorScreenPos().X
		switch kind {
		case "drawing":
			a.drawingRibbon(r)
		case "part":
			a.partRibbon(r)
		default:
			a.sketchRibbon(r)
		}
		a.ribbonW[levels[level]] = imgui.ItemRectMax().X - x0
	}
	imgui.End()
	imgui.PopStyleVar()
}

func (a *App) partRibbon(r *ribbon) {
	r.group()
	r.cmd("edit.undo", 0xf0e2, "Undo", false)
	r.cmd("edit.redo", 0xf01e, "Redo", false)
	r.cmd("file.save", ui.FaFloppy, "Save", false)
	r.cmd("file.import", icImport, "Import", false)
	r.cmd("file.export", ui.FaDownload, "Export", false)
	r.cmd("drawing.open", icDrawing, "Drawing", false)
	r.end("File")

	r.group()
	// with a flat face picked, straight onto it; else a choice of planes
	if _, onFace := a.planeFor(""); onFace {
		r.cmd("sketch.face", ui.FaPen, "Sketch", false)
	} else {
		r.menu("sketch.face", ui.FaPen, "Sketch", "New sketch (pick a flat face first to sketch on it)", [][2]string{
			{"sketch.xy", "On XY (top)"}, {"sketch.xz", "On XZ (front)"}, {"sketch.yz", "On YZ (right)"},
		})
	}
	r.end("Sketch")

	r.group()
	r.cmd("feature.extrude", 0xf062, "Extrude", false)
	r.cmd("feature.revolve", ui.FaRotate, "Revolve", false)
	r.cmd("feature.hole", icHole, "Hole", false)
	var prims [][2]string
	for _, k := range PrimKinds {
		prims = append(prims, [2]string{"create." + k, ""})
	}
	r.menu("create.prim", ui.FaCube, "Primitive", "Box, cylinder, cone, sphere, torus", prims)
	r.end("Create")

	r.group()
	r.cmd("feature.fillet", 0xf1ce, "Fillet", false)
	r.cmd("feature.chamfer", 0xf5cb, "Chamfer", false)
	r.cmd("feature.shell", 0xf49e, "Shell", false)
	r.cmd("feature.emboss", icEmboss, "Emboss", false)
	r.cmd("feature.combine", icCombine, "Combine", false)
	r.cmd("feature.move", icMove, "Move", false)
	r.end("Modify")

	r.group()
	r.cmd("feature.mirror", icMirror, "Mirror", false)
	r.cmd("feature.pattern", icPattern, "Linear", false)
	r.cmd("feature.circular", 0xf2f9, "Circular", false)
	r.end("Pattern")

	r.group()
	r.cmd("assemble.component", icComponent, "Component", false)
	r.cmd("assemble.copy", icCopy, "Copy", false)
	r.cmd("assemble.joint", icJoint, "Joint", false)
	r.cmd("assemble.interference", icInterference, "Clash", a.asm.showClashes)
	r.cmd("assemble.explode", icExplode, "Explode", a.m13.showExplode)
	r.cmd("assemble.bom", icBOM, "BOM", a.m13.showBOM)
	r.cmd("electronics.open", icCircuit, "Circuit", a.elec.open)
	r.end("Assemble")

	r.group()
	r.cmd("view.section", icSection, "Section", a.section.on)
	r.cmd("inspect.mass", 0xf5cd, "Mass", false)
	r.cmd("simulate.open", icStress, "Stress", a.sim.open)
	r.cmd("view.params", 0xf1de, "Params", a.showParams)
	r.end("Inspect")

	r.group()
	r.cmd("view.fit", ui.FaExpand, "Fit", false)
	r.cmd("view.iso", ui.FaHouse, "Home", false)
	r.cmd("view.ortho", 0xf546, "Ortho", a.cam.Ortho)
	r.cmd("view.xray", icXRay, "X-ray", a.xrayLit())
	r.end("View")

	r.group()
	r.cmd("palette", ui.FaSearch, "Search", false)
	r.end("More")
}

func (a *App) sketchRibbon(r *ribbon) {
	// read the sketch state first: a button below may end the sketch
	tool, grid := a.sk.tool, !a.snap.off
	r.group()
	r.cmd("sketch.finish", ui.FaCheck, "Finish", false)
	r.cmd("edit.undo", 0xf0e2, "Undo", false)
	r.cmd("edit.redo", 0xf01e, "Redo", false)
	r.end("Sketch")

	r.group()
	for _, t := range []struct {
		id   string
		t    sketchTool
		icon rune
		name string
	}{{"select", toolSelect, 0xf245, "Select"}, {"line", toolLine, ui.FaSlash, "Line"}, {"rect", toolRect, ui.FaSquare, "Rectangle"},
		{"circle", toolCircle, ui.FaCircle, "Circle"}, {"arc", toolArc, ui.FaBezier, "Arc"}, {"point", toolPoint, ui.FaDotCircle, "Point"}, {"spline", toolSpline, ui.FaBezier, "Spline"}, {"text", toolText, 0xf031, "Text"}, {"project", toolProject, 0xf247, "Project"},
		{"dim", toolDim, ui.FaRuler, "Dimension"}} {
		r.cmd("tool."+t.id, t.icon, t.name, tool == t.t)
	}
	r.cmd("sketch.offset", icOffset, "Offset", false)
	r.end("Draw")

	// constraints: two rows of compact buttons
	r.group()
	rows := [][]struct{ id, short string }{
		{{"coincident", "Coincident"}, {"horizontal", "Horizontal"}, {"vertical", "Vertical"}, {"parallel", "Parallel"}, {"perpendicular", "Perpendicular"}},
		{{"tangent", "Tangent"}, {"equal", "Equal"}, {"concentric", "Concentric"}, {"midpoint", "Midpoint"}, {"fix", "Fix"}},
	}
	for _, row := range rows {
		for j, c := range row {
			if j > 0 {
				imgui.SameLineV(0, 2)
			}
			r.small("constrain."+c.id, c.short)
		}
	}
	r.end("Constrain")

	r.group()
	r.cmd("sketch.construction", 0xf0c5, "Construction", false)
	r.cmd("sketch.gridsnap", 0xf00a, "Grid snap", grid)
	r.end("Options")

	r.group()
	r.cmd("view.fit", ui.FaExpand, "Fit", false)
	r.cmd("palette", ui.FaSearch, "Search", false)
	r.end("View")
}

// massOf works a body's mass properties out and picks it, so Properties
// shows them.
func (a *App) massOf(b *BodyItem) {
	if b.mass == nil {
		mp := brep.Mass(b.Body, 2e-5)
		b.mass = &mp
	}
	a.browserSel = b.ID
}

// dropArrow marks a tile that opens a menu (the UI font has no ▾).
func dropArrow() string {
	if ui.Icons {
		return " " + string(rune(0xf0d7))
	}
	return " v"
}
