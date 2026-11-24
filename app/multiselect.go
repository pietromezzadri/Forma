package app

// Selecting several things at once: Ctrl+click (or Shift) in the browser
// or the timeline picks more bodies, sketches and steps besides the first
// (a.browserSel), and Properties then edits what the steps share; a box
// dragged over the view picks the edges inside it (faces with Shift);
// double-clicking an edge picks its tangent chain, Ctrl+double-clicking a
// body adds all of it. Fillet and chamfer with whole bodies picked round
// all their sharp edges.

import (
	"fmt"
	"math"
	"sort"
	"strings"

	imgui "github.com/AllenDang/cimgui-go/imgui"

	"forma/brep"
	"forma/doc"
	"forma/ui"
)

// pickBrowser picks an item of the browser or a step of the timeline: with
// Ctrl or Shift held, as well as what is picked; else instead of it.
func (a *App) pickBrowser(id int) {
	io := imgui.CurrentIO()
	if !io.KeyCtrl() && !io.KeyShift() {
		a.browserSel, a.multi = id, nil
		return
	}
	if a.browserSel == 0 {
		a.browserSel = id
		return
	}
	if id == a.browserSel {
		// let the first go: the next takes its place
		if len(a.multi) > 0 {
			a.browserSel, a.multi = a.multi[0], a.multi[1:]
		} else {
			a.browserSel = 0
		}
		return
	}
	for i, m := range a.multi {
		if m == id {
			a.multi = append(a.multi[:i], a.multi[i+1:]...)
			return
		}
	}
	a.multi = append(a.multi, id)
}

