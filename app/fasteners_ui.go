package app

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	imgui "github.com/AllenDang/cimgui-go/imgui"

	"forma/brep"
	"forma/doc"
	"forma/geom"
)

// Screws (M20): the Screw command puts an ISO screw in the design as a
// component — in the hole whose rim is selected, its head on the rim's
// face and its shank down the hole — and a screw's properties pick its
// standard, size and length. A hole's properties size it for a screw.

const icScrew = 0xf54a // screwdriver

func (a *App) registerFastenerCommands(c *Commands) {
	c.add(&Command{ID: "assemble.screw", Label: "Screw: an ISO screw (in each hole whose rim is selected)", Group: "Assemble", Icon: icScrew,
		Enabled: func() bool { return a.sk == nil },
		Run: func(arg string) {
			// arg: "ISO 4762,M5,16" (else a socket head M5×16, or the size the
			// selected hole was made for)
			var given *doc.ScrewSpec
			if p := strings.Split(arg, ","); len(p) == 3 {
				if l, err := strconv.ParseFloat(strings.TrimSpace(p[2]), 64); err == nil {
					given = &doc.ScrewSpec{Std: strings.TrimSpace(p[0]), Size: strings.TrimSpace(p[1]), Length: l}
				}
			}
			rims := a.selectedRims()
			if len(rims) == 0 {
				rims = []rimSel{{}} // one at the origin
			}
			var screws, joints []*doc.Feature
			for _, rim := range rims {
				spec := doc.ScrewSpec{Std: "ISO 4762", Size: "M5", Length: 16}
				if rim.edge != nil {
					if size := a.doc.D.HoleScrew(rim.edge); size != "" {
						spec.Size = size
					}
				}
				if given != nil {
					spec = *given
				}
				f, err := doc.NewScrew(spec)
				if err != nil {
					a.notify("Screw: %v", err)
					return
				}
				if rim.edge != nil {
					// head on the rim's face, shank along the hole (into the
					// part), where it stays if the joint can't be made
					o, n := rim.place.O, rim.place.Z
					rx := -math.Asin(math.Max(-1, math.Min(1, n.Y)))
					ry := math.Atan2(n.X, n.Z)
					f.Inputs["Move X"], f.Inputs["Move Y"], f.Inputs["Move Z"] = fnum(o.X), fnum(o.Y), fnum(o.Z)
					f.Inputs["Rotate X"], f.Inputs["Rotate Y"] = fnum(rx*180/math.Pi), fnum(ry*180/math.Pi)
				}
				a.doc.D.Insert(f)
				screws = append(screws, f)
				if rim.edge == nil {
					continue
				}
				// jointed into the hole, so it follows the hole
				j, err := a.doc.D.ScrewInHole(f, rim.key, rim.edge)
				if err != nil {
					a.notify("%s isn't jointed: %v", f.Name, err)
					continue
				}
				a.doc.D.Insert(j)
				joints = append(joints, j)
			}
			a.doc.Commit()
			for _, j := range joints {
				if j.Err != "" { // the screw stays where it was put
					a.notify("%s: %s", j.Name, j.Err)
					a.doc.Remove(j.ID)
				}
			}
			last := screws[len(screws)-1]
			a.sel = a.sel[:0]
			a.browserSel, a.multi = bodyKey(last.ID, 0), nil
			switch {
			case last.Err != "":
				a.notify("%s: %s", last.Name, last.Err)
			case len(screws) > 1:
				a.notify("%d screws put in their holes", len(screws))
			}
		}})
}

func fnum(x float64) string {
	if math.Abs(x) < 1e-9 {
		x = 0
	}
	return strconv.FormatFloat(math.Round(x*1e4)/1e4, 'f', -1, 64)
}

// rimSel is a selected hole rim: a circular edge with a flat face beside
// it, on body key; place is its centre with Z the flat face's outward
// normal (out of the part).
type rimSel struct {
	key   int
	edge  *brep.Edge
	place geom.Frame
}

// selectedRims are the selected circular edges with a flat face beside them.
func (a *App) selectedRims() []rimSel {
	var out []rimSel
	for _, r := range a.sel {
		if r.Kind != KindEdge {
			continue
		}
		b, e := a.refEdge(r)
		if e == nil {
			continue
		}
		c, ok := e.Curve.(geom.Circle)
		if !ok {
			continue
		}
		for _, h := range e.HE {
			if h == nil {
				continue
			}
			f := h.Loop.Face
			if _, ok := f.Surf.(geom.Plane); !ok {
				continue
			}
			u, v := f.Surf.Project(c.F.O)
			out = append(out, rimSel{key: b.ID, edge: e, place: geom.Frame{O: c.F.O, Z: f.OutwardNormal(u, v).Norm()}})
			break
		}
	}
	return out
}

