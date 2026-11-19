package app

import (
	"fmt"
	"math"
	"strings"

	imgui "github.com/AllenDang/cimgui-go/imgui"
	"github.com/AllenDang/cimgui-go/imguizmo"
	"github.com/go-gl/mathgl/mgl32"

	"forma/brep"
	"forma/doc"
	"forma/geom"
	"forma/sketch"
	"forma/ui"
)

// Icons of the modelling features.
const (
	icMove    = 0xf0b2
	icMirror  = 0xf07e
	icPattern = 0xf00a
	icFillet  = 0xf1db
	icChamfer = 0xf0c8
	icHole    = 0xf192
	icShell   = 0xf466
	icMeasure = 0xf545
	icSection = 0xf0c4
)

// --- what is selected ---

func (a *App) refEdge(r Ref) (*BodyItem, *brep.Edge) {
	b := a.doc.Body(r.Item)
	if b == nil {
		return nil, nil
	}
	for _, e := range b.Body.Edges() {
		if e.ID == r.Local {
			return b, e
		}
	}
	return b, nil
}

func (a *App) refFace(r Ref) (*BodyItem, *brep.Face) {
	b := a.doc.Body(r.Item)
	if b == nil {
		return nil, nil
	}
	for _, f := range b.Body.Faces() {
		if f.ID == r.Local {
			return b, f
		}
	}
	return b, nil
}

func (a *App) refVertex(r Ref) (*BodyItem, *brep.Vertex) {
	b := a.doc.Body(r.Item)
	if b == nil {
		return nil, nil
	}
	for _, v := range b.Body.Vertices() {
		if v.ID == r.Local {
			return b, v
		}
	}
	return b, nil
}

// selectedEdges are the selected body edges, and the edges round the
// selected faces, as persistent references.
func (a *App) selectedEdges() []doc.EdgeRef {
	var out []doc.EdgeRef
	seen := map[*brep.Edge]bool{}
	add := func(e *brep.Edge) {
		if e == nil || seen[e] || e.HE[0] == nil || e.HE[1] == nil || e.HE[0].Loop.Face == e.HE[1].Loop.Face {
			return
		}
		seen[e] = true
		if ref, ok := doc.RefForEdge(e, e.Curve.Eval((e.T0+e.T1)/2)); ok {
			out = append(out, ref)
		}
	}
	for _, r := range a.sel {
		switch r.Kind {
		case KindEdge:
			_, e := a.refEdge(r)
			add(e)
		case KindFace:
			if _, f := a.refFace(r); f != nil {
				for _, l := range f.Loops {
					for _, h := range l.HalfEdges() {
						add(h.Edge)
					}
				}
			}
		}
	}
	return out
}

// selectedBodies are the bodies of the selected faces, edges and vertices,
// else the body picked in the browser.
func (a *App) selectedBodies() []int {
	seen := map[int]bool{}
	var out []int
	for _, r := range a.sel {
		switch r.Kind {
		case KindFace, KindEdge, KindVertex:
			if b := a.doc.Body(r.Item); b != nil && !seen[b.ID] {
				seen[b.ID] = true
				out = append(out, b.ID)
			}
		}
	}
	if len(out) == 0 {
		if b := a.doc.Body(a.browserSel); b != nil {
			out = append(out, b.ID)
		}
	}
	return out
}

// selectedFaces are the selected faces of one body.
func (a *App) selectedFaces() (int, []doc.FaceRef) {
	body := 0
	var out []doc.FaceRef
	for _, r := range a.sel {
		if r.Kind != KindFace {
			continue
		}
		b, f := a.refFace(r)
		if f == nil || (body != 0 && b.ID != body) {
			continue
		}
		body = b.ID
		out = append(out, doc.FaceRef{Feature: b.F.ID, Body: b.K, Face: f.Name})
	}
	return body, out
}

// selectedPoints are the selected points of one sketch, else the lone
// points (not ends of curves) of the sketch picked in the browser.
func (a *App) selectedPoints() (int, []int) {
	skID := 0
	var out []int
	for _, r := range a.sel {
		if r.Kind != KindSkPoint {
			continue
		}
		s := a.doc.Sketch(r.Item)
		if s == nil || (skID != 0 && s.F.ID != skID) {
			continue
		}
		skID = s.F.ID
		out = append(out, r.Local)
	}
	if len(out) == 0 {
		if s := a.doc.Sketch(a.browserSel); s != nil {
			used := map[int]bool{}
			for _, e := range s.S.Ents {
				if e.Kind != sketch.Point {
					for _, p := range s.S.PointsOf(e.ID) {
						used[p] = true
					}
				}
			}
			for _, e := range s.S.Ents {
				if e.Kind == sketch.Point && !used[e.ID] {
					out = append(out, e.ID)
				}
			}
			skID = s.F.ID
		}
	}
	return skID, out
}

