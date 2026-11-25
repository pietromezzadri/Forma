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

// Screws, washers and nuts (M20): the Screw, Washer and Nut commands put an
// ISO part in the design as a component on each selected hole rim, jointed
// there onto whatever is already stacked on it (a washer under a screw's
// head, a washer and a nut where the screw comes out); their properties
// pick the standard and size. A hole's properties size it for a screw.

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
			a.placeFasteners("screw", func(size string) (*doc.Feature, error) {
				spec := doc.ScrewSpec{Std: "ISO 4762", Size: size, Length: 16}
				if given != nil {
					spec = *given
				}
				return doc.NewScrew(spec)
			})
		}})
	for _, k := range []struct{ kind, label, std string }{
		{"washer", "Washer: an ISO washer (on each hole rim selected, under a screw's head or a nut)", "ISO 7089"},
		{"nut", "Nut: an ISO hex nut (on each hole rim selected, where the screw comes out)", "ISO 4032"},
	} {
		k := k
		c.add(&Command{ID: "assemble." + k.kind, Label: k.label, Group: "Assemble", Icon: map[string]rune{"washer": icWasher, "nut": icNut}[k.kind],
			Enabled: func() bool { return a.sk == nil },
			Run: func(arg string) {
				// arg: "ISO 7089,M5" (else the default standard at the hole's size)
				std, size := k.std, ""
				if p := strings.Split(arg, ","); len(p) == 2 {
					std, size = strings.TrimSpace(p[0]), strings.TrimSpace(p[1])
				}
				a.placeFasteners(k.kind, func(hole string) (*doc.Feature, error) {
					if size != "" {
						hole = size
					}
					return doc.NewFastener(doc.PartSpec{Kind: k.kind, Std: std, Size: hole})
				})
			}})
	}
}

const (
	icWasher = 0xf192 // dot-circle
	icNut    = 0xf013 // gear
)

// placeFasteners puts a screw, washer or nut (made by make, given the size
// the hole was made for, M5 if none) on each selected hole rim, jointed
// onto what is already stacked there; with no rim selected, one at the
// origin.
func (a *App) placeFasteners(kind string, make func(size string) (*doc.Feature, error)) {
	rims := a.selectedRims()
	if len(rims) == 0 {
		rims = []rimSel{{}}
	}
	var placed, joints []*doc.Feature
	for _, rim := range rims {
		size := "M5"
		if rim.edge != nil {
			if s := a.doc.D.HoleScrew(rim.edge); s != "" {
				size = s
			}
		}
		f, err := make(size)
		if err != nil {
			a.notify("%s: %v", strings.ToUpper(kind[:1])+kind[1:], err)
			return
		}
		if rim.edge != nil {
			// on the rim's face, along the hole (into the part), where it
			// stays if the joint can't be made
			o, n := rim.place.O, rim.place.Z
			rx := -math.Asin(math.Max(-1, math.Min(1, n.Y)))
			ry := math.Atan2(n.X, n.Z)
			f.Inputs["Move X"], f.Inputs["Move Y"], f.Inputs["Move Z"] = fnum(o.X), fnum(o.Y), fnum(o.Z)
			f.Inputs["Rotate X"], f.Inputs["Rotate Y"] = fnum(rx*180/math.Pi), fnum(ry*180/math.Pi)
		}
		a.doc.D.Insert(f)
		if rim.edge == nil {
			placed = append(placed, f)
			continue
		}
		// jointed into the hole (or onto what is on it), so it follows it
		j, err := a.doc.D.FastenerInHole(f, rim.key, rim.edge)
		if err != nil {
			a.notify("%s: %v", f.Name, err)
			a.doc.D.Remove(f.ID)
			continue
		}
		a.doc.D.Insert(j)
		placed = append(placed, f)
		joints = append(joints, j)
	}
	a.doc.Commit()
	if len(placed) == 0 {
		return
	}
	for _, j := range joints {
		if j.Err != "" { // it stays where it was put
			a.notify("%s: %s", j.Name, j.Err)
			a.doc.Remove(j.ID)
		}
	}
	last := placed[len(placed)-1]
	a.sel = a.sel[:0]
	a.browserSel, a.multi = bodyKey(last.ID, 0), nil
	switch {
	case last.Err != "":
		a.notify("%s: %s", last.Name, last.Err)
	case len(placed) > 1:
		a.notify("%d %ss put on their holes", len(placed), kind)
	}
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
	a.threadChoice(f)
	imgui.TextDisabled("Length in mm.")
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
	if fit == "Tapped" {
		a.threadChoice(f)
	}
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

// fastenerProperties draws a washer's or nut's standard and size.
func (a *App) fastenerProperties(f *doc.Feature) {
	p, ok := doc.FastenerOf(f)
	if !ok {
		return
	}
	imgui.SeparatorText(map[string]string{"washer": "Washer", "nut": "Nut"}[p.Kind])
	ids, labels := p.Stds()
	var opts []string
	cur := ""
	for i, id := range ids {
		opts = append(opts, id+" "+labels[i])
		if id == p.Std {
			cur = opts[i]
		}
	}
	input := map[string]string{"washer": "Washer", "nut": "Nut"}[p.Kind]
	apply := func(n doc.PartSpec) {
		f.Inputs[input], f.Inputs["Size"] = n.Std, n.Size
		f.Name = n.Label()
		a.doc.Commit()
		if f.Err != "" {
			a.notify("%s: %s", f.Name, f.Err)
		}
	}
	if m, ok := a.combo("Standard", cur, opts, nil); ok {
		for i, o := range opts {
			if o == m {
				n := doc.PartSpec{Kind: p.Kind, Std: ids[i], Size: p.Size}
				if sizes := n.Sizes(); !contains(sizes, n.Size) {
					n.Size = sizes[len(sizes)/2]
				}
				apply(n)
			}
		}
	}
	a.markItemLeft("prop:Standard")
	if m, ok := a.combo("Size", p.Size, p.Sizes(), nil); ok {
		apply(doc.PartSpec{Kind: p.Kind, Std: p.Std, Size: m})
	}
	a.markItemLeft("prop:Size")
	if p.Kind == "nut" {
		a.threadChoice(f)
	}
}

// threadChoice: a screw's, nut's or tapped hole's thread drawn (thin lines
// on drawings, a plain shank or bore) or modelled (a helical solid).
func (a *App) threadChoice(f *doc.Feature) {
	cur := "Cosmetic"
	if f.Inputs["Thread"] == "Modelled" {
		cur = "Modelled"
	}
	if m, ok := a.combo("Thread", cur, []string{"Cosmetic", "Modelled"}, nil); ok && m != cur {
		if m == "Modelled" {
			f.Inputs["Thread"] = "Modelled"
		} else {
			delete(f.Inputs, "Thread")
		}
		a.doc.Commit()
		if f.Err != "" {
			a.notify("%s: %s", f.Name, f.Err)
		}
	}
	a.markItemLeft("prop:Thread")
	imgui.SetItemTooltip("Cosmetic: a plain shank or bore, the thread drawn on drawings.\nModelled: the thread as a solid (for 3D printing); slower, thousands of facets.")
}
