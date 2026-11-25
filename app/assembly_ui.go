package app

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"time"

	imgui "github.com/AllenDang/cimgui-go/imgui"
	"github.com/go-gl/mathgl/mgl32"

	"forma/brep"
	"forma/doc"
	"forma/geom"
	"forma/render"
	"forma/ui"
)

// Assemblies in the app: insert a design as a component, copy it, joint
// two components, drive a joint's motion, check for interference.

const (
	icComponent    = 0xf12e // puzzle piece
	icJoint        = 0xf0c1 // link
	icInterference = 0xf071 // warning
	icCopy         = 0xf0c5
	icPlay         = 0xf04b
	icPause        = 0xf04c
)

type assemblyState struct {
	componentPath string
	clashes       []clashView
	showClashes   bool
	animJoint     int // the joint being animated (feature ID), 0: none
	animLast      time.Time
	animDir       float64
}

type clashView struct {
	c   doc.Clash
	gpu *render.TriMesh
}

func (a *App) registerAssemblyCommands(c *Commands) {
	c.add(&Command{ID: "assemble.component", Label: "Insert component…", Group: "Assemble", Icon: icComponent,
		Run: func(arg string) {
			if arg != "" {
				a.insertComponent(arg)
				return
			}
			a.pickFile(false, "Insert a design as a component", "Insert", startIn(a.asm.componentPath, ""), designFilter, a.insertComponent)
		}})
	c.add(&Command{ID: "assemble.copy", Label: "Copy component", Group: "Assemble", Icon: icCopy,
		Enabled: func() bool { return a.selComponent() != nil },
		Run:     func(string) { a.copyComponent() }})
	c.add(&Command{ID: "assemble.joint", Label: "Joint (pick the fixed end, then the moving end)", Group: "Assemble", Icon: icJoint,
		Run: func(arg string) { a.newJoint(arg) }})
	c.add(&Command{ID: "assemble.interference", Label: "Interference", Group: "Assemble", Icon: icInterference,
		Run: func(string) { a.checkInterference() }})
	c.add(&Command{ID: "assemble.update", Label: "Update component from its file", Group: "Assemble",
		Enabled: func() bool { f := a.selComponent(); return f != nil && f.Ref == 0 && f.Inputs["Path"] != "" },
		Run:     func(string) { a.updateComponent() }})
}

// selComponent is the component picked in the browser or timeline, or the
// component of the picked body or of the selected faces and edges.
func (a *App) selComponent() *doc.Feature {
	if f := a.selFeature(); f != nil && f.Kind == doc.KComponent {
		return f
	}
	keys := []int{a.browserSel}
	for _, r := range a.sel {
		keys = append(keys, r.Item)
	}
	for _, k := range keys {
		if c := a.doc.D.ComponentOf(k); c != 0 {
			return a.doc.D.Feature(c)
		}
	}
	return nil
}

func (a *App) insertComponent(path string) {
	a.asm.componentPath = path
	data, err := os.ReadFile(path)
	if err != nil {
		a.notify("Insert failed: %v", err)
		return
	}
	if abs, _ := filepath.Abs(path); a.doc.Path != "" && sameFile(abs, a.doc.Path) {
		a.notify("Inserting this design into itself puts in its last saved state")
	}
	f := doc.NewComponent(path, data)
	f.Inputs["Path"] = path
	// the first component is grounded: the rest are joined to it
	first := true
	for _, g := range a.doc.D.Features {
		first = first && g.Kind != doc.KComponent
	}
	if first {
		f.Inputs["Grounded"] = "1"
	}
	a.placeNew(f)
	a.notify("Inserted %s as a component", filepath.Base(path))
}

func sameFile(a, b string) bool {
	x, err1 := os.Stat(a)
	y, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(x, y)
}

