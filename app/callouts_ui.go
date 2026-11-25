package app

import (
	"sort"
	"strings"

	"forma/drawing"
	"forma/geom"
)

// Hole callouts (M20): the Hole dimension tool puts one on a picked rim;
// Callouts puts one on every hole step seen end on in the selected view —
// on its biggest rim (a counterbore's, a countersink's), its shoulder over
// the view, out on the side of the view the hole is on.

const (
	icCallout  = 0xf02b // tag
	icCallouts = 0xf02c // tags
)

func (a *App) addCallouts() {
	s := a.dwSheet()
	if s == nil || a.dw.selKind != "view" {
		a.notify("Callouts: select a view")
		return
	}
	a.dwRebuild()
	b := a.dw.res.Views[a.dw.selID]
	if b == nil || b.Err != "" {
		return
	}
	// those already called out in this view
	have := map[string]bool{}
	for _, d := range s.Dims {
		if d.View == b.View.ID && d.Kind == "hole" && len(d.Anchors) == 1 {
			have[holeOf(d.Anchors[0].A)] = true
			have[holeOf(d.Anchors[0].B)] = true
		}
	}
	// per hole step: its biggest rim seen end on, visible
	type pick struct {
		c   *drawing.Curve
		key string
	}
	best := map[string]pick{}
	var order []string
	for i := range b.Proj.Curves {
		c := &b.Proj.Curves[i]
		if c.Arc == nil || c.Edge == nil || c.Hidden || c.Thread {
			continue
		}
		key := ""
		for _, h := range c.Edge.HE {
			if h != nil && holeOf(h.Loop.Face.Name) != "" && a.doc.D.HoleCallout(h.Loop.Face.Name) != nil {
				key = holeOf(h.Loop.Face.Name)
			}
		}
		if key == "" || have[key] {
			continue
		}
		p, ok := best[key]
		if !ok {
			order = append(order, key)
		}
		if !ok || c.Arc.R > p.c.Arc.R+1e-9 {
			best[key] = pick{c: c, key: key}
		}
	}
	if len(order) == 0 {
		a.notify("Callouts: no hole steps seen end on in this view (or every one has its callout)")
		return
	}
	mid := b.Lo.Lerp(b.Hi, 0.5)
	// left to right, each a step higher than the last over the view
	sort.SliceStable(order, func(i, j int) bool { return best[order[i]].c.Arc.C.X < best[order[j]].c.Arc.C.X })
	for k, key := range order {
		c := best[key].c
		centre := c.Edge.Curve.(geom.Circle).F.O
		an := drawing.Anchor{Body: c.Body, A: c.Edge.HE[0].Loop.Face.Name, B: c.Edge.HE[1].Loop.Face.Name, P: centre.Add(c.Edge.Curve.(geom.Circle).F.X.Scale(c.Arc.R))}
		at := b.Sheet(c.Arc.C)
		// over the view, out to the side of its middle the hole is on
		side := 1.0
		if at.X < mid.X {
			side = -1
		}
		q := geom.V2(at.X+side*(c.Arc.R*b.Scale+10), b.Hi.Y+8+10*float64(k))
		d := &drawing.Dim{View: b.View.ID, Kind: "hole", Anchors: []drawing.Anchor{an}, Place: q.Sub(geom.V2(b.View.X, b.View.Y))}
		if _, err := d.Value(b); err != nil {
			continue
		}
		s.AddDim(d)
	}
	a.doc.Commit()
	a.notify("%d hole callouts", len(order))
}

// holeOf is the hole step a face name is from ("f12" of "f12/hole:1:wall"),
// "" if it isn't a hole's.
func holeOf(face string) string {
	i := strings.Index(face, "/hole:")
	if i < 0 {
		return ""
	}
	return face[:i]
}
