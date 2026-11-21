package app

import (
	"math"

	imgui "github.com/AllenDang/cimgui-go/imgui"
	"github.com/AllenDang/cimgui-go/imguizmo"
	"github.com/go-gl/mathgl/mgl32"

	"forma/doc"
	"forma/geom"
)

// gizmoMode is what dragging the selected feature does: resize it by its
// arrows, or move, turn or scale it with the gizmo.
type gizmoMode int

const (
	modeResize gizmoMode = iota
	modeMove
	modeRotate
	modeScale
)

var modeNames = [...]string{"Resize", "Move", "Turn", "Scale"}

// a scale drag on a primitive: its sizes when the drag began, and the ratio
// dragged to so far
type scaleDrag struct {
	on    bool
	ratio float64
	base  map[string]float64
}

func isPrimitive(f *doc.Feature) bool {
	switch f.Kind {
	case doc.KBox, doc.KCylinder, doc.KCone, doc.KSphere, doc.KTorus:
		return true
	}
	return false
}

// sizeInputs are a primitive's size inputs (what Scale multiplies).
func sizeInputs(f *doc.Feature) []string {
	switch f.Kind {
	case doc.KBox:
		return []string{"Width", "Depth", "Height"}
	case doc.KCylinder:
		return []string{"Radius", "Height"}
	case doc.KCone:
		return []string{"Base radius", "Top radius", "Height"}
	case doc.KSphere:
		return []string{"Radius"}
	case doc.KTorus:
		return []string{"Major radius", "Tube radius"}
	}
	return nil
}

// transformable: the selected feature makes or moves bodies the bar works on.
func (a *App) transformable(f *doc.Feature) bool {
	if f == nil || a.sk != nil || !f.Active || f.Err != "" {
		return false
	}
	switch f.Kind {
	case doc.KMove:
		return true
	case doc.KComponent:
		return a.jointedBy(f.ID) == 0 && !doc.Grounded(f)
	case doc.KExtrude, doc.KRevolve, doc.KImport:
		return len(f.Bodies) > 0
	}
	return isPrimitive(f)
}

// syncMode puts the mode right for a newly selected feature: a move feature
// starts on Move, anything else on Resize.
func (a *App) syncMode(f *doc.Feature) {
	if f == nil {
		a.modeFor = 0
		return
	}
	if a.modeFor == f.ID {
		return
	}
	a.modeFor = f.ID
	if f.Kind == doc.KMove || f.Kind == doc.KComponent {
		if a.gizmoMode == modeResize {
			a.gizmoMode = modeMove
		}
	} else {
		a.gizmoMode = modeResize
	}
}

// modeBar is the Resize · Move · Turn · Scale bar at the top of the view for
// the selected feature.
func (a *App) modeBar() {
	f := a.selFeature()
	if !a.transformable(f) {
		a.syncMode(nil)
		return
	}
	a.syncMode(f)
	vp := &a.vp
	imgui.SetNextWindowPosV(imgui.Vec2{X: vp.pos.X + vp.size.X/2, Y: vp.pos.Y + 10}, imgui.CondAlways, imgui.Vec2{X: 0.5})
	imgui.SetNextWindowBgAlpha(0.92)
	flags := imgui.WindowFlagsNoDecoration | imgui.WindowFlagsAlwaysAutoResize | imgui.WindowFlagsNoMove |
		imgui.WindowFlagsNoSavedSettings | imgui.WindowFlagsNoFocusOnAppearing | imgui.WindowFlagsNoNav
	if imgui.BeginV("##modebar", nil, flags) {
		for i, name := range modeNames {
			m := gizmoMode(i)
			if i > 0 {
				imgui.SameLine()
			}
			on := a.gizmoMode == m
			if on {
				imgui.PushStyleColorVec4(imgui.ColButton, accentVec4(0.85))
				imgui.PushStyleColorVec4(imgui.ColText, imgui.Vec4{X: 0.08, Y: 0.08, Z: 0.1, W: 1})
			}
			if imgui.Button(name + "##mode") {
				a.pickMode(f, m)
			}
			if on {
				imgui.PopStyleColorV(2)
			}
			a.markItem("btn:mode." + map[gizmoMode]string{modeResize: "resize", modeMove: "move", modeRotate: "turn", modeScale: "scale"}[m])
		}
		imgui.SameLine()
		imgui.TextDisabled("  Ctrl free · Shift fine")
	}
	imgui.End()
}

