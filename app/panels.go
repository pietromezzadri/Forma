package app

import (
	"fmt"

	imgui "github.com/AllenDang/cimgui-go/imgui"

	"forma/sketch"
	"forma/ui"
)

func (a *App) menuItem(id string) {
	c := a.cmds.get(id)
	if c == nil {
		return
	}
	if imgui.MenuItemBoolV(ui.Icon(iconOr(c.Icon), c.Label), c.Shortcut, false, a.cmds.enabled(id)) {
		c.Run("")
	}
}

func iconOr(r rune) rune {
	if r == 0 {
		return ' '
	}
	return r
}

func (a *App) mainMenu() {
	if !imgui.BeginMainMenuBar() {
		return
	}
	if imgui.BeginMenu("File") {
		a.menuItem("file.new")
		a.menuItem("file.open")
		a.menuItem("file.save")
		a.menuItem("file.saveas")
		a.menuItem("file.recover")
		imgui.Separator()
		a.menuItem("file.import")
		a.menuItem("file.export")
		imgui.Separator()
		a.menuItem("file.quit")
		imgui.EndMenu()
	}
	if imgui.BeginMenu("Edit") {
		a.menuItem("edit.undo")
		a.menuItem("edit.redo")
		imgui.Separator()
		a.menuItem("edit.delete")
		imgui.Separator()
		a.menuItem("param.add")
		a.menuItem("timeline.end")
		imgui.EndMenu()
	}
	if imgui.BeginMenu("Create") {
		for _, k := range PrimKinds {
			a.menuItem("create." + k)
		}
		imgui.Separator()
		a.menuItem("feature.extrude")
		a.menuItem("feature.revolve")
		imgui.Separator()
		a.menuItem("sketch.xy")
		a.menuItem("sketch.xz")
		a.menuItem("sketch.yz")
		a.menuItem("sketch.face")
		imgui.EndMenu()
	}
	if a.sk != nil && imgui.BeginMenu("Sketch") {
		for _, t := range []string{"select", "line", "rect", "circle", "arc", "point", "dim"} {
			a.menuItem("tool." + t)
		}
		imgui.Separator()
		for _, c := range []string{"coincident", "horizontal", "vertical", "parallel", "perpendicular", "tangent", "equal", "concentric", "midpoint", "fix"} {
			a.menuItem("constrain." + c)
		}
		imgui.Separator()
		a.menuItem("sketch.construction")
		a.menuItem("sketch.gridsnap")
		a.menuItem("sketch.finish")
		imgui.EndMenu()
	}
	if imgui.BeginMenu("View") {
		for _, v := range []string{"iso", "front", "back", "right", "left", "top", "bottom"} {
			a.menuItem("view." + v)
		}
		imgui.Separator()
		a.menuItem("view.fit")
		a.menuItem("view.ortho")
		imgui.Separator()
		a.menuItem("view.layout")
		a.menuItem("view.grid")
		a.menuItem("view.params")
		imgui.EndMenu()
	}
	if imgui.BeginMenu("Help") {
		a.menuItem("palette")
		a.menuItem("help.shortcuts")
		imgui.EndMenu()
	}
	imgui.EndMainMenuBar()
}

func (a *App) statusBar() {
	h := imgui.FrameHeight() + 4
	flags := imgui.WindowFlagsNoScrollbar | imgui.WindowFlagsNoSavedSettings | imgui.WindowFlagsNoTitleBar
	imgui.PushStyleVarVec2(imgui.StyleVarWindowPadding, imgui.Vec2{X: 10, Y: 3})
	if imgui.InternalBeginViewportSideBar("##status", imgui.MainViewport(), imgui.DirDown, h, flags) {
		hov := a.describe(a.hover)
		if a.sk != nil && a.sk.hoverDim != 0 {
			hov = a.describe(Ref{Kind: KindDim, Item: a.sk.item.ID, Local: a.sk.hoverDim})
		}
		if hov == "" {
			hov = "—"
		}
		imgui.TextUnformatted(hov)
		imgui.SameLine()
		right := fmt.Sprintf("%d selected   ·   mm   ·   %.0f fps", len(a.sel), imgui.CurrentIO().Framerate())
		if a.vp.hasCursor {
			p := a.vp.cursor3
			right = fmt.Sprintf("(%.2f, %.2f, %.2f)   ·   %s", p.X, p.Y, p.Z, right)
		}
		w := imgui.CalcTextSize(right).X
		imgui.SetCursorPosX(imgui.WindowWidth() - w - 12)
		imgui.TextDisabled(right)
	}
	imgui.End()
	imgui.PopStyleVar()
}

// defaultLayout docks the panels round the viewport (first run, or captures).
func (a *App) defaultLayout(id imgui.ID) {
	if n := imgui.InternalDockBuilderGetNode(id); n != nil && !a.layoutReset && (a.capture == nil || a.capture.script.Ini != "") {
		// (ChildNodes wraps the child slots, never nil: ask ImGui)
		if n.InternalIsSplitNode() {
			return // a saved layout
		}
	}
	imgui.InternalDockBuilderRemoveNode(id)
	imgui.InternalDockBuilderAddNodeV(id, imgui.DockNodeFlags(imgui.DockNodeFlagsDockSpace)|imgui.DockNodeFlagsPassthruCentralNode)
	vpt := imgui.MainViewport()
	imgui.InternalDockBuilderSetNodeSize(id, vpt.WorkSize())
	var left, rest, right, center, bottom, mid imgui.ID
	imgui.InternalDockBuilderSplitNode(id, imgui.DirLeft, 0.17, &left, &rest)
	imgui.InternalDockBuilderSplitNode(rest, imgui.DirRight, 0.24, &right, &center)
	imgui.InternalDockBuilderSplitNode(center, imgui.DirDown, 0.10, &bottom, &mid)
	imgui.InternalDockBuilderDockWindow("Browser", left)
	imgui.InternalDockBuilderDockWindow("Properties", right)
	imgui.InternalDockBuilderDockWindow("Timeline", bottom)
	imgui.InternalDockBuilderDockWindow("Viewport", mid)
	imgui.InternalDockBuilderFinish(id)
	a.layoutReset = false
}

