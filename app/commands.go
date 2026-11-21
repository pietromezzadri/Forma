package app

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	imgui "github.com/AllenDang/cimgui-go/imgui"

	"forma/doc"
	"forma/sketch"
	"forma/ui"
)

// Command is one action, shared by the menus, the toolbar, the command
// palette, shortcuts and capture scripts.
type Command struct {
	ID       string
	Label    string
	Group    string
	Shortcut string
	Icon     rune
	Run      func(arg string)
	Enabled  func() bool
}

type Commands struct {
	list []*Command
	byID map[string]*Command
}

func (c *Commands) add(cmd *Command) {
	c.list = append(c.list, cmd)
	c.byID[cmd.ID] = cmd
}

func (c *Commands) get(id string) *Command { return c.byID[id] }

// Run runs a command by ID with an argument.
func (c *Commands) Run(id, arg string) error {
	cmd := c.byID[id]
	if cmd == nil {
		return fmt.Errorf("no command %q", id)
	}
	if cmd.Enabled != nil && !cmd.Enabled() {
		return fmt.Errorf("%s is not available now", cmd.Label)
	}
	cmd.Run(arg)
	return nil
}

func (c *Commands) enabled(id string) bool {
	cmd := c.byID[id]
	return cmd != nil && (cmd.Enabled == nil || cmd.Enabled())
}