// pickMode switches what dragging does. Resizing a move feature goes back
// to the feature that made its body; moving, turning or scaling something
// that has no such inputs adds a move feature after it.
func (a *App) pickMode(f *doc.Feature, m gizmoMode) {
	switch {
	case m == modeResize && f.Kind == doc.KMove:
		for _, key := range f.Targets {
			if owner := a.doc.D.Feature(featureOf(key)); owner != nil {
				a.browserSel = bodyKey(owner.ID, key%doc.KeysPerFeature)
				a.sel = a.sel[:0]
				a.modeFor = owner.ID
				a.gizmoMode = modeResize
				return
			}
		}
		a.notify("Nothing to resize: the moved body's own feature is gone")
		return
	case f.Kind == doc.KMove || f.Kind == doc.KComponent:
	case a.primMovable(f) && m != modeRotate:
		// a primitive moves and scales by its own inputs (not one whose body
		// has since been joined to or changed: that body moves as a whole)
	case m != modeResize:
		// a move feature after it, on its bodies
		var keys []int
		var box geom.Box
		for k := range f.Bodies {
			if b := a.doc.Body(bodyKey(f.ID, k)); b != nil {
				keys = append(keys, b.ID)
				box.Union(b.Body.Bounds())
			}
		}
		if len(keys) == 0 {
			a.notify("Its body has been joined into another: select that body to move it")
			return
		}
		mv := doc.NewMove(keys, box.Center())
		a.newFeature(mv, "")
		a.modeFor = mv.ID
	}
	a.gizmoMode = m
}

