package app

import (
	"fmt"
	"strconv"
	"strings"

	imgui "github.com/AllenDang/cimgui-go/imgui"

	"forma/brep"
	"forma/doc"
)

// Edge sets in a fillet's or chamfer's properties: edges with a size of
// their own. An edge already rounded is gone from the model, so it is picked
// by its round (a face named f<ID>/round:<k>, k its number in the step);
// sharp edges picked in the view are added to the step.

// selectedRounds are the step's edges whose rounds (or chamfer faces) are
// selected, by index.
func (a *App) selectedRounds(f *doc.Feature) []int {
	word := "round"
	if f.Kind == doc.KChamfer {
		word = "chamfer"
	}
	prefix := fmt.Sprintf("f%d/%s:", f.ID, word)
	seen := map[int]bool{}
	var out []int
	for _, r := range a.sel {
		if r.Kind != KindFace {
			continue
		}
		_, fc := a.refFace(r)
		if fc == nil || !strings.HasPrefix(fc.Name, prefix) {
			continue
		}
		num := fc.Name[len(prefix):]
		if i := strings.IndexFunc(num, func(c rune) bool { return c < '0' || c > '9' }); i >= 0 {
			num = num[:i]
		}
		k, err := strconv.Atoi(num)
		if err != nil || k < 1 || k > len(f.Edges) || seen[k-1] {
			continue
		}
		seen[k-1] = true
		out = append(out, k-1)
	}
	return out
}

// selectedSharpEdges are the sharp edges picked themselves (not a face's).
func (a *App) selectedSharpEdges() []doc.EdgeRef {
	var out []doc.EdgeRef
	seen := map[*brep.Edge]bool{}
	for _, r := range a.sel {
		if r.Kind != KindEdge {
			continue
		}
		_, e := a.refEdge(r)
		if e == nil || seen[e] || !brep.Sharp(e) {
			continue
		}
		seen[e] = true
		if ref, ok := doc.RefForEdge(e, e.Curve.Eval((e.T0+e.T1)/2)); ok {
			out = append(out, ref)
		}
	}
	return out
}

// giveOwnSize puts the selected rounds and sharp edges in a new set of
// step f with size expr (the step's own size when empty).
func (a *App) giveOwnSize(f *doc.Feature, expr string) bool {
	idx, add := a.selectedRounds(f), a.selectedSharpEdges()
	if len(idx)+len(add) == 0 {
		return false
	}
	if expr == "" {
		expr = f.Inputs[doc.SizeKey(f, 0)]
	}
	doc.SetEdgeSize(f, idx, add, expr)
	a.sel = a.sel[:0]
	a.doc.Commit()
	return true
}

// edgeSetsUI draws a fillet's or chamfer's edge sets under its own size.
func (a *App) edgeSetsUI(f *doc.Feature) {
	word := "radius"
	if f.Kind == doc.KChamfer {
		word = "distance"
	}
	for _, set := range doc.EdgeSets(f) {
		n := 0
		for _, r := range f.Edges {
			if r.Set == set {
				n++
			}
		}
		imgui.SeparatorText(fmt.Sprintf("%d %s with a %s of %s own", n, plural(n, "edge", "edges"), word, plural(n, "its", "their")))
		a.exprField(f, doc.SizeKey(f, set), " mm")
		if imgui.SmallButton(fmt.Sprintf("Back to the step's %s##set%d", word, set)) {
			doc.ClearEdgeSet(f, set)
			a.doc.Commit()
		}
	}
	if n := len(a.selectedRounds(f)) + len(a.selectedSharpEdges()); n > 0 {
		imgui.Spacing()
		if imgui.SmallButton(fmt.Sprintf("Give the %d selected a %s of their own", n, word)) {
			a.giveOwnSize(f, "")
		}
		a.markItemLeft("btn:fillet.ownsize")
	} else if len(f.Edges) > 1 {
		imgui.TextDisabled(fmt.Sprintf("For a %s of their own, select rounds\n(or sharp edges to add) and come back here.", word))
	}
}