// toolFeature is the feature picked in the browser or timeline when it is
// one that adds or cuts a shape (it can be patterned or mirrored).
func (a *App) toolFeature() *doc.Feature {
	f := a.selFeature()
	if f == nil || len(a.sel) > 0 {
		return nil
	}
	switch f.Kind {
	case doc.KSketch, doc.KMove, doc.KMirror, doc.KPattern, doc.KFillet, doc.KChamfer, doc.KShell:
		return nil
	}
	return f
}

// newFeature inserts a feature (args as "Name=expr,…"), selects it and
// says what went wrong, if anything.
func (a *App) newFeature(f *doc.Feature, arg string) {
	if a.sk != nil {
		a.finishSketch()
	}
	for _, kv := range strings.Split(arg, ",") {
		if name, val, ok := strings.Cut(kv, "="); ok {
			f.Inputs[strings.TrimSpace(name)] = strings.TrimSpace(val)
		}
	}
	a.doc.D.Insert(f)
	a.doc.Commit()
	a.sel = a.sel[:0]
	a.browserSel = bodyKey(f.ID, 0)
	if f.Err != "" {
		a.notify("%s: %s", f.Name, f.Err)
	}
}

func (a *App) registerModifyCommands(c *Commands) {
	notSketch := func() bool { return a.sk == nil }
	c.add(&Command{ID: "feature.fillet", Label: "Fillet", Group: "Modify", Icon: icFillet,
		Enabled: func() bool { return notSketch() && len(a.selectedEdges()) > 0 },
		Run:     func(arg string) { a.newFeature(doc.NewFillet(a.selectedEdges(), false), arg) }})
	c.add(&Command{ID: "feature.chamfer", Label: "Chamfer", Group: "Modify", Icon: icChamfer,
		Enabled: func() bool { return notSketch() && len(a.selectedEdges()) > 0 },
		Run:     func(arg string) { a.newFeature(doc.NewFillet(a.selectedEdges(), true), arg) }})
	c.add(&Command{ID: "feature.move", Label: "Move / rotate", Group: "Modify", Icon: icMove,
		Enabled: func() bool { return notSketch() && len(a.selectedBodies()) > 0 },
		Run: func(arg string) {
			keys := a.selectedBodies()
			var box geom.Box
			for _, k := range keys {
				if b := a.doc.Body(k); b != nil {
					box.Union(b.Body.Bounds())
				}
			}
			a.newFeature(doc.NewMove(keys, box.Center()), arg)
		}})
	repeat := func(make func(bodies, feats []int) *doc.Feature, arg string) {
		if f := a.toolFeature(); f != nil {
			a.newFeature(make(nil, []int{f.ID}), arg)
			return
		}
		if bs := a.selectedBodies(); len(bs) > 0 {
			a.newFeature(make(bs, nil), arg)
			return
		}
		a.notify("Select a body, or pick an extrude, revolve, hole or primitive in the timeline")
	}
	canRepeat := func() bool { return notSketch() && (a.toolFeature() != nil || len(a.selectedBodies()) > 0) }
	c.add(&Command{ID: "feature.mirror", Label: "Mirror", Group: "Modify", Icon: icMirror, Enabled: canRepeat,
		Run: func(arg string) { repeat(func(b, f []int) *doc.Feature { return doc.NewMirror(b, f) }, arg) }})
	c.add(&Command{ID: "feature.pattern", Label: "Linear pattern", Group: "Modify", Icon: icPattern, Enabled: canRepeat,
		Run: func(arg string) { repeat(func(b, f []int) *doc.Feature { return doc.NewPattern(b, f, false) }, arg) }})
	c.add(&Command{ID: "feature.circular", Label: "Circular pattern", Group: "Modify", Icon: ui.FaRotate, Enabled: canRepeat,
		Run: func(arg string) { repeat(func(b, f []int) *doc.Feature { return doc.NewPattern(b, f, true) }, arg) }})
	c.add(&Command{ID: "feature.hole", Label: "Hole", Group: "Modify", Icon: icHole,
		Enabled: func() bool { _, p := a.selectedPoints(); return notSketch() && len(p) > 0 },
		Run: func(arg string) {
			sk, pts := a.selectedPoints()
			if s := a.doc.D.Feature(sk); s != nil {
				s.Hidden = true
			}
			a.newFeature(doc.NewHole(sk, pts), arg)
		}})
	c.add(&Command{ID: "feature.shell", Label: "Shell", Group: "Modify", Icon: icShell,
		Enabled: func() bool { return notSketch() && len(a.selectedBodies()) > 0 },
		Run: func(arg string) {
			body, faces := a.selectedFaces()
			if body == 0 {
				bs := a.selectedBodies()
				body = bs[0]
			}
			a.newFeature(doc.NewShell(body, faces), arg)
		}})
	c.add(&Command{ID: "feature.set", Label: "Set an input of the selected feature", Group: "Design",
		Enabled: func() bool { return a.selFeature() != nil },
		Run: func(arg string) {
			f := a.selFeature()
			for _, kv := range strings.Split(arg, ",") {
				if name, val, ok := strings.Cut(kv, "="); ok {
					f.Inputs[strings.TrimSpace(name)] = strings.TrimSpace(val)
				}
			}
			a.doc.Commit()
			if f.Err != "" {
				a.notify("%s: %s", f.Name, f.Err)
			}
		}})
	c.add(&Command{ID: "feature.plane", Label: "Mirror about the selected face", Group: "Design",
		Enabled: func() bool {
			f := a.selFeature()
			_, ok := a.planeFor("")
			return f != nil && f.Kind == doc.KMirror && ok
		},
		Run: func(string) {
			ref, _ := a.planeFor("")
			f := a.selFeature()
			f.Plane = &ref
			a.sel = a.sel[:0]
			a.doc.Commit()
		}})
	c.add(&Command{ID: "view.section", Label: "Section view", Group: "View", Icon: icSection,
		Run: func(arg string) { a.section.toggle(a, arg) }})
	c.add(&Command{ID: "view.gizmo", Label: "Gizmo: move / turn", Group: "View",
		Run: func(arg string) { a.gizmoRotate = arg == "rotate" || (arg == "" && !a.gizmoRotate) }})
}