// moveGizmo shows ImGuizmo's arrows, rings or scale handles on the selected
// feature: a move feature's (or component's) placement and scale, or a
// primitive's position and sizes. Dragging snaps to the increments (Ctrl
// free, Shift fine).
func (a *App) moveGizmo(dl *imgui.DrawList) {
	vp := &a.vp
	vp.gizmoHot = false
	f := a.selFeature()
	if !a.transformable(f) || a.gizmoMode == modeResize {
		vp.gizmoDrag = false
		a.scale.on = false
		return
	}
	prim := a.primMovable(f)
	if !prim && f.Kind != doc.KMove && f.Kind != doc.KComponent {
		vp.gizmoDrag = false
		return
	}
	if prim && a.gizmoMode == modeRotate {
		return
	}
	// the gizmo's frame: where it stands, its axes and scale
	var x geom.Xform
	var pv geom.Vec3
	s := 1.0
	if prim {
		var p [3]float64
		for i, k := range []string{"X", "Y", "Z"} {
			v, err := a.doc.D.Eval(f.Inputs[k])
			if err != nil {
				return
			}
			p[i] = v
		}
		x = geom.Translate(geom.V3(p[0], p[1], p[2]))
		if a.scale.on {
			s = a.scale.ratio
		}
	} else {
		var err error
		if x, err = a.doc.D.MoveXform(f); err != nil {
			return
		}
		for i, k := range []string{"Pivot X", "Pivot Y", "Pivot Z"} {
			v, _ := a.doc.D.Eval(f.Inputs[k])
			switch i {
			case 0:
				pv.X = v
			case 1:
				pv.Y = v
			default:
				pv.Z = v
			}
		}
		if f.Kind == doc.KMove {
			s, _ = a.doc.D.MoveScale(f)
		}
	}
	at := x.Pt(pv)
	c0, c1, c2 := x.R[0].Scale(s), x.R[1].Scale(s), x.R[2].Scale(s)
	m := mgl32.Mat4{
		float32(c0.X), float32(c0.Y), float32(c0.Z), 0,
		float32(c1.X), float32(c1.Y), float32(c1.Z), 0,
		float32(c2.X), float32(c2.Y), float32(c2.Z), 0,
		float32(at.X), float32(at.Y), float32(at.Z), 1,
	}
	before := m
	view, proj := vp.view.View, a.cam.GizmoProj(a.aspect())
	imguizmo.SetOrthographic(a.cam.Ortho)
	// the arrows point along +X, +Y, +Z whichever way the view looks (ImGuizmo
	// turns them toward the camera otherwise, so they swap ends as it orbits)
	imguizmo.AllowAxisFlip(false)
	imguizmo.SetGizmoSizeClipSpace(0.17)
	imguizmo.SetDrawlistV(dl)
	imguizmo.SetRect(vp.pos.X, vp.pos.Y, vp.size.X, vp.size.Y)
	op, space := imguizmo.TRANSLATE, imguizmo.WORLD
	switch a.gizmoMode {
	case modeRotate:
		op = imguizmo.ROTATE
	case modeScale:
		op, space = imguizmo.SCALE, imguizmo.LOCAL
		if f.Kind == doc.KComponent {
			return // components keep their design's size
		}
	}
	k := a.dragKeys()
	var snap *float32
	if k.snap && op != imguizmo.SCALE {
		st := a.increment()
		if op == imguizmo.ROTATE {
			st = a.snap.angleStep()
		}
		if k.fine {
			st /= 10
		}
		steps := [3]float32{float32(st), float32(st), float32(st)}
		snap = &steps[0]
	}
	imguizmo.ManipulateV(&view[0], &proj[0], op, space, &m[0], nil, snap, nil, nil)
	using := imguizmo.IsUsing()
	vp.gizmoHot = using || imguizmo.IsOver()
	if using && !vp.gizmoDrag && prim && op == imguizmo.SCALE {
		// a primitive's scale drag starts from its sizes now
		a.scale = scaleDrag{on: true, ratio: 1, base: map[string]float64{}}
		for _, name := range sizeInputs(f) {
			a.scale.base[name], _ = a.doc.D.Eval(f.Inputs[name])
		}
	}
	if m != before {
		g := func(i int) float64 { return float64(m[i]) }
		put := func(name string, v float64) {
			v = math.Round(v*1000) / 1000
			if v == 0 {
				v = 0 // no "-0"
			}
			f.Inputs[name] = fmtLen(v)
		}
		switch {
		case op == imguizmo.SCALE:
			// uniform: the column that changed most gives the ratio
			cols := [3]geom.Vec3{geom.V3(g(0), g(1), g(2)), geom.V3(g(4), g(5), g(6)), geom.V3(g(8), g(9), g(10))}
			ratio := 1.0
			for _, c := range cols {
				if r := c.Len() / s; math.Abs(math.Log(r)) > math.Abs(math.Log(ratio)) {
					ratio = r
				}
			}
			ns := s * ratio
			if k.snap {
				st := 0.05
				if k.fine {
					st = 0.005
				}
				ns = math.Max(st, math.Round(ns/st)*st)
			}
			if prim {
				if a.scale.on {
					a.scale.ratio = ns
					for name, v := range a.scale.base {
						put(name, v*ns)
					}
				}
			} else {
				put("Scale", ns)
			}
		case prim:
			put("X", g(12))
			put("Y", g(13))
			put("Z", g(14))
		default:
			r0 := geom.V3(g(0), g(1), g(2)).Norm()
			r1 := geom.V3(g(4), g(5), g(6)).Norm()
			r2 := geom.V3(g(8), g(9), g(10)).Norm()
			deg := 180 / math.Pi
			mv := geom.V3(g(12), g(13), g(14)).Sub(pv)
			put("Move X", mv.X)
			put("Move Y", mv.Y)
			put("Move Z", mv.Z)
			put("Rotate X", math.Atan2(r1.Z, r2.Z)*deg)
			put("Rotate Y", math.Asin(math.Max(-1, math.Min(1, -r0.Z)))*deg)
			put("Rotate Z", math.Atan2(r0.Y, r0.X)*deg)
		}
		a.doc.Regenerate()
	}
	if using {
		vp.gizmoDrag = true
	} else if vp.gizmoDrag {
		vp.gizmoDrag = false
		a.scale.on = false
		a.doc.Commit()
	}
}

// primMovable: a primitive that the gizmo moves and scales by its own
// inputs — its body is as it made it, or has gone into another (moving it
// then moves it within that one). A primitive whose body later features
// joined others into or changed is moved as a whole body by a move
// feature instead: moving its inputs pulled the joined shape apart.
func (a *App) primMovable(f *doc.Feature) bool {
	if !isPrimitive(f) {
		return false
	}
	for k, own := range f.Bodies {
		if b := a.doc.Body(bodyKey(f.ID, k)); b != nil && b.Body != own {
			return false
		}
	}
	return true
}