// placeNew inserts a component and moves it clear of what is there: its
// pivot at its middle, beside the design along X.
func (a *App) placeNew(f *doc.Feature) {
	before := a.doc.Bounds()
	a.doc.D.Insert(f)
	a.doc.Commit()
	if f.Err != "" {
		a.notify("Component failed: %s", f.Err)
		a.doc.Remove(f.ID)
		return
	}
	var lb geom.Box
	for _, b := range f.Local {
		lb.Union(b.Bounds())
	}
	c := lb.Center()
	set := func(k string, x float64) {
		if f.Inputs[k] == "" || f.Inputs[k] == "0" {
			f.Inputs[k] = fmtLen(x)
		}
	}
	set("Pivot X", c.X)
	set("Pivot Y", c.Y)
	set("Pivot Z", c.Z)
	var wb geom.Box
	for _, b := range f.Bodies {
		wb.Union(b.Bounds())
	}
	if !before.Empty() && boxesOverlap(before, wb) {
		gap := math.Max(5, 0.1*math.Max(before.Diagonal(), wb.Diagonal()))
		mx, _ := a.doc.D.Eval(f.Inputs["Move X"])
		f.Inputs["Move X"] = fmtLen(mx + before.Max.X - wb.Min.X + gap)
	}
	a.doc.Commit()
	if len(f.Bodies) > 0 {
		a.browserSel = bodyKey(f.ID, 0)
	}
	a.cam.Fit(a.doc.Bounds(), a.aspect(), a.animate())
}

func boxesOverlap(a, b geom.Box) bool {
	return a.Min.X <= b.Max.X && b.Min.X <= a.Max.X && a.Min.Y <= b.Max.Y && b.Min.Y <= a.Max.Y &&
		a.Min.Z <= b.Max.Z && b.Min.Z <= a.Max.Z
}

func (a *App) copyComponent() {
	src := a.selComponent()
	if src == nil {
		a.notify("Pick a component to copy")
		return
	}
	for src.Ref != 0 {
		if s := a.doc.D.Feature(src.Ref); s != nil {
			src = s
		} else {
			break
		}
	}
	f := doc.NewInstance(src)
	a.placeNew(f)
}

func (a *App) updateComponent() {
	f := a.selComponent()
	if f == nil || f.Inputs["Path"] == "" {
		return
	}
	data, err := os.ReadFile(f.Inputs["Path"])
	if err != nil {
		a.notify("Update failed: %v", err)
		return
	}
	if string(data) == f.Data {
		a.notify("%s is up to date", f.Name)
		return
	}
	f.Data = string(data)
	a.doc.Commit()
	a.notify("Updated %s from %s", f.Name, filepath.Base(f.Inputs["Path"]))
}

// newJoint joins the two selected faces, edges or vertices: the first one's
// component stays, the second's moves onto it.
func (a *App) newJoint(typ string) {
	if typ == "" {
		typ = "Rigid"
	}
	var ends []doc.JointRef
	var comps []int
	for _, r := range a.sel {
		var j doc.JointRef
		var err error
		switch r.Kind {
		case KindFace:
			b, fc := a.refFace(r)
			if fc == nil {
				continue
			}
			j, err = a.doc.D.JointRefFor(b.ID, fc, nil, nil, geom.Vec3{})
		case KindEdge:
			b, e := a.refEdge(r)
			if e == nil {
				continue
			}
			j, err = a.doc.D.JointRefFor(b.ID, nil, e, nil, e.Curve.Eval((e.T0+e.T1)/2))
		case KindVertex:
			b, v := a.refVertex(r)
			if v == nil {
				continue
			}
			j, err = a.doc.D.JointRefFor(b.ID, nil, nil, v, v.P)
		default:
			continue
		}
		if err != nil {
			a.notify("Joint: %v", err)
			return
		}
		ends = append(ends, j)
		comps = append(comps, j.Feature)
	}
	if len(ends) != 2 {
		a.notify("Joint: pick a face, edge or vertex on the part that stays, then (Shift) one on the part that moves")
		return
	}
	if comps[0] == comps[1] {
		a.notify("Joint: the two ends are on the same component")
		return
	}
	f := doc.NewJoint(ends[0], ends[1], typ)
	a.doc.D.Insert(f)
	a.doc.Commit()
	if f.Err != "" {
		a.notify("Joint failed: %s", f.Err)
		a.doc.Remove(f.ID)
		return
	}
	a.sel, a.hover = a.sel[:0], Ref{}
	a.browserSel = bodyKey(f.ID, 0)
	a.notify("%s joint: %s onto %s", typ, a.featName(ends[1].Feature), a.featName(ends[0].Feature))
}

func (a *App) featName(id int) string {
	if f := a.doc.D.Feature(id); f != nil {
		return f.Name
	}
	return "?"
}

// jointedBy is the active joint that moves a component (0: none).
func (a *App) jointedBy(comp int) int {
	for _, f := range a.doc.D.Features {
		if f.Kind == doc.KJoint && f.Active && len(f.Joint) == 2 && f.Joint[1].Feature == comp {
			return f.ID
		}
	}
	return 0
}