// --- properties ---

// modifyProperties draws a modelling feature's inputs; false if f isn't one.
func (a *App) modifyProperties(f *doc.Feature) bool {
	set := func(k, v string) {
		f.Inputs[k] = v
		a.doc.Commit()
	}
	comboIn := func(label, key string, options []string) {
		cur := f.Inputs[key]
		if cur == "" {
			cur = options[0]
		}
		if m, ok := a.combo(label, cur, options, nil); ok {
			set(key, m)
		}
		a.markItemLeft("prop:" + label)
	}
	targets := func() {
		if len(f.Sources) > 0 {
			var names []string
			for _, id := range f.Sources {
				if g := a.doc.D.Feature(id); g != nil {
					names = append(names, g.Name)
				}
			}
			imgui.TextWrapped("Repeats " + strings.Join(names, ", "))
		} else {
			var names []string
			for _, k := range f.Targets {
				if b := a.doc.Body(k); b != nil {
					names = append(names, b.Name)
				}
			}
			if len(names) == 0 {
				names = []string{fmt.Sprintf("%d bodies", len(f.Targets))}
			}
			imgui.TextWrapped("Bodies: " + strings.Join(names, ", "))
		}
		if bs := a.selectedBodies(); len(a.sel) > 0 && len(bs) > 0 {
			if imgui.SmallButton(fmt.Sprintf("Use the %d selected bodies", len(bs))) {
				f.Targets, f.Sources = bs, nil
				a.sel = a.sel[:0]
				a.doc.Commit()
			}
		}
	}
	switch f.Kind {
	case doc.KMove:
		targets()
		imgui.SeparatorText("Move")
		for _, k := range []string{"Move X", "Move Y", "Move Z"} {
			a.exprField(f, k, " mm")
		}
		imgui.SeparatorText("Turn (degrees, X then Y then Z)")
		for _, k := range []string{"Rotate X", "Rotate Y", "Rotate Z"} {
			a.exprField(f, k, "°")
		}
		imgui.SeparatorText("About")
		for _, k := range []string{"Pivot X", "Pivot Y", "Pivot Z"} {
			a.exprField(f, k, " mm")
		}
		imgui.Spacing()
		if imgui.RadioButtonBool("Drag to move", !a.gizmoRotate) {
			a.gizmoRotate = false
		}
		imgui.SameLine()
		if imgui.RadioButtonBool("Drag to turn", a.gizmoRotate) {
			a.gizmoRotate = true
		}
		imgui.TextDisabled("The arrows in the view move it.")
	case doc.KMirror:
		targets()
		cur := "Selected face"
		if f.Plane != nil && f.Plane.Std != "" {
			cur = strings.ToUpper(f.Plane.Std)
		}
		opts := append(append([]string{}, doc.MirrorPlanes...), "Selected face")
		if m, ok := a.combo("Plane", cur, opts, nil); ok {
			if m == "Selected face" {
				if ref, ok := a.planeFor(""); ok {
					f.Plane = &ref
				} else {
					a.notify("Select a flat face to mirror about")
				}
			} else {
				f.Plane = &doc.PlaneRef{Std: strings.ToLower(m)}
			}
			a.doc.Commit()
		}
		a.markItemLeft("prop:Plane")
		if f.Plane != nil && f.Plane.Std == "" {
			imgui.TextDisabled(planeText(f.Plane, a.doc.D))
		}
		if len(f.Sources) == 0 {
			comboIn("Result", "Operation", doc.CopyModes)
		}
	case doc.KPattern:
		targets()
		comboIn("Type", "Type", doc.PatternTypes)
		if f.Inputs["Type"] == "Circular" {
			if f.AxisFace != nil {
				imgui.TextWrapped(fmt.Sprintf("About the axis of %s", shortFace(f.AxisFace.Face)))
				if imgui.SmallButton("Use X / Y / Z instead") {
					f.AxisFace = nil
					a.doc.Commit()
				}
			} else {
				comboIn("Axis", "Axis", doc.Axes)
				for _, k := range []string{"Center X", "Center Y", "Center Z"} {
					a.exprField(f, k, " mm")
				}
			}
			if _, faces := a.selectedFaces(); len(faces) == 1 {
				if imgui.SmallButton("Turn about the selected round face") {
					r := faces[0]
					f.AxisFace = &r
					a.sel = a.sel[:0]
					a.doc.Commit()
				}
			}
			a.exprField(f, "Count", "")
			a.exprField(f, "Angle", "°")
		} else {
			comboIn("Direction", "Direction", doc.Axes)
			a.exprField(f, "Count", "")
			a.exprField(f, "Spacing", " mm")
			comboIn("Direction 2", "Direction 2", doc.Axes2)
			if d2 := f.Inputs["Direction 2"]; d2 != "" && d2 != "None" {
				a.exprField(f, "Count 2", "")
				a.exprField(f, "Spacing 2", " mm")
			}
		}
		if len(f.Sources) == 0 {
			comboIn("Result", "Operation", doc.CopyModes)
		}
	case doc.KFillet, doc.KChamfer:
		imgui.Text(fmt.Sprintf("%d edge(s)", len(f.Edges)))
		if es := a.selectedEdges(); len(es) > 0 {
			imgui.SameLine()
			if imgui.SmallButton(fmt.Sprintf("Use the %d selected", len(es))) {
				f.Edges = es
				a.sel = a.sel[:0]
				a.doc.Commit()
			}
		}
		if f.Kind == doc.KFillet {
			a.exprField(f, "Radius", " mm")
		} else {
			a.exprField(f, "Distance", " mm")
		}
	case doc.KHole:
		imgui.Text(fmt.Sprintf("%d hole(s) at the points of %s", len(f.Points), featName(a.doc.D, f.Ref)))
		if sk, pts := a.selectedPoints(); len(a.sel) > 0 && len(pts) > 0 {
			if imgui.SmallButton(fmt.Sprintf("Use the %d selected points", len(pts))) {
				f.Ref, f.Points = sk, pts
				a.sel = a.sel[:0]
				a.doc.Commit()
			}
		}
		comboIn("Type", "Type", doc.HoleTypes)
		a.exprField(f, "Diameter", " mm")
		switch f.Inputs["Type"] {
		case "Counterbore":
			a.exprField(f, "Bore diameter", " mm")
			a.exprField(f, "Bore depth", " mm")
		case "Countersink":
			a.exprField(f, "Sink diameter", " mm")
			imgui.TextDisabled("90° countersink")
		}
		comboIn("Extent", "Extent", doc.HoleExtents)
		if f.Inputs["Extent"] != "Through all" {
			a.exprField(f, "Depth", " mm")
			comboIn("Bottom", "Bottom", doc.HoleBottoms)
		}
	case doc.KShell:
		targets()
		imgui.Text(fmt.Sprintf("%d open face(s)", len(f.Faces)))
		if _, faces := a.selectedFaces(); len(faces) > 0 {
			imgui.SameLine()
			if imgui.SmallButton(fmt.Sprintf("Open the %d selected", len(faces))) {
				f.Faces = faces
				a.sel = a.sel[:0]
				a.doc.Commit()
			}
		}
		if len(f.Faces) == 0 {
			imgui.TextDisabled("None: hollow inside, closed all round")
		}
		a.exprField(f, "Thickness", " mm")
	default:
		return false
	}
	return true
}

