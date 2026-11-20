package app

import (
	"fmt"

	imgui "github.com/AllenDang/cimgui-go/imgui"

	"forma/brep"
	"forma/doc"
	"forma/ui"
)

var (
	colErr    = imgui.Vec4{X: 1, Y: 0.42, Z: 0.35, W: 1}
	colWarn   = imgui.Vec4{X: 1, Y: 0.78, Z: 0.35, W: 1}
	colOK     = imgui.Vec4{X: 0.5, Y: 0.85, Z: 0.5, W: 1}
	colAccent = imgui.Vec4{X: 0.88, Y: 0.62, Z: 0.28, W: 0.55}
)

func kindIcon(k string) rune {
	switch k {
	case doc.KSketch:
		return ui.FaPen
	case doc.KExtrude:
		return 0xf062
	case doc.KRevolve:
		return ui.FaRotate
	case doc.KMove:
		return icMove
	case doc.KMirror:
		return icMirror
	case doc.KPattern:
		return icPattern
	case doc.KFillet:
		return icFillet
	case doc.KChamfer:
		return icChamfer
	case doc.KHole:
		return icHole
	case doc.KShell:
		return icShell
	case doc.KImport:
		return icImport
	case doc.KMesh:
		return icMesh
	case doc.KComponent:
		return icComponent
	case doc.KJoint:
		return icJoint
	}
	return ui.FaCube
}

func (a *App) browserPanel() {
	if !imgui.Begin("Browser") {
		imgui.End()
		return
	}
	eye := func(f *doc.Feature) {
		label := ui.Icon(rune(ui.FaEye), "")
		if f.Hidden {
			label = ui.Icon(rune(ui.FaEyeSlash), "")
		}
		if !ui.Icons {
			label = map[bool]string{false: "o", true: "-"}[f.Hidden]
		}
		if imgui.SmallButton(fmt.Sprintf("%s##eye%d", label, f.ID)) {
			f.Hidden = !f.Hidden
			a.doc.Commit()
		}
		imgui.SameLine()
	}
	if imgui.TreeNodeExStrV(fmt.Sprintf("%s (%d)###params", ui.Icon(0xf1ec, "Parameters"), len(a.doc.D.Params)), imgui.TreeNodeFlagsSpanAvailWidth) {
		for _, p := range a.doc.D.Params {
			col := imgui.Vec4{X: 0.8, Y: 0.8, Z: 0.8, W: 1}
			if p.Err != "" {
				col = colErr
			}
			val := fmtLen(p.Value)
			if p.IsText {
				val = "\"" + p.Text + "\""
			}
			imgui.TextColored(col, fmt.Sprintf("%s = %s", p.Name, val))
		}
		if imgui.SmallButton("Edit parameters…") {
			a.showParams = true
		}
		imgui.TreePop()
	}
	if imgui.TreeNodeExStrV(ui.Icon(ui.FaLayer, "Bodies")+"###bodies", imgui.TreeNodeFlagsDefaultOpen|imgui.TreeNodeFlagsSpanAvailWidth) {
		shown := map[int]bool{}
		for _, b := range a.doc.Bodies {
			if shown[b.F.ID] && len(b.F.Bodies) > 1 {
				// one eye per feature
				imgui.Dummy(imgui.Vec2{X: imgui.FrameHeight(), Y: 1})
				imgui.SameLine()
			} else {
				eye(b.F)
			}
			shown[b.F.ID] = true
			if imgui.SelectableBoolV(fmt.Sprintf("%s##b%d", ui.Icon(kindIcon(b.F.Kind), b.Name), b.ID), a.browserSel == b.ID, 0, imgui.Vec2{}) {
				a.browserSel = b.ID
			}
			a.markItem("item:" + b.Name)
			a.featureMenu(b.F)
		}
		if len(a.doc.Bodies) == 0 {
			imgui.TextDisabled("none yet: Create → Box…")
		}
		imgui.TreePop()
	}
	a.meshBrowser(eye)
	if imgui.TreeNodeExStrV(ui.Icon(ui.FaPen, "Sketches")+"###sketches", imgui.TreeNodeFlagsDefaultOpen|imgui.TreeNodeFlagsSpanAvailWidth) {
		for _, s := range a.doc.Sketches {
			eye(s.F)
			name := s.Name
			if a.sk != nil && a.sk.item == s {
				name += "  (editing)"
			}
			if imgui.SelectableBoolV(fmt.Sprintf("%s##s%d", name, s.ID), a.browserSel == s.ID, 0, imgui.Vec2{}) {
				a.browserSel = s.ID
			}
			a.markItem("item:" + s.Name)
			if imgui.IsItemHovered() && imgui.IsMouseDoubleClicked(imgui.MouseButtonLeft) && a.sk == nil {
				a.enterSketch(s)
			}
			a.featureMenu(s.F)
		}
		if len(a.doc.Sketches) == 0 {
			imgui.TextDisabled("none yet: Sketch on XY…")
		}
		imgui.TreePop()
	}
	imgui.End()
}