func (a *App) registerCommands() *Commands {
	c := &Commands{byID: map[string]*Command{}}
	inSketch := func() bool { return a.sk != nil }
	notSketch := func() bool { return a.sk == nil }

	c.add(&Command{ID: "file.new", Label: "New design", Group: "File", Icon: ui.FaFile, Shortcut: "Ctrl+N", Run: func(string) {
		a.doc.freeAll()
		a.doc = NewDocument()
		a.sk, a.sel, a.hover, a.browserSel = nil, nil, Ref{}, 0
		a.cam = NewCamera()
		a.updateTitle()
	}})
	c.add(&Command{ID: "file.open", Label: "Open…", Group: "File", Icon: ui.FaFolder, Shortcut: "Ctrl+O",
		Run: func(arg string) {
			if arg != "" {
				a.openFile(arg)
				return
			}
			a.pickFile(false, "Open design", "Open", startIn(a.doc.Path, ""), designFilter, a.openFile)
		}})
	c.add(&Command{ID: "file.save", Label: "Save", Group: "File", Icon: ui.FaFloppy, Shortcut: "Ctrl+S",
		Run: func(arg string) {
			if arg != "" || a.doc.Path != "" {
				a.saveFile(pickStr(arg, a.doc.Path))
				return
			}
			a.pickFile(true, "Save design", "Save", defaultPath("design.forma"), designFilter, a.saveFile)
		}})
	c.add(&Command{ID: "file.saveas", Label: "Save as…", Group: "File", Shortcut: "Ctrl+Shift+S",
		Run: func(string) {
			a.pickFile(true, "Save design as", "Save", startIn(a.doc.Path, "design.forma"), designFilter, a.saveFile)
		}})
	c.add(&Command{ID: "file.recover", Label: "Recover autosave", Group: "File",
		Enabled: func() bool { return autosaveExists() }, Run: func(string) { a.openFile(autosavePath()) }})
	c.add(&Command{ID: "file.quit", Label: "Quit", Group: "File", Run: func(string) { a.quit = true }})

	c.add(&Command{ID: "edit.undo", Label: "Undo", Group: "Edit", Shortcut: "Ctrl+Z", Icon: 0xf0e2,
		Enabled: func() bool { return a.doc.hist.CanUndo() }, Run: func(string) { a.undo(false) }})
	c.add(&Command{ID: "edit.redo", Label: "Redo", Group: "Edit", Shortcut: "Ctrl+Y", Icon: 0xf01e,
		Enabled: func() bool { return a.doc.hist.CanRedo() }, Run: func(string) { a.undo(true) }})

	for _, k := range PrimKinds {
		k := k
		c.add(&Command{ID: "create." + k, Label: doc.KindLabel(k), Group: "Create", Icon: ui.FaCube, Enabled: notSketch,
			Run: func(arg string) {
				in := map[string]string{}
				for _, kv := range strings.Split(arg, ",") {
					if name, val, ok := strings.Cut(kv, "="); ok {
						in[strings.TrimSpace(name)] = strings.TrimSpace(val)
					}
				}
				f, err := a.doc.AddPrim(k, in)
				if err != nil {
					a.notify("%s: %v", doc.KindLabel(k), err)
				}
				a.browserSel = bodyKey(f.ID, 0)
				a.sel = a.sel[:0]
			}})
	}

	c.add(&Command{ID: "feature.extrude", Label: "Extrude", Group: "Create", Shortcut: "Shift+E", Icon: 0xf062,
		Enabled: func() bool { return len(a.selectedProfiles()) > 0 }, Run: func(arg string) { a.newSweep(doc.KExtrude, arg) }})
	c.add(&Command{ID: "feature.revolve", Label: "Revolve", Group: "Create", Shortcut: "Shift+R", Icon: ui.FaRotate,
		Enabled: func() bool { return len(a.selectedProfiles()) > 0 }, Run: func(arg string) { a.newSweep(doc.KRevolve, arg) }})
	c.add(&Command{ID: "param.add", Label: "Add parameter", Group: "Design", Icon: 0xf1ec, Run: func(arg string) {
		name, ex, _ := strings.Cut(arg, "=")
		if ex == "" {
			ex = "10"
		}
		a.doc.D.AddParam(strings.TrimSpace(name), strings.TrimSpace(ex))
		a.doc.Commit()
		a.showParams = true
	}})
	c.add(&Command{ID: "param.set", Label: "Set parameter", Group: "Design", Run: func(arg string) {
		name, ex, _ := strings.Cut(arg, "=")
		for _, p := range a.doc.D.Params {
			if p.Name == strings.TrimSpace(name) {
				p.Expr = strings.TrimSpace(ex)
				a.doc.Commit()
				return
			}
		}
		a.notify("No parameter %q", name)
	}})
	c.add(&Command{ID: "feature.op", Label: "Set the operation", Group: "Design",
		Enabled: func() bool { return a.selFeature() != nil && a.selFeature().Kind != doc.KSketch },
		Run: func(arg string) {
			f := a.selFeature()
			for _, o := range doc.Operations {
				if strings.EqualFold(o, arg) {
					f.Inputs["Operation"] = o
					if o == "Cut" && f.Kind == doc.KExtrude {
						f.Inputs["Flip"] = "1"
					}
					a.doc.Commit()
					return
				}
			}
			a.notify("No operation %q", arg)
		}})
	c.add(&Command{ID: "timeline.end", Label: "Roll the timeline to the end", Group: "Design",
		Enabled: func() bool { return a.doc.D.Rollback >= 0 }, Run: func(string) { a.rollTo(-1) }})
	c.add(&Command{ID: "view.params", Label: "Parameters", Group: "View", Icon: 0xf1ec, Run: func(string) { a.showParams = !a.showParams }})

	for _, pl := range []struct{ id, label string }{{"xy", "Sketch on XY (top)"}, {"xz", "Sketch on XZ (front)"}, {"yz", "Sketch on YZ (right)"}} {
		pl := pl
		c.add(&Command{ID: "sketch." + pl.id, Label: pl.label, Group: "Sketch", Icon: ui.FaPen, Enabled: notSketch,
			Run: func(string) {
				ref, _ := a.planeFor(pl.id)
				a.enterSketch(a.doc.AddSketch(ref))
			}})
	}
	c.add(&Command{ID: "sketch.face", Label: "Sketch on selected face", Group: "Sketch", Icon: ui.FaPen,
		Enabled: func() bool {
			if a.sk != nil {
				return false
			}
			_, ok := a.planeFor("")
			return ok
		},
		Run: func(string) {
			ref, ok := a.planeFor("")
			if !ok {
				a.notify("Select a flat face first")
				return
			}
			a.enterSketch(a.doc.AddSketch(ref))
		}})
	c.add(&Command{ID: "sketch.edit", Label: "Edit sketch", Group: "Sketch", Enabled: notSketch, Run: func(arg string) {
		id, _ := strconv.Atoi(arg)
		if id == 0 {
			id = a.browserSel
		}
		if s := a.doc.Sketch(id); s != nil {
			a.enterSketch(s)
		}
	}})
	c.add(&Command{ID: "sketch.finish", Label: "Finish sketch", Group: "Sketch", Icon: ui.FaCheck, Shortcut: "Ctrl+Enter",
		Enabled: inSketch, Run: func(string) { a.finishSketch() }})

	tools := []struct {
		id   string
		t    sketchTool
		key  string
		icon rune
	}{
		{"select", toolSelect, "S", 0xf245}, {"line", toolLine, "L", ui.FaSlash}, {"rect", toolRect, "R", ui.FaSquare},
		{"circle", toolCircle, "C", ui.FaCircle}, {"arc", toolArc, "A", ui.FaBezier}, {"point", toolPoint, "P", ui.FaDotCircle}, {"spline", toolSpline, "B", ui.FaBezier}, {"text", toolText, "", 0xf031}, {"project", toolProject, "", 0xf247},
		{"dim", toolDim, "D", ui.FaRuler},
	}
	for _, t := range tools {
		t := t
		c.add(&Command{ID: "tool." + t.id, Label: toolNames[t.t], Group: "Sketch tool", Shortcut: t.key, Icon: t.icon, Enabled: inSketch,
			Run: func(string) { a.sk.setTool(t.t) }})
	}
	c.add(&Command{ID: "sketch.gridsnap", Label: "Snap to grid", Group: "View", Shortcut: "G",
		Run: func(string) { a.snap.off = !a.snap.off }})
	c.add(&Command{ID: "sketch.construction", Label: "Toggle construction", Group: "Sketch", Shortcut: "X", Enabled: inSketch,
		Run: func(string) {
			s := a.sk.item.S
			for _, id := range a.selectedSketchEnts(a.sk.item.ID) {
				if e := s.Ent(id); e != nil && e.Kind != sketch.Point {
					e.Construction = !e.Construction
				}
			}
			a.sk.item.markDirty()
			a.doc.Commit()
		}})

	cons := []struct {
		id  string
		k   sketch.ConKind
		key string
	}{
		{"coincident", sketch.Coincident, ""}, {"horizontal", sketch.Horizontal, "H"}, {"vertical", sketch.Vertical, "V"},
		{"parallel", sketch.Parallel, ""}, {"perpendicular", sketch.Perpendicular, ""}, {"tangent", sketch.Tangent, "T"},
		{"equal", sketch.Equal, "E"}, {"concentric", sketch.Concentric, ""}, {"midpoint", sketch.Midpoint, ""},
		{"fix", sketch.Fix, ""},
	}
	for _, k := range cons {
		k := k
		c.add(&Command{ID: "constrain." + k.id, Label: k.k.String(), Group: "Constrain", Shortcut: k.key, Enabled: inSketch,
			Run: func(string) { a.applyConstraint(k.k) }})
	}

	views := []struct {
		id  string
		v   StdView
		key string
	}{{"iso", ViewIso, "Num 0"}, {"front", ViewFront, "Num 1"}, {"back", ViewBack, "Ctrl+Num 1"}, {"right", ViewRight, "Num 3"},
		{"left", ViewLeft, "Ctrl+Num 3"}, {"top", ViewTop, "Num 7"}, {"bottom", ViewBottom, "Ctrl+Num 7"}}
	for _, v := range views {
		v := v
		c.add(&Command{ID: "view." + v.id, Label: strings.Title(v.id) + " view", Group: "View", Shortcut: v.key, Icon: ui.FaCamera,
			Run: func(arg string) {
				if arg == "now" || !a.animate() {
					a.cam.SetView(v.v)
				} else {
					a.cam.AnimateView(v.v)
				}
			}})
	}
	c.add(&Command{ID: "view.fit", Label: "Zoom to fit", Group: "View", Shortcut: "F", Icon: ui.FaExpand, Run: func(string) {
		a.cam.Fit(a.fitBounds(), a.aspect(), a.animate())
	}})
	c.add(&Command{ID: "view.layout", Label: "Reset panel layout", Group: "View",
		Run: func(string) { a.layoutSet, a.layoutReset = false, true }})
	c.add(&Command{ID: "view.ortho", Label: "Toggle orthographic", Group: "View", Shortcut: "Num 5", Run: func(string) {
		a.cam.Ortho = !a.cam.Ortho
	}})
	c.add(&Command{ID: "view.grid", Label: "Toggle grid", Group: "View", Run: func(string) { a.showGrid = !a.showGrid }})
	c.add(&Command{ID: "edit.delete", Label: "Delete", Group: "Edit", Shortcut: "Del", Icon: ui.FaTrash, Run: func(string) { a.deleteSelection() }})
	c.add(&Command{ID: "edit.clear", Label: "Clear selection", Group: "Edit", Shortcut: "Esc", Run: func(string) { a.escape() }})
	c.add(&Command{ID: "help.shortcuts", Label: "Keyboard shortcuts", Group: "Help", Shortcut: "F1", Icon: ui.FaKeyboard,
		Run: func(string) { a.showHelp = !a.showHelp }})
	c.add(&Command{ID: "palette", Label: "Command palette", Group: "Help", Shortcut: "Ctrl+K", Icon: ui.FaSearch,
		Run: func(string) { a.palette.open = true; a.palette.filter = ""; a.palette.cursor = 0 }})
	a.registerModifyCommands(c)
	a.registerExchangeCommands(c)
	a.registerAssemblyCommands(c)
	a.registerInspectCommands(c)
	a.registerSnapCommands(c)
	a.registerTextCommands(c)
	a.registerOffsetCommands(c)
	a.registerM13Commands(c)
	a.registerSimCommands(c)
	return c
}