// pickedIDs are the browser picks: the first and the others.
func (a *App) pickedIDs() []int {
	var out []int
	seen := map[int]bool{}
	for _, id := range append([]int{a.browserSel}, a.multi...) {
		if id != 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// browserPicked: id is among the browser picks.
func (a *App) browserPicked(id int) bool {
	for _, p := range a.pickedIDs() {
		if p == id {
			return true
		}
	}
	return false
}

// stepPicked: a pick belongs to step fid.
func (a *App) stepPicked(fid int) bool {
	for _, p := range a.pickedIDs() {
		if featureOf(p) == fid {
			return true
		}
	}
	return false
}

// pickedFeatures are the steps picked, in the design's order.
func (a *App) pickedFeatures() []*doc.Feature {
	var out []*doc.Feature
	for _, f := range a.doc.D.Features {
		if a.stepPicked(f.ID) {
			out = append(out, f)
		}
	}
	return out
}

// pickedBodies are the bodies picked in the browser (not through faces).
func (a *App) pickedBodies() []*BodyItem {
	var out []*BodyItem
	for _, id := range a.pickedIDs() {
		if b := a.doc.Body(id); b != nil {
			out = append(out, b)
		}
	}
	return out
}

// sharp: a round or chamfer can go on e (brep.Sharp).
func sharp(e *brep.Edge) bool { return brep.Sharp(e) }

// --- box selection ---

// boxSelect picks what the box from p to q (UI units in the view) shows:
// the edges in it, or with Shift its faces.
func (a *App) boxSelect(p, q imgui.Vec2, add, faces bool) {
	vp := &a.vp
	x0, y0 := int(math.Min(float64(p.X), float64(q.X))*float64(vp.scale)), int(math.Min(float64(p.Y), float64(q.Y))*float64(vp.scale))
	x1, y1 := int(math.Max(float64(p.X), float64(q.X))*float64(vp.scale)), int(math.Max(float64(p.Y), float64(q.Y))*float64(vp.scale))
	ids := a.r.Pick.ReadRect(x0, y0, x1, y1)
	want := KindEdge
	if faces {
		want = KindFace
	}
	var got []Ref
	for _, id := range ids {
		k, slot, local := decodeID(id)
		if k != want {
			continue
		}
		r := Ref{Kind: k, Item: vp.slots.item(slot), Local: local}
		if a.doc.Body(r.Item) == nil {
			continue
		}
		if k == KindEdge {
			if _, e := a.refEdge(r); e == nil || !sharp(e) {
				continue // seams and tangent edges: nothing to round
			}
		}
		got = append(got, r)
	}
	if !add {
		a.sel = a.sel[:0]
	}
	for _, r := range got {
		if !a.isSelected(r) {
			a.sel = append(a.sel, r)
		}
	}
	what := "edges"
	if faces {
		what = "faces"
	}
	a.notify("%d %s picked", len(got), what)
}

// drawBox draws the selection box while it is dragged.
func (a *App) drawBox(dl *imgui.DrawList) {
	vp := &a.vp
	if !vp.boxOn || !vp.leftHeld {
		return
	}
	p := imgui.Vec2{X: vp.pos.X + vp.boxFrom.X, Y: vp.pos.Y + vp.boxFrom.Y}
	q := imgui.Vec2{X: vp.pos.X + vp.mouse.X, Y: vp.pos.Y + vp.mouse.Y}
	mn := imgui.Vec2{X: float32(math.Min(float64(p.X), float64(q.X))), Y: float32(math.Min(float64(p.Y), float64(q.Y)))}
	mx := imgui.Vec2{X: float32(math.Max(float64(p.X), float64(q.X))), Y: float32(math.Max(float64(p.Y), float64(q.Y)))}
	dl.AddRectFilled(mn, mx, rgba(0.25, 0.55, 0.95, 0.12))
	dl.AddRectV(mn, mx, rgba(0.25, 0.55, 0.95, 0.9), 0, 1.2, 0)
}

// --- chains ---

// tangentChain is edge r and the edges running on from it smoothly at
// either end (the rim round a rounded slot, along a filleted plate).
func (a *App) tangentChain(r Ref) []Ref {
	b, e0 := a.refEdge(r)
	if e0 == nil {
		return []Ref{r}
	}
	var out []Ref
	for _, e := range brep.TangentChain(b.Body, e0) {
		out = append(out, Ref{Kind: KindEdge, Item: r.Item, Local: e.ID})
	}
	return out
}

// --- several steps in Properties ---

// multiProperties edits the steps picked together: their shared values
// (each set on all of them), hiding, suppressing, deleting.
func (a *App) multiProperties(fs []*doc.Feature) {
	kinds := map[string]bool{}
	for _, f := range fs {
		kinds[f.Kind] = true
	}
	title := fmt.Sprintf("%d steps", len(fs))
	if len(kinds) == 1 {
		title = fmt.Sprintf("%d × %s", len(fs), doc.KindLabel(fs[0].Kind))
	}
	imgui.SeparatorText(title)
	var names []string
	for _, f := range fs {
		names = append(names, f.Name)
	}
	imgui.TextWrapped(strings.Join(names, ", "))
	// the values every one of them has, numbers or expressions
	var keys []string
	for k := range fs[0].Inputs {
		common := true
		for _, f := range fs[1:] {
			if _, ok := f.Inputs[k]; !ok {
				common = false
			}
		}
		for _, f := range fs {
			if _, err := a.doc.D.Eval(f.Inputs[k]); err != nil {
				common = false
			}
		}
		if common {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		imgui.SeparatorText("Shared values")
	} else {
		imgui.TextDisabled("They have no values in common to set together.")
	}
	for _, k := range keys {
		same := true
		for _, f := range fs[1:] {
			same = same && f.Inputs[k] == fs[0].Inputs[k]
		}
		v := ""
		if same {
			v = fs[0].Inputs[k]
		}
		hint := "various: type one for all"
		imgui.SetNextItemWidth(130)
		imgui.InputTextWithHint(k+"##multi", hint, &v, imgui.InputTextFlagsAutoSelectAll, nil)
		a.markItemLeft("prop:" + k)
		if imgui.IsItemDeactivatedAfterEdit() && strings.TrimSpace(v) != "" {
			if _, err := a.doc.D.Eval(v); err != nil {
				a.notify("%s: %v", k, err)
			} else {
				for _, f := range fs {
					f.Inputs[k] = v
				}
				a.doc.Commit()
			}
		}
		if !same {
			imgui.SameLine()
			imgui.TextDisabled("(various)")
		}
	}
	imgui.Spacing()
	hidden := true
	off := true
	for _, f := range fs {
		hidden = hidden && f.Hidden
		off = off && f.Off
	}
	if imgui.Button(map[bool]string{true: "Show all", false: "Hide all"}[hidden]) {
		for _, f := range fs {
			f.Hidden = !hidden
		}
		a.doc.Commit()
	}
	imgui.SameLine()
	if imgui.Button(map[bool]string{true: "Unsuppress all", false: "Suppress all"}[off]) {
		for _, f := range fs {
			f.Off = !off
		}
		a.doc.Commit()
	}
	imgui.SameLine()
	if imgui.Button(ui.Icon(ui.FaTrash, "Delete all")) {
		for _, f := range fs {
			a.doc.D.Remove(f.ID)
		}
		a.browserSel, a.multi = 0, nil
		a.doc.Commit()
	}
}

const icXRay = 0xf5fd // layers: see-through

// anyXRay: some part is see-through.
func (a *App) anyXRay() bool {
	for _, b := range a.doc.Bodies {
		if b.F.XRay {
			return true
		}
	}
	return false
}

// xrayLit: the selected parts are all see-through (nothing selected: any is).
func (a *App) xrayLit() bool {
	keys := a.selectedBodies()
	if len(keys) == 0 {
		return a.anyXRay()
	}
	for _, k := range keys {
		if b := a.doc.Body(k); b == nil || !b.F.XRay {
			return false
		}
	}
	return len(keys) > 0
}

const icCombine = 0xf248 // object group