// featureMenu is a feature's right-click menu (browser and timeline).
func (a *App) featureMenu(f *doc.Feature) {
	if !imgui.BeginPopupContextItem() {
		return
	}
	i := a.doc.D.Index(f.ID)
	if f.Kind == doc.KSketch && a.sk == nil {
		if imgui.MenuItemBool("Edit sketch") {
			if it := a.doc.Sketch(bodyKey(f.ID, 0)); it != nil {
				a.enterSketch(it)
			}
		}
	}
	if imgui.MenuItemBool("Roll the timeline back to here") {
		a.rollTo(i + 1)
	}
	if imgui.MenuItemBool("Roll back to before it") {
		a.rollTo(i)
	}
	label := "Suppress"
	if f.Off {
		label = "Unsuppress"
	}
	if imgui.MenuItemBool(label) {
		f.Off = !f.Off
		a.doc.Commit()
	}
	if imgui.MenuItemBool(map[bool]string{false: "Hide", true: "Show"}[f.Hidden]) {
		f.Hidden = !f.Hidden
		a.doc.Commit()
	}
	imgui.Separator()
	deps := a.doc.D.Dependents(f.ID)
	del := "Delete"
	if len(deps) > 0 {
		del = fmt.Sprintf("Delete (%d features use it)", len(deps))
	}
	if imgui.MenuItemBool(del) {
		if a.sk != nil && a.sk.item.F == f {
			a.sk = nil
		}
		a.doc.Remove(bodyKey(f.ID, 0))
		a.sel, a.hover = a.sel[:0], Ref{}
		if featureOf(a.browserSel) == f.ID {
			a.browserSel = 0
		}
	}
	imgui.EndPopup()
}

func (a *App) propertiesPanel() {
	if !imgui.Begin("Properties") {
		imgui.End()
		return
	}
	if a.sk != nil {
		a.sketchProperties(a.sk.item)
	} else if f := a.selFeature(); f != nil {
		a.featureProperties(f)
	} else {
		imgui.TextDisabled("Select a body, sketch or timeline step.")
		imgui.Spacing()
		imgui.TextWrapped("To extrude: make a sketch with a closed loop, click inside the loop (a profile) and press Extrude.")
	}
	if len(a.sel) > 0 {
		imgui.SeparatorText("Selection")
		for _, r := range a.sel {
			imgui.BulletText(a.describe(r))
		}
		if a.sk == nil {
			if ms := a.measureLines(); len(ms) > 0 {
				imgui.SeparatorText(ui.Icon(icMeasure, "Measure"))
				for _, m := range ms {
					imgui.TextUnformatted(m)
				}
			}
		}
	}
	imgui.End()
}

// exprField edits an expression input of a feature, committing when done.
func (a *App) exprField(f *doc.Feature, name, unit string) {
	v := f.Inputs[name]
	imgui.SetNextItemWidth(130)
	imgui.InputTextWithHint(name+"##"+fmt.Sprint(f.ID), "expression", &v, imgui.InputTextFlagsAutoSelectAll, nil)
	a.markItemLeft("prop:" + name)
	if v != f.Inputs[name] {
		f.Inputs[name] = v
	}
	if imgui.IsItemDeactivatedAfterEdit() {
		a.doc.Commit()
	}
	if x, err := a.doc.D.Eval(f.Inputs[name]); err == nil {
		if fmtLen(x) != f.Inputs[name] {
			imgui.SameLine()
			imgui.TextDisabled("= " + fmtLen(x) + unit)
		}
	} else {
		imgui.SameLine()
		imgui.TextColored(colErr, "?")
		imgui.SetItemTooltip(err.Error())
	}
}