// assemblyProperties shows a component's or joint's settings.
func (a *App) assemblyProperties(f *doc.Feature) bool {
	switch f.Kind {
	case doc.KComponent:
		src := "From " + f.Inputs["File"]
		if f.Ref != 0 {
			src = "A copy of " + a.featName(f.Ref)
		}
		imgui.TextDisabled(src)
		a.screwProperties(f)
		a.groundedProperty(f)
		if j := a.jointedBy(f.ID); j != 0 {
			imgui.TextWrapped(fmt.Sprintf("Placed by %s (its placement here comes first).", a.featName(j)))
		}
		imgui.SeparatorText("Placement")
		for _, k := range []string{"Move X", "Move Y", "Move Z"} {
			a.exprField(f, k, " mm")
		}
		for _, k := range []string{"Rotate X", "Rotate Y", "Rotate Z"} {
			a.exprField(f, k, "°")
		}
		imgui.TextDisabled("Drag the gizmo's arrows, or G for its rings")
		if imgui.Button(ui.Icon(icCopy, "Copy")) {
			a.copyComponent()
		}
		if f.Ref == 0 && f.Inputs["Path"] != "" {
			imgui.SameLine()
			if imgui.Button("Update from file") {
				a.updateComponent()
			}
		}
		return true
	case doc.KJoint:
		typ := f.Inputs["Type"]
		if t, ok := a.combo("Type", typ, doc.JointTypes, nil); ok {
			f.Inputs["Type"] = t
			a.doc.Commit()
		}
		if len(f.Joint) == 2 {
			imgui.TextDisabled(fmt.Sprintf("%s moves onto %s", a.featName(f.Joint[1].Feature), a.featName(f.Joint[0].Feature)))
		}
		motion := doc.MotionInput(f)
		for _, k := range []string{"Angle", "Distance"} {
			unit := " mm"
			if k == "Angle" {
				unit = "°"
			}
			label := k
			if motion != k && f.Inputs["Type"] != "Cylindrical" {
				label += " offset"
			}
			imgui.TextDisabled(label)
			a.exprField(f, k, unit)
		}
		flip := f.Inputs["Flip"] == "1"
		if imgui.Checkbox("Flip", &flip) {
			f.Inputs["Flip"] = map[bool]string{true: "1", false: "0"}[flip]
			a.doc.Commit()
		}
		if imgui.SmallButton("Swap the ends") && len(f.Joint) == 2 {
			f.Joint[0], f.Joint[1] = f.Joint[1], f.Joint[0]
			a.doc.Commit()
		}
		if motion != "" {
			imgui.SeparatorText("Motion")
			a.limitProperties(f, motion)
			a.motionSlider(f, motion)
		}
		return true
	}
	return false
}

// motionSlider drags a joint's motion (regenerating as it goes, one undo
// step when let go) and animates it.
func (a *App) motionSlider(f *doc.Feature, input string) {
	v, _ := a.doc.D.Eval(f.Inputs[input])
	x := float32(v)
	l, h := a.motionRange(f, input)
	lo, hi := float32(l), float32(h)
	imgui.SetNextItemWidth(-70)
	if imgui.SliderFloatV("##motion", &x, lo, hi, "%.1f", 0) {
		f.Inputs[input] = strconv.FormatFloat(math.Round(float64(x)*10)/10, 'f', -1, 64)
		a.doc.Regenerate()
	}
	if imgui.IsItemDeactivatedAfterEdit() {
		a.doc.Commit()
	}
	imgui.SameLine()
	playing := a.asm.animJoint == f.ID
	label := ui.Icon(icPlay, "Play")
	if playing {
		label = ui.Icon(icPause, "Stop")
	}
	if imgui.Button(label + "##anim") {
		if playing {
			a.stopAnimation()
		} else {
			a.asm.animJoint, a.asm.animLast, a.asm.animDir = f.ID, time.Now(), 1
		}
	}
	a.markItem("btn:joint.play")
}