func shortFace(n string) string {
	if i := strings.Index(n, "/"); i >= 0 {
		return n[i+1:]
	}
	return n
}

func featName(d *doc.Document, id int) string {
	if f := d.Feature(id); f != nil {
		return f.Name
	}
	return "a sketch that is gone"
}

// --- measuring ---

// samplePoints are points spread over a picked thing (for distances).
func (a *App) samplePoints(r Ref) []geom.Vec3 {
	switch r.Kind {
	case KindVertex:
		if _, v := a.refVertex(r); v != nil {
			return []geom.Vec3{v.P}
		}
	case KindEdge:
		if _, e := a.refEdge(r); e != nil {
			var out []geom.Vec3
			for _, t := range geom.SampleParams(e.Curve, e.T0, e.T1, 0.01, math.Max(e.Curve.Eval(e.T0).Dist(e.Curve.Eval(e.T1))/64, 0.05)) {
				out = append(out, e.Curve.Eval(t))
			}
			return out
		}
	case KindFace:
		b, f := a.refFace(r)
		if f == nil || b.Mesh == nil {
			return nil
		}
		var out []geom.Vec3
		for ti, t := range b.Mesh.Tris {
			if b.Mesh.TriFace[ti] != f.ID {
				continue
			}
			p0, p1, p2 := b.Mesh.Pos[t[0]], b.Mesh.Pos[t[1]], b.Mesh.Pos[t[2]]
			out = append(out, p0, p1, p2, p0.Add(p1).Add(p2).Scale(1.0/3))
		}
		return out
	case KindSkPoint:
		if s := a.doc.Sketch(r.Item); s != nil && s.S.Ent(r.Local) != nil {
			return []geom.Vec3{s.S.Plane.At2(s.S.P(r.Local))}
		}
	}
	return nil
}