func (a *App) combo(label string, cur string, options []string, disabled map[string]string) (string, bool) {
	changed := false
	imgui.SetNextItemWidth(130)
	if imgui.BeginCombo(label, cur) {
		for _, o := range options {
			why, off := disabled[o]
			imgui.BeginDisabledV(off)
			txt := o
			if off {
				txt += "  (" + why + ")"
			}
			if imgui.SelectableBoolV(txt, o == cur, 0, imgui.Vec2{}) && o != cur {
				cur, changed = o, true
			}
			imgui.EndDisabled()
		}
		imgui.EndCombo()
	}
	return cur, changed
}

func (a *App) featureProperties(f *doc.Feature) {
	imgui.SeparatorText(ui.Icon(kindIcon(f.Kind), doc.KindLabel(f.Kind)))
	name := f.Name
	imgui.SetNextItemWidth(-1)
	imgui.InputTextWithHint("##name", "name", &name, 0, nil)
	if name != f.Name && name != "" {
		f.Name = name
	}
	if imgui.IsItemDeactivatedAfterEdit() {
		a.doc.Commit()
	}
	if f.Err != "" {
		imgui.PushStyleColorVec4(imgui.ColText, colErr)
		imgui.TextWrapped(f.Err)
		imgui.PopStyleColor()
	}
	if !f.Active {
		imgui.TextDisabled(map[bool]string{true: "Suppressed", false: "Rolled back (after the timeline marker)"}[f.Off])
	}
	switch f.Kind {
	case doc.KSketch:
		if it := a.doc.Sketch(bodyKey(f.ID, 0)); it != nil {
			a.sketchProperties(it)
		}
		imgui.TextDisabled(planeText(f.Plane, a.doc.D))
		return
	case doc.KExtrude:
		a.profilesRow(f)
		mode := f.Inputs["Direction"]
		if mode == "" {
			mode = doc.ExtrudeModes[0]
		}
		if m, ok := a.combo("Direction", mode, doc.ExtrudeModes, nil); ok {
			f.Inputs["Direction"] = m
			a.doc.Commit()
		}
		a.exprField(f, "Distance", " mm")
		if f.Inputs["Direction"] == "Two sides" {
			a.exprField(f, "Distance 2", " mm")
		}
		flip := f.Inputs["Flip"] == "1"
		if imgui.Checkbox("Flip direction", &flip) {
			f.Inputs["Flip"] = map[bool]string{true: "1", false: "0"}[flip]
			a.doc.Commit()
		}
		a.opCombo(f)
	case doc.KRevolve:
		a.profilesRow(f)
		axes := []string{"Sketch Y axis", "Sketch X axis", "A sketch line"}
		cur := axes[0]
		if f.Axis != nil {
			switch f.Axis.Kind {
			case "x":
				cur = axes[1]
			case "line":
				cur = axes[2]
			}
		}
		if m, ok := a.combo("Axis", cur, axes, nil); ok {
			switch m {
			case axes[0]:
				f.Axis = &doc.AxisRef{Kind: "y"}
			case axes[1]:
				f.Axis = &doc.AxisRef{Kind: "x"}
			default:
				a.notify("Select a line of the sketch, then press Use selected line")
			}
			a.doc.Commit()
		}
		if imgui.SmallButton("Use selected line") {
			a.useSelectedLine(f)
		}
		a.exprField(f, "Angle", "°")
		a.opCombo(f)
	default:
		if a.assemblyProperties(f) {
			return
		}
		if a.importProperties(f) {
			return
		}
		if a.modifyProperties(f) {
			return
		}
		for _, kv := range doc.PrimInputs[f.Kind] {
			a.exprField(f, kv[0], " mm")
		}
		a.opCombo(f)
	}
	for k := range f.Bodies {
		if it := a.doc.Body(bodyKey(f.ID, k)); it != nil {
			a.bodyInfo(it)
		}
	}
}

func (a *App) profilesRow(f *doc.Feature) {
	imgui.Text(fmt.Sprintf("%d profile(s)", len(f.Profiles)))
	if sel := a.selectedProfiles(); len(sel) > 0 {
		imgui.SameLine()
		if imgui.SmallButton(fmt.Sprintf("Use the %d selected", len(sel))) {
			f.Profiles = sel
			a.sel = a.sel[:0]
			a.doc.Commit()
		}
	}
}