func (a *App) sketchProperties(s *SketchItem) {
	imgui.SeparatorText(ui.Icon(ui.FaPen, s.Name))
	sk := s.S
	st := sk.Status
	pts, curves := 0, 0
	for _, e := range sk.Ents {
		if e.Kind == sketch.Point {
			pts++
		} else {
			curves++
		}
	}
	imgui.Text(fmt.Sprintf("%d curves · %d points · %d constraints", curves, pts, len(userCons(sk))))
	if st.DOF == 0 && len(sk.Ents) > 0 {
		imgui.TextColored(imgui.Vec4{X: 0.45, Y: 0.85, Z: 0.5, W: 1}, "Fully constrained")
	} else {
		imgui.TextColored(imgui.Vec4{X: 0.5, Y: 0.7, Z: 1, W: 1}, fmt.Sprintf("%d degrees of freedom", st.DOF))
	}
	if a.sk == nil {
		if imgui.Button(ui.Icon(ui.FaPen, "Edit sketch")) {
			a.enterSketch(s)
		}
	}
	imgui.SeparatorText(fmt.Sprintf("Profiles (%d)", len(s.Regions)))
	for i, r := range s.Regions {
		holes := ""
		if len(r.Holes) > 0 {
			holes = fmt.Sprintf(", %d holes", len(r.Holes))
		}
		imgui.BulletText(fmt.Sprintf("Profile %d: %s mm²%s", i+1, fmtLen(r.Area), holes))
	}
	if len(s.Regions) == 0 {
		imgui.TextDisabled("Close a loop to make a profile.")
	}
	imgui.SeparatorText("Constraints")
	for _, c := range userCons(sk) {
		imgui.PushIDStr(fmt.Sprint(c.ID))
		label := c.Kind.String()
		if c.Kind.Dimensional() {
			label += " " + dimText(c)
		}
		ref := Ref{Kind: KindDim, Item: s.ID, Local: c.ID}
		if imgui.SelectableBoolV(label, a.isSelected(ref), 0, imgui.Vec2{X: imgui.ContentRegionAvail().X - 30}) {
			a.toggleSelect(ref, imgui.CurrentIO().KeyCtrl())
		}
		if c.Kind.Dimensional() && a.sk != nil && imgui.IsItemHovered() && imgui.IsMouseDoubleClicked(imgui.MouseButtonLeft) {
			a.sk.openEdit(c.ID)
		}
		imgui.SameLine()
		if imgui.SmallButton("x") {
			sk.DeleteConstraint(c.ID)
			sk.Solve()
			s.markDirty()
			a.doc.Commit()
		}
		imgui.PopID()
	}
}

func userCons(s *sketch.Sketch) []*sketch.Constraint {
	var out []*sketch.Constraint
	for _, c := range s.Cons {
		if !c.Internal {
			out = append(out, c)
		}
	}
	return out
}

func (a *App) helpWindow() {
	if !a.showHelp {
		return
	}
	imgui.SetNextWindowSizeV(imgui.Vec2{X: 460, Y: 520}, imgui.CondFirstUseEver)
	if imgui.BeginV("Keyboard shortcuts", &a.showHelp, 0) {
		rows := [][2]string{
			{"Right drag / Shift + middle drag", "Orbit round the point under the cursor"},
			{"Middle drag", "Pan"},
			{"Wheel", "Zoom toward the cursor"},
			{"Click / Ctrl+click", "Select / add to the selection"},
			{"F", "Zoom to fit"},
			{"Num 0 / 1 / 3 / 7", "Iso / front / right / top (Ctrl: opposite)"},
			{"Num 5", "Orthographic on / off"},
			{"Ctrl+K", "Command palette"},
			{"Ctrl+E", "Export (STEP, STL, OBJ)"},
			{"Ctrl+I", "Import (STEP, STL)"},
			{"Esc", "Cancel / clear selection"},
			{"Del", "Delete the selection"},
			{"", ""},
			{"In a sketch", ""},
			{"S L R C A P D", "Select, line, rectangle, circle, arc, point, dimension"},
			{"H V T E", "Horizontal, vertical, tangent, equal"},
			{"X", "Construction on / off"},
			{"G", "Grid snap on / off"},
			{"Double-click a dimension", "Type its value (mm, cm, in, deg, 20+5)"},
			{"Ctrl+Enter", "Finish the sketch"},
		}
		if imgui.BeginTableV("keys", 2, imgui.TableFlagsRowBg|imgui.TableFlagsSizingStretchProp, imgui.Vec2{}, 0) {
			for _, r := range rows {
				imgui.TableNextRow()
				imgui.TableNextColumn()
				imgui.TextUnformatted(r[0])
				imgui.TableNextColumn()
				imgui.TextDisabled(r[1])
			}
			imgui.EndTable()
		}
	}
	imgui.End()
}