// dirOf is a picked line's direction or a flat face's normal.
func (a *App) dirOf(r Ref) (geom.Vec3, bool, bool) { // dir, is a normal, ok
	switch r.Kind {
	case KindEdge:
		if _, e := a.refEdge(r); e != nil {
			if l, ok := e.Curve.(geom.Line); ok {
				return l.D, false, true
			}
			p, q := e.Verts[0].P, e.Verts[1].P
			if e.Verts[0] != e.Verts[1] && e.Curve.Eval((e.T0+e.T1)/2).Sub(p).Cross(q.Sub(p)).Len() < 1e-6*q.Dist(p)*q.Dist(p) {
				return q.Sub(p).Norm(), false, true
			}
		}
	case KindFace:
		if _, f := a.refFace(r); f != nil {
			if fr, ok := doc.PlaneOf(f); ok {
				return fr.Z, true, true
			}
		}
	}
	return geom.Vec3{}, false, false
}

func (a *App) faceArea(b *BodyItem, f *brep.Face) float64 {
	s := 0.0
	for ti, t := range b.Mesh.Tris {
		if b.Mesh.TriFace[ti] == f.ID {
			p0, p1, p2 := b.Mesh.Pos[t[0]], b.Mesh.Pos[t[1]], b.Mesh.Pos[t[2]]
			s += p1.Sub(p0).Cross(p2.Sub(p0)).Len() / 2
		}
	}
	return s
}

func edgeLength(e *brep.Edge) float64 {
	ts := geom.SampleParams(e.Curve, e.T0, e.T1, 1e-4, 1e9)
	l := 0.0
	for i := 1; i < len(ts); i++ {
		l += e.Curve.Eval(ts[i]).Dist(e.Curve.Eval(ts[i-1]))
	}
	return l
}