func (a *App) useSelectedLine(f *doc.Feature) {
	for _, r := range a.sel {
		if r.Kind != KindSkCurve {
			continue
		}
		if s := a.doc.Sketch(r.Item); s != nil && len(f.Profiles) > 0 && s.F.ID == f.Profiles[0].Sketch {
			if e := s.S.Ent(r.Local); e != nil && e.Kind == 1 {
				f.Axis = &doc.AxisRef{Kind: "line", Line: e.ID}
				a.doc.Commit()
				return
			}
		}
	}
	a.notify("Select a line in the profile's sketch")
}

func planeText(p *doc.PlaneRef, d *doc.Document) string {
	if p == nil {
		return ""
	}
	if p.Std != "" {
		return "On the " + map[string]string{"xy": "XY (top)", "xz": "XZ (front)", "yz": "YZ (right)"}[p.Std] + " plane"
	}
	if f := d.Feature(p.Feature); f != nil {
		return fmt.Sprintf("On face %q of %s", p.Face, f.Name)
	}
	return "On a face that is gone"
}

func (a *App) bodyInfo(b *BodyItem) {
	imgui.SeparatorText(b.Name)
	v, e, f, r, s := b.Body.Counts()
	imgui.Text(fmt.Sprintf("%d faces · %d edges · %d vertices", f, e, v))
	imgui.Text(fmt.Sprintf("%d shell · %d rings · genus %d", s, r, brep.Genus(b.Body)))
	if err := brep.Check(b.Body); err != nil {
		imgui.TextColored(colErr, "Invalid: "+err.Error())
	} else {
		imgui.TextColored(colOK, "Valid solid")
	}
	if b.Mesh != nil && b.mass == nil {
		imgui.Text(fmt.Sprintf("Volume  ~%s mm³", fmtSig(b.Mesh.Volume())))
		imgui.Text(fmt.Sprintf("Area    ~%s mm²", fmtSig(b.Mesh.Area())))
	}
	a.massPanel(b)
}

// timelinePanel is the history: every feature in order, the rollback
// marker after the last one in play.
func (a *App) timelinePanel() {
	if !imgui.Begin("Timeline") {
		imgui.End()
		return
	}
	d := a.doc.D
	n := d.ActiveCount()
	step := func(icon rune, tip string, to int, ok bool) {
		imgui.BeginDisabledV(!ok)
		if imgui.SmallButton(ui.Icon(icon, "") + "##" + tip) {
			a.rollTo(to)
		}
		a.markItem("tl:" + tip)
		imgui.EndDisabled()
		imgui.SetItemTooltip(tip)
		imgui.SameLine()
	}
	step(0xf048, "Roll back to the start", 0, n > 0)
	step(0xf04a, "Roll back one step", n-1, n > 0)
	step(0xf04e, "Roll forward one step", n+1, n < len(d.Features))
	step(0xf051, "Roll to the end", -1, n < len(d.Features))
	imgui.TextDisabled("|")
	imgui.SameLine()
	if len(d.Features) == 0 {
		imgui.TextDisabled("The design's steps appear here.")
	}
	marker := func() {
		imgui.PushStyleColorVec4(imgui.ColButton, imgui.Vec4{X: 0.88, Y: 0.62, Z: 0.28, W: 1})
		imgui.ButtonV("##marker", imgui.Vec2{X: 6, Y: imgui.FrameHeight()})
		imgui.PopStyleColor()
		imgui.SetItemTooltip("Timeline marker: steps after it are rolled back")
		imgui.SameLine()
	}
	if n == 0 && len(d.Features) > 0 {
		marker()
	}
	for i, f := range d.Features {
		sel := featureOf(a.browserSel) == f.ID
		pushed := 0
		switch {
		case sel:
			imgui.PushStyleColorVec4(imgui.ColButton, colAccent)
			pushed++
		case f.Err != "":
			imgui.PushStyleColorVec4(imgui.ColButton, imgui.Vec4{X: 0.45, Y: 0.15, Z: 0.13, W: 1})
			pushed++
		}
		if !f.Active {
			imgui.PushStyleColorVec4(imgui.ColText, imgui.Vec4{X: 0.5, Y: 0.5, Z: 0.5, W: 1})
			pushed++
		}
		if imgui.Button(fmt.Sprintf("%s##t%d", ui.Icon(kindIcon(f.Kind), f.Name), f.ID)) {
			a.browserSel = bodyKey(f.ID, 0)
		}
		a.markItem("step:" + f.Name)
		imgui.PopStyleColorV(int32(pushed))
		if f.Err != "" {
			imgui.SetItemTooltip(f.Err)
		}
		if imgui.IsItemHovered() && imgui.IsMouseDoubleClicked(imgui.MouseButtonLeft) && f.Kind == doc.KSketch && a.sk == nil {
			if it := a.doc.Sketch(bodyKey(f.ID, 0)); it != nil {
				a.enterSketch(it)
			}
		}
		a.featureMenu(f)
		imgui.SameLine()
		if i == n-1 && n < len(d.Features) {
			marker()
		}
	}
	imgui.NewLine()
	imgui.End()
}