// escape cancels the tool in progress, then the tool, then the selection.
func (a *App) escape() {
	if a.sk != nil {
		sm := a.sk
		switch {
		case sm.editOpen:
			sm.editOpen = false
		case len(sm.clicks) > 0 || !sm.dimFirst.IsZero():
			sm.cancel()
		case len(a.sel) > 0:
			a.sel = a.sel[:0]
		case sm.tool != toolSelect:
			sm.setTool(toolSelect)
		}
		return
	}
	a.sel = a.sel[:0]
	a.browserSel = 0
}

// shortcuts handles the keyboard when no text field has it.
func (a *App) shortcuts() {
	io := imgui.CurrentIO()
	if io.WantTextInput() || a.palette.open {
		return
	}
	ctrl := io.KeyCtrl()
	press := func(k imgui.Key) bool { return imgui.IsKeyPressedBoolV(k, false) }
	run := func(id string) { _ = a.cmds.Run(id, "") }
	shift := io.KeyShift()
	switch {
	case ctrl && press(imgui.KeyZ) && !shift:
		run("edit.undo")
	case ctrl && (press(imgui.KeyY) || (shift && press(imgui.KeyZ))):
		run("edit.redo")
	case ctrl && shift && press(imgui.KeyS):
		run("file.saveas")
	case ctrl && press(imgui.KeyS):
		run("file.save")
	case ctrl && press(imgui.KeyO):
		run("file.open")
	case ctrl && press(imgui.KeyN):
		run("file.new")
	case shift && !ctrl && press(imgui.KeyE):
		run("feature.extrude")
		return
	case shift && !ctrl && press(imgui.KeyR):
		run("feature.revolve")
		return
	case ctrl && press(imgui.KeyK):
		run("palette")
	case ctrl && press(imgui.KeyE):
		run("file.export")
	case ctrl && press(imgui.KeyI):
		run("file.import")
	case ctrl && (press(imgui.KeyEnter) || press(imgui.KeyKeypadEnter)):
		run("sketch.finish")
	case press(imgui.KeyEscape):
		run("edit.clear")
	case press(imgui.KeyDelete) || press(imgui.KeyBackspace):
		run("edit.delete")
	case press(imgui.KeyF1):
		run("help.shortcuts")
	case press(imgui.KeyKeypad0):
		run("view.iso")
	case press(imgui.KeyKeypad1):
		run(pick(ctrl, "view.back", "view.front"))
	case press(imgui.KeyKeypad3):
		run(pick(ctrl, "view.left", "view.right"))
	case press(imgui.KeyKeypad7):
		run(pick(ctrl, "view.bottom", "view.top"))
	case press(imgui.KeyKeypad5):
		run("view.ortho")
	case !ctrl && !shift && a.sk == nil && press(imgui.KeyG):
		run("sketch.gridsnap")
	case !ctrl && press(imgui.KeyF):
		run("view.fit")
	}
	if a.sk == nil || ctrl || shift {
		return
	}
	for key, id := range map[imgui.Key]string{
		imgui.KeyS: "tool.select", imgui.KeyL: "tool.line", imgui.KeyR: "tool.rect", imgui.KeyC: "tool.circle",
		imgui.KeyA: "tool.arc", imgui.KeyP: "tool.point", imgui.KeyB: "tool.spline", imgui.KeyD: "tool.dim", imgui.KeyH: "constrain.horizontal",
		imgui.KeyV: "constrain.vertical", imgui.KeyT: "constrain.tangent", imgui.KeyE: "constrain.equal",
		imgui.KeyG: "sketch.gridsnap", imgui.KeyX: "sketch.construction",
	} {
		if press(key) {
			run(id)
		}
	}
	if imgui.IsMouseClickedBool(imgui.MouseButtonRight) && a.vp.hovered && len(a.sk.clicks) > 0 {
		a.sk.cancel()
	}
}