// screwProperties draws a screw component's standard, size and length.
func (a *App) screwProperties(f *doc.Feature) {
	spec, ok := doc.ScrewOf(f)
	if !ok {
		return
	}
	imgui.SeparatorText("Screw")
	var labels []string
	cur := ""
	for _, s := range doc.ScrewStds {
		l := s.ID + " " + s.Label
		labels = append(labels, l)
		if s.ID == spec.Std {
			cur = l
		}
	}
	std := doc.ScrewStdByID(spec.Std)
	set := func(k, v string) {
		f.Inputs[k] = v
		a.doc.Commit()
		if f.Err != "" {
			a.notify("%s: %s", f.Name, f.Err)
		}
	}
	if m, ok := a.combo("Standard", cur, labels, nil); ok {
		id := strings.SplitN(m, " ", 3)
		ns := doc.ScrewStdByID(id[0] + " " + id[1])
		if ns != nil {
			f.Inputs["Screw"] = ns.ID
			// keep the size and length where the new standard has them
			sizes := ns.Sizes()
			if !contains(sizes, spec.Size) {
				f.Inputs["Size"] = sizes[len(sizes)/2]
			}
			ls := ns.Lengths(f.Inputs["Size"])
			if !contains(doc.SortedLengths(ls), f.Inputs["Length"]) {
				f.Inputs["Length"] = fnum(nearest(ls, spec.Length))
			}
			f.Name = doc.ScrewSpec{Std: ns.ID, Size: f.Inputs["Size"], Length: mustFloat(f.Inputs["Length"])}.Label()
			set("Screw", ns.ID)
		}
	}
	a.markItemLeft("prop:Standard")
	if std == nil {
		return
	}
	if m, ok := a.combo("Size", spec.Size, std.Sizes(), nil); ok {
		f.Inputs["Size"] = m
		ls := std.Lengths(m)
		if !contains(doc.SortedLengths(ls), f.Inputs["Length"]) {
			f.Inputs["Length"] = fnum(nearest(ls, spec.Length))
		}
		f.Name = doc.ScrewSpec{Std: std.ID, Size: m, Length: mustFloat(f.Inputs["Length"])}.Label()
		set("Size", m)
	}
	a.markItemLeft("prop:Size")
	if m, ok := a.combo("Length", f.Inputs["Length"], doc.SortedLengths(std.Lengths(spec.Size)), nil); ok {
		f.Name = doc.ScrewSpec{Std: std.ID, Size: spec.Size, Length: mustFloat(m)}.Label()
		set("Length", m)
	}
	a.markItemLeft("prop:Length")
	imgui.TextDisabled("Length in mm; the thread isn't modelled.")
}

// holeScrewProperties: a hole made for a screw — the size and fit, and the
// diameters the tables give. It reports whether the hole is one (its own
// diameter fields then aren't shown).
func (a *App) holeScrewProperties(f *doc.Feature) bool {
	set := func(k, v string) {
		f.Inputs[k] = v
		a.doc.Commit()
	}
	opts := append([]string{"None (sizes below)"}, doc.ScrewSizes...)
	cur := f.Inputs["Screw"]
	if cur == "" {
		cur = opts[0]
	}
	if m, ok := a.combo("For a screw", cur, opts, nil); ok {
		if m == opts[0] {
			m = ""
		}
		if f.Inputs["Fit"] == "" {
			f.Inputs["Fit"] = "Normal"
		}
		set("Screw", m)
	}
	a.markItemLeft("prop:For a screw")
	if f.Inputs["Screw"] == "" {
		return false
	}
	fit := f.Inputs["Fit"]
	if fit == "" {
		fit = "Normal"
	}
	if m, ok := a.combo("Fit", fit, doc.HoleFits, nil); ok {
		set("Fit", m)
	}
	a.markItemLeft("prop:Fit")
	dia, bore, boreDepth, sink, err := doc.ScrewHoleSizes(f.Inputs["Screw"], fit)
	if err != nil {
		imgui.TextColored(colErr, err.Error())
		return true
	}
	what := "clearance (ISO 273)"
	if fit == "Tapped" {
		what = "tap drill"
	}
	imgui.TextDisabled(fmt.Sprintf("Hole Ø%s mm, %s", fnum(dia), what))
	switch f.Inputs["Type"] {
	case "Counterbore":
		imgui.TextDisabled(fmt.Sprintf("Counterbore Ø%s × %s mm (a socket head's)", fnum(bore), fnum(boreDepth)))
	case "Countersink":
		imgui.TextDisabled(fmt.Sprintf("Countersink Ø%s mm, 90° (a countersunk head's)", fnum(sink)))
	}
	return true
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func nearest(ls []float64, l float64) float64 {
	best := ls[0]
	for _, x := range ls {
		if math.Abs(x-l) < math.Abs(best-l) {
			best = x
		}
	}
	return best
}

func mustFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