// measureLines are what the selection measures, one line each.
func (a *App) measureLines() []string {
	var out []string
	v3 := func(p geom.Vec3) string { return fmt.Sprintf("(%s, %s, %s)", fmtLen(p.X), fmtLen(p.Y), fmtLen(p.Z)) }
	if len(a.sel) == 1 {
		r := a.sel[0]
		switch r.Kind {
		case KindVertex:
			if _, v := a.refVertex(r); v != nil {
				out = append(out, "At "+v3(v.P))
			}
		case KindEdge:
			if _, e := a.refEdge(r); e != nil {
				out = append(out, "Length "+fmtLen(edgeLength(e))+" mm")
				if c, ok := e.Curve.(geom.Circle); ok {
					out = append(out, fmt.Sprintf("Radius %s mm · Ø %s mm", fmtLen(c.R), fmtLen(2*c.R)), "Centre "+v3(c.F.O))
				}
			}
		case KindFace:
			if b, f := a.refFace(r); f != nil && b.Mesh != nil {
				out = append(out, "Area "+fmtLen(a.faceArea(b, f))+" mm²")
				switch s := f.Surf.(type) {
				case geom.Cylinder:
					out = append(out, fmt.Sprintf("Cylinder · radius %s mm · Ø %s mm", fmtLen(s.R), fmtLen(2*s.R)))
				case geom.Sphere:
					out = append(out, fmt.Sprintf("Sphere · radius %s mm", fmtLen(s.R)))
				case geom.Plane:
					fr, _ := doc.PlaneOf(f)
					out = append(out, "Flat · normal "+v3(fr.Z))
				}
			}
		}
		return out
	}
	if len(a.sel) != 2 {
		return nil
	}
	r0, r1 := a.sel[0], a.sel[1]
	d0, n0, ok0 := a.dirOf(r0)
	d1, n1, ok1 := a.dirOf(r1)
	// two parallel flat faces: their distance, exactly
	if ok0 && ok1 && n0 && n1 && math.Abs(math.Abs(d0.Dot(d1))-1) < 1e-9 {
		_, f0 := a.refFace(r0)
		_, f1 := a.refFace(r1)
		p0, _ := doc.PlaneOf(f0)
		p1, _ := doc.PlaneOf(f1)
		out = append(out, "Distance "+fmtLen(math.Abs(p1.O.Sub(p0.O).Dot(d0)))+" mm (parallel faces)")
		return out
	}
	ps, qs := a.samplePoints(r0), a.samplePoints(r1)
	if len(ps) > 0 && len(qs) > 0 {
		best, bp, bq := math.Inf(1), geom.Vec3{}, geom.Vec3{}
		for _, p := range ps {
			for _, q := range qs {
				if d := p.Dist(q); d < best {
					best, bp, bq = d, p, q
				}
			}
		}
		exact := len(ps) == 1 && len(qs) == 1
		// a point and a flat face: exactly, square to the face
		if len(ps) == 1 && ok1 && n1 {
			_, f := a.refFace(r1)
			fr, _ := doc.PlaneOf(f)
			best, exact = math.Abs(ps[0].Sub(fr.O).Dot(fr.Z)), true
		} else if len(qs) == 1 && ok0 && n0 {
			_, f := a.refFace(r0)
			fr, _ := doc.PlaneOf(f)
			best, exact = math.Abs(qs[0].Sub(fr.O).Dot(fr.Z)), true
		}
		mark := "~"
		if exact {
			mark = ""
		}
		out = append(out, "Distance "+mark+fmtLen(best)+" mm")
		if exact && len(ps) == 1 && len(qs) == 1 {
			d := bq.Sub(bp)
			out = append(out, fmt.Sprintf("ΔX %s  ΔY %s  ΔZ %s", fmtLen(d.X), fmtLen(d.Y), fmtLen(d.Z)))
		}
	}
	if ok0 && ok1 {
		c := math.Abs(d0.Dot(d1))
		ang := math.Acos(math.Min(1, c)) * 180 / math.Pi
		if n0 != n1 {
			ang = 90 - ang // a line against a face's normal
		}
		out = append(out, fmt.Sprintf("Angle %s°", fmtLen(ang)))
	}
	return out
}

// --- mass properties ---

func (a *App) massPanel(b *BodyItem) {
	if b.mass == nil {
		if imgui.SmallButton(ui.Icon(icMeasure, "Mass properties") + fmt.Sprintf("##mp%d", b.ID)) {
			mp := brep.Mass(b.Body, 2e-5)
			b.mass = &mp
		}
		a.markItem("btn:mass")
		return
	}
	mp := b.mass
	imgui.SetNextItemWidth(90)
	imgui.InputTextWithHint("Density g/cm³##dens", "7.85", &a.density, imgui.InputTextFlagsAutoSelectAll, nil)
	rho, err := a.doc.D.Eval(a.density)
	if err != nil {
		rho = 7.85
	}
	mass := mp.Volume * rho / 1000 // g
	imgui.Text(fmt.Sprintf("Volume   %s mm³", fmtSig(mp.Volume)))
	imgui.Text(fmt.Sprintf("Area     %s mm²", fmtSig(mp.Area)))
	imgui.Text(fmt.Sprintf("Mass     %s g", fmtSig(mass)))
	c := mp.Centroid
	imgui.Text(fmt.Sprintf("Centroid (%s, %s, %s)", fmtLen(round4(c.X)), fmtLen(round4(c.Y)), fmtLen(round4(c.Z))))
	k := rho / 1e6 // mm⁵ → kg·mm²
	imgui.Text(fmt.Sprintf("Inertia about it, kg·mm²:"))
	imgui.Text(fmt.Sprintf("  Ixx %s  Iyy %s  Izz %s", fmtSig(mp.Inertia[0][0]*k), fmtSig(mp.Inertia[1][1]*k), fmtSig(mp.Inertia[2][2]*k)))
	imgui.Text(fmt.Sprintf("  principal %s, %s, %s", fmtSig(mp.Principal[0]*k), fmtSig(mp.Principal[1]*k), fmtSig(mp.Principal[2]*k)))
}