// animate steps the animated joint: a revolute one turns at 90°/s, a
// slider goes back and forth across the design.
func (a *App) animateJoint() {
	if a.asm.animJoint == 0 {
		return
	}
	f := a.doc.D.Feature(a.asm.animJoint)
	if f == nil || !f.Active || doc.MotionInput(f) == "" {
		a.asm.animJoint = 0
		return
	}
	now := time.Now()
	dt := math.Min(now.Sub(a.asm.animLast).Seconds(), 0.1)
	if a.capture != nil {
		dt = 1.0 / 30
	}
	a.asm.animLast = now
	in := doc.MotionInput(f)
	v, _ := a.doc.D.Eval(f.Inputs[in])
	if lo, hi, ok := a.doc.D.Limits(f, in); ok && !math.IsInf(lo, 0) && !math.IsInf(hi, 0) {
		// back and forth between its limits
		speed := 90.0
		if in == "Distance" {
			speed = math.Max(10, (hi-lo)/2)
		}
		v += a.asm.animDir * speed * dt
		if v >= hi || v <= lo {
			v = math.Max(lo, math.Min(hi, v))
			a.asm.animDir = -a.asm.animDir
		}
	} else if in == "Angle" {
		v = math.Mod(v+90*dt+180, 360) - 180
	} else {
		r := math.Max(10, a.doc.Bounds().Diagonal()/4)
		v += a.asm.animDir * r * dt
		if math.Abs(v) > r {
			a.asm.animDir = -a.asm.animDir
		}
	}
	f.Inputs[in] = strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
	a.doc.Regenerate()
}

func (a *App) stopAnimation() {
	if a.asm.animJoint != 0 {
		a.asm.animJoint = 0
		a.doc.Commit()
	}
}

// checkInterference finds the overlapping bodies and shows them.
func (a *App) checkInterference() {
	a.freeClashes()
	visible := map[int]bool{}
	for _, b := range a.doc.Bodies {
		visible[b.ID] = b.Visible
	}
	for _, c := range a.doc.D.Interference(func(r *doc.BodyRef) bool { return visible[r.Key] }) {
		cv := clashView{c: c}
		if c.Shape != nil {
			m := brep.Tessellate(c.Shape, brep.DefaultMeshOptions(c.Shape))
			ids := make([]uint32, len(m.Pos))
			cv.gpu = render.NewTriMesh(m.Pos, m.Nrm, ids, m.Tris)
		}
		a.asm.clashes = append(a.asm.clashes, cv)
	}
	a.asm.showClashes = true
	n := 0
	for _, c := range a.asm.clashes {
		if c.c.Err == nil {
			n++
		}
	}
	if n == 0 {
		a.notify("No interference between the visible bodies")
	} else {
		a.notify("%d %s overlap", n, plural(n, "pair of bodies", "pairs of bodies"))
	}
}

func (a *App) freeClashes() {
	for _, c := range a.asm.clashes {
		if c.gpu != nil {
			c.gpu.Delete()
		}
	}
	a.asm.clashes = nil
}

func (a *App) drawClashes(v render.View) {
	if !a.asm.showClashes {
		return
	}
	for _, c := range a.asm.clashes {
		if c.gpu != nil {
			a.r.DrawFaces(v, c.gpu, render.Paint{Color: mgl32.Vec4{0.95, 0.2, 0.15, 0.85}, OnTop: true}, false)
		}
	}
}

// clashWindow lists what overlaps.
func (a *App) clashWindow() {
	if !a.asm.showClashes {
		return
	}
	imgui.SetNextWindowSizeV(imgui.Vec2{X: 380, Y: 0}, imgui.CondAppearing)
	open := true
	if imgui.BeginV(ui.Icon(icInterference, "Interference")+"###clashes", &open, imgui.WindowFlagsNoSavedSettings|imgui.WindowFlagsAlwaysAutoResize) {
		if len(a.asm.clashes) == 0 {
			imgui.Text("Nothing overlaps.")
		}
		for i, c := range a.asm.clashes {
			name := fmt.Sprintf("%s · %s", c.c.A.Name, c.c.B.Name)
			if c.c.Err != nil {
				imgui.TextColored(colWarn, name)
				imgui.TextWrapped("  couldn't be checked: " + c.c.Err.Error())
				continue
			}
			imgui.Text(fmt.Sprintf("%d. %s", i+1, name))
			imgui.SameLine()
			imgui.TextDisabled(fmtSig(c.c.Volume) + " mm³")
		}
		if imgui.Button("Check again") {
			a.checkInterference()
		}
		imgui.SameLine()
		if imgui.Button("Close") {
			open = false
		}
	}
	imgui.End()
	if !open {
		a.asm.showClashes = false
		a.freeClashes()
	}
}