func pick(c bool, a, b string) string {
	if c {
		return a
	}
	return b
}

// paletteState is the Ctrl+K command palette.
type paletteState struct {
	open   bool
	filter string
	cursor int
}

func fuzzy(label, f string) bool {
	label, f = strings.ToLower(label), strings.ToLower(f)
	i := 0
	for _, r := range label {
		if i < len(f) && rune(f[i]) == r {
			i++
		}
	}
	return i == len(f)
}

func (p *paletteState) draw(a *App) {
	if !p.open {
		return
	}
	vpt := imgui.MainViewport()
	center := imgui.Vec2{X: vpt.Pos().X + vpt.Size().X/2, Y: vpt.Pos().Y + vpt.Size().Y*0.22}
	imgui.SetNextWindowPosV(center, imgui.CondAlways, imgui.Vec2{X: 0.5, Y: 0})
	imgui.SetNextWindowSizeV(imgui.Vec2{X: 520, Y: 0}, imgui.CondAlways)
	flags := imgui.WindowFlagsNoTitleBar | imgui.WindowFlagsNoResize | imgui.WindowFlagsNoMove | imgui.WindowFlagsNoSavedSettings
	if imgui.BeginV("##palette", &p.open, flags) {
		if imgui.IsWindowAppearing() {
			imgui.SetKeyboardFocusHere()
		}
		imgui.SetNextItemWidth(-1)
		imgui.InputTextWithHint("##f", ui.Icon(ui.FaSearch, "Type a command…"), &p.filter, 0, nil)
		var hits []*Command
		for _, c := range a.cmds.list {
			if c.ID == "palette" {
				continue
			}
			if fuzzy(c.Group+" "+c.Label, p.filter) && a.cmds.enabled(c.ID) {
				hits = append(hits, c)
			}
		}
		sort.SliceStable(hits, func(i, j int) bool {
			pi := strings.HasPrefix(strings.ToLower(hits[i].Label), strings.ToLower(p.filter))
			pj := strings.HasPrefix(strings.ToLower(hits[j].Label), strings.ToLower(p.filter))
			return pi && !pj
		})
		if imgui.IsKeyPressedBool(imgui.KeyDownArrow) {
			p.cursor++
		}
		if imgui.IsKeyPressedBool(imgui.KeyUpArrow) {
			p.cursor--
		}
		if p.cursor >= len(hits) {
			p.cursor = len(hits) - 1
		}
		if p.cursor < 0 {
			p.cursor = 0
		}
		run := -1
		if imgui.IsKeyPressedBool(imgui.KeyEnter) && len(hits) > 0 {
			run = p.cursor
		}
		if imgui.IsKeyPressedBool(imgui.KeyEscape) {
			p.open = false
		}
		for i, c := range hits {
			if i >= 14 {
				break
			}
			label := fmt.Sprintf("%s  %s", c.Group, ui.Icon(' ', ""))
			_ = label
			if imgui.SelectableBoolV(fmt.Sprintf("%s › %s##%s", c.Group, c.Label, c.ID), i == p.cursor, 0, imgui.Vec2{}) {
				run = i
			}
			if c.Shortcut != "" {
				imgui.SameLine()
				w := imgui.CalcTextSize(c.Shortcut).X
				imgui.SetCursorPosX(imgui.WindowWidth() - w - 16)
				imgui.TextDisabled(c.Shortcut)
			}
		}
		if run >= 0 && run < len(hits) {
			p.open = false
			hits[run].Run("")
		}
	}
	imgui.End()
}

// toast is a short message that fades.
type toast struct {
	msg string
	t   float64
}

func (t *toast) show(m string) { t.msg, t.t = m, 3.5 }
func (t *toast) tick(dt float64) {
	if t.t > 0 {
		t.t -= dt
	}
}

func (t *toast) draw() {
	if t.t <= 0 {
		return
	}
	vpt := imgui.MainViewport()
	pos := imgui.Vec2{X: vpt.Pos().X + vpt.Size().X/2, Y: vpt.Pos().Y + vpt.Size().Y - 70}
	imgui.SetNextWindowPosV(pos, imgui.CondAlways, imgui.Vec2{X: 0.5, Y: 1})
	alpha := float32(t.t)
	if alpha > 1 {
		alpha = 1
	}
	imgui.SetNextWindowBgAlpha(0.92 * alpha)
	flags := imgui.WindowFlagsNoDecoration | imgui.WindowFlagsAlwaysAutoResize | imgui.WindowFlagsNoSavedSettings |
		imgui.WindowFlagsNoFocusOnAppearing | imgui.WindowFlagsNoNav | imgui.WindowFlagsNoInputs
	if imgui.BeginV("##toast", nil, flags) {
		imgui.TextUnformatted(t.msg)
	}
	imgui.End()
}