func round4(x float64) float64 { return math.Round(x*1e4) / 1e4 }

// fmtSig shows a value to 5 significant digits.
func fmtSig(x float64) string {
	if x == 0 {
		return "0"
	}
	d := 4 - int(math.Floor(math.Log10(math.Abs(x))))
	if d < 0 {
		d = 0
	}
	return fmtLen(math.Round(x*math.Pow(10, float64(d))) / math.Pow(10, float64(d)))
}

// --- section view ---

type sectionView struct {
	on     bool
	axis   int     // 0 X, 1 Y, 2 Z: the plane's normal
	offset float64 // along it
	flip   bool
}

func (s *sectionView) toggle(a *App, arg string) {
	if arg == "" {
		s.on = !s.on
		if s.on {
			c := a.doc.Bounds().Center()
			s.offset = [3]float64{c.X, c.Y, c.Z}[s.axis]
		}
		return
	}
	// "y:10", "off", "x:5:flip"
	if arg == "off" {
		s.on = false
		return
	}
	parts := strings.Split(arg, ":")
	s.on = true
	switch strings.ToLower(parts[0]) {
	case "x":
		s.axis = 0
	case "y":
		s.axis = 1
	case "z":
		s.axis = 2
	}
	if len(parts) > 1 {
		if v, err := a.doc.D.Eval(parts[1]); err == nil {
			s.offset = v
		}
	}
	s.flip = len(parts) > 2 && parts[2] == "flip"
}

// clip is the plane as the shaders take it (kept where n·p + w ≥ 0): the
// half on the camera's side is cut away (Flip keeps it instead), so the
// cut faces the camera.
func (s *sectionView) clip(eye geom.Vec3) [4]float32 {
	if !s.on {
		return [4]float32{0, 0, 0, 1}
	}
	var n [3]float32
	n[s.axis] = -1
	if [3]float64{eye.X, eye.Y, eye.Z}[s.axis] < s.offset {
		n[s.axis] = 1
	}
	if s.flip {
		n[s.axis] = -n[s.axis]
	}
	return [4]float32{n[0], n[1], n[2], -n[s.axis] * float32(s.offset)}
}

// panel is a small window over the view while the section is on.
func (s *sectionView) panel(a *App) {
	if !s.on {
		return
	}
	vp := &a.vp
	imgui.SetNextWindowPosV(imgui.Vec2{X: vp.pos.X + 10, Y: vp.pos.Y + 10}, imgui.CondAlways, imgui.Vec2{})
	imgui.SetNextWindowBgAlpha(0.85)
	flags := imgui.WindowFlagsNoDecoration | imgui.WindowFlagsAlwaysAutoResize | imgui.WindowFlagsNoSavedSettings | imgui.WindowFlagsNoFocusOnAppearing | imgui.WindowFlagsNoMove
	if imgui.BeginV("##section", nil, flags) {
		imgui.TextUnformatted(ui.Icon(icSection, "Section"))
		imgui.SameLine()
		for i, n := range []string{"X", "Y", "Z"} {
			if imgui.RadioButtonBool(n+"##sec", s.axis == i) {
				s.axis = i
			}
			imgui.SameLine()
		}
		imgui.Checkbox("Flip##sec", &s.flip)
		b := a.doc.Bounds()
		lo := [3]float64{b.Min.X, b.Min.Y, b.Min.Z}[s.axis]
		hi := [3]float64{b.Max.X, b.Max.Y, b.Max.Z}[s.axis]
		if hi-lo < 1 {
			lo, hi = lo-10, hi+10
		}
		off := float32(s.offset)
		imgui.SetNextItemWidth(220)
		if imgui.SliderFloatV("##secoff", &off, float32(lo), float32(hi), "%.2f mm", 0) {
			s.offset = float64(off)
		}
		imgui.SameLine()
		if imgui.SmallButton("Off##sec") {
			s.on = false
		}
	}
	imgui.End()
}