// paramsWindow is the parameters table.
func (a *App) paramsWindow() {
	if !a.showParams {
		return
	}
	imgui.SetNextWindowSizeV(imgui.Vec2{X: 520, Y: 300}, imgui.CondFirstUseEver)
	if imgui.BeginV(ui.Icon(0xf1ec, "Parameters")+"###Parameters", &a.showParams, 0) {
		imgui.TextDisabled("Use a parameter's name in any value: dimensions, distances, sizes.")
		d := a.doc.D
		del := -1
		if imgui.BeginTableV("params", 4, imgui.TableFlagsRowBg|imgui.TableFlagsSizingStretchProp, imgui.Vec2{}, 0) {
			imgui.TableSetupColumnV("Name", imgui.TableColumnFlagsWidthStretch, 1, 0)
			imgui.TableSetupColumnV("Expression", imgui.TableColumnFlagsWidthStretch, 2, 0)
			imgui.TableSetupColumnV("Value", imgui.TableColumnFlagsWidthStretch, 1, 0)
			imgui.TableSetupColumnV("", imgui.TableColumnFlagsWidthFixed, 24, 0)
			imgui.TableHeadersRow()
			for i, p := range d.Params {
				imgui.PushIDStr(fmt.Sprint(i))
				imgui.TableNextRow()
				imgui.TableNextColumn()
				imgui.SetNextItemWidth(-1)
				imgui.InputTextWithHint("##n", "name", &p.Name, 0, nil)
				a.markItem("param:" + fmt.Sprint(i) + ":name")
				if imgui.IsItemDeactivatedAfterEdit() {
					a.doc.Commit()
				}
				imgui.TableNextColumn()
				imgui.SetNextItemWidth(-1)
				imgui.InputTextWithHint("##e", "expression", &p.Expr, imgui.InputTextFlagsAutoSelectAll, nil)
				a.markItem("param:" + fmt.Sprint(i) + ":expr")
				if imgui.IsItemDeactivatedAfterEdit() {
					a.doc.Commit()
				}
				imgui.TableNextColumn()
				if p.Err != "" {
					imgui.TextColored(colErr, "error")
					imgui.SetItemTooltip(p.Err)
				} else {
					if p.IsText {
						imgui.TextUnformatted("\"" + p.Text + "\"")
					} else {
						imgui.TextUnformatted(fmtLen(p.Value))
					}
				}
				imgui.TableNextColumn()
				if imgui.SmallButton("x") {
					del = i
				}
				imgui.PopID()
			}
			imgui.EndTable()
		}
		if del >= 0 {
			d.Params = append(d.Params[:del], d.Params[del+1:]...)
			a.doc.Commit()
		}
		if imgui.Button(ui.Icon(0xf067, "Add parameter")) {
			_ = a.cmds.Run("param.add", "")
		}
		a.markItem("btn:param.add")
	}
	imgui.End()
}

// opCombo picks what a body-making feature does with the bodies it touches.
func (a *App) opCombo(f *doc.Feature) {
	cur := f.Inputs["Operation"]
	if cur == "" {
		cur = doc.Operations[0]
	}
	if m, ok := a.combo("Operation", cur, doc.Operations, nil); ok {
		f.Inputs["Operation"] = m
		// an extrude on a face cuts into the body, joins out of it
		if f.Kind == doc.KExtrude && len(f.Profiles) > 0 && f.Inputs["Direction"] != "Symmetric" {
			if sk := a.doc.D.Feature(f.Profiles[0].Sketch); sk != nil && sk.Plane != nil && sk.Plane.Feature != 0 {
				f.Inputs["Flip"] = map[bool]string{true: "1", false: "0"}[m == "Cut"]
			}
		}
		a.doc.Commit()
	}
	a.markItemLeft("prop:Operation")
}