// --- the move gizmo ---

// moveGizmo shows ImGuizmo's arrows (or rings) on the selected move
// feature's bodies; dragging them sets its inputs.
func (a *App) moveGizmo(dl *imgui.DrawList) {
	vp := &a.vp
	vp.gizmoHot = false
	f := a.selFeature()
	if f == nil || (f.Kind != doc.KMove && (f.Kind != doc.KComponent || a.jointedBy(f.ID) != 0)) || a.sk != nil || !f.Active {
		vp.gizmoDrag = false
		return
	}
	x, err := a.doc.D.MoveXform(f)
	if err != nil {
		return
	}
	var pivot [3]float64
	for i, k := range []string{"Pivot X", "Pivot Y", "Pivot Z"} {
		pivot[i], _ = a.doc.D.Eval(f.Inputs[k])
	}
	pv := geom.V3(pivot[0], pivot[1], pivot[2])
	at := x.Pt(pv)
	c0, c1, c2 := x.R[0], x.R[1], x.R[2]
	m := mgl32.Mat4{
		float32(c0.X), float32(c0.Y), float32(c0.Z), 0,
		float32(c1.X), float32(c1.Y), float32(c1.Z), 0,
		float32(c2.X), float32(c2.Y), float32(c2.Z), 0,
		float32(at.X), float32(at.Y), float32(at.Z), 1,
	}
	before := m
	view, proj := vp.view.View, vp.view.Proj
	imguizmo.SetOrthographic(a.cam.Ortho)
	imguizmo.SetGizmoSizeClipSpace(0.17)
	imguizmo.SetDrawlistV(dl)
	imguizmo.SetRect(vp.pos.X, vp.pos.Y, vp.size.X, vp.size.Y)
	op := imguizmo.TRANSLATE
	if a.gizmoRotate {
		op = imguizmo.ROTATE
	}
	imguizmo.ManipulateV(&view[0], &proj[0], op, imguizmo.WORLD, &m[0], nil, nil, nil, nil)
	using := imguizmo.IsUsing()
	vp.gizmoHot = using || imguizmo.IsOver()
	if m != before {
		g := func(i int) float64 { return float64(m[i]) }
		r0 := geom.V3(g(0), g(1), g(2)).Norm()
		r1 := geom.V3(g(4), g(5), g(6)).Norm()
		r2 := geom.V3(g(8), g(9), g(10)).Norm()
		deg := 180 / math.Pi
		rx := math.Atan2(r1.Z, r2.Z) * deg
		ry := math.Asin(math.Max(-1, math.Min(1, -r0.Z))) * deg
		rz := math.Atan2(r0.Y, r0.X) * deg
		mv := geom.V3(g(12), g(13), g(14)).Sub(pv)
		put := func(k string, v float64) {
			v = math.Round(v*100) / 100
			if v == 0 {
				v = 0 // no "-0"
			}
			f.Inputs[k] = fmtLen(v)
		}
		put("Move X", mv.X)
		put("Move Y", mv.Y)
		put("Move Z", mv.Z)
		put("Rotate X", rx)
		put("Rotate Y", ry)
		put("Rotate Z", rz)
		a.doc.Regenerate()
	}
	if using {
		vp.gizmoDrag = true
	} else if vp.gizmoDrag {
		vp.gizmoDrag = false
		a.doc.Commit()
	}
}

// gizmoAt is where the move gizmo stands (the moved pivot), if it shows.
func (a *App) gizmoAt() (geom.Vec3, bool) {
	f := a.selFeature()
	if f == nil || f.Kind != doc.KMove {
		return geom.Vec3{}, false
	}
	x, err := a.doc.D.MoveXform(f)
	if err != nil {
		return geom.Vec3{}, false
	}
	var p [3]float64
	for i, k := range []string{"Pivot X", "Pivot Y", "Pivot Z"} {
		p[i], _ = a.doc.D.Eval(f.Inputs[k])
	}
	return x.Pt(geom.V3(p[0], p[1], p[2])), true
}

func (a *App) registerInspectCommands(c *Commands) {
	c.add(&Command{ID: "inspect.mass", Label: "Mass properties of the selected body", Group: "Inspect", Icon: 0xf5cd,
		Enabled: func() bool { return a.sk == nil && len(a.selectedBodies()) > 0 },
		Run: func(string) {
			if bs := a.selectedBodies(); len(bs) > 0 {
				if b := a.doc.Body(bs[0]); b != nil {
					a.massOf(b)
					a.sel = a.sel[:0]
				}
			}
		}})
}
