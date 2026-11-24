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
	c.add(&Command{ID: "assemble.screw", Label: "Screw: an ISO screw (in the hole whose rim is selected)", Group: "Assemble", Icon: icScrew,
		Enabled: func() bool { return a.sk == nil },
		Run: func(arg string) {
			// arg: "ISO 4762,M5,16" (else a socket head M5×16, or the size the
			// selected hole was made for)
			spec := doc.ScrewSpec{Std: "ISO 4762", Size: "M5", Length: 16}
			place, rim := a.selectedRim()
			if rim != nil {
				if size := a.holeSizeAt(rim); size != "" {
					spec.Size = size
				}
			}
			if arg != "" {
				p := strings.Split(arg, ",")
				if len(p) == 3 {
					l, err := strconv.ParseFloat(strings.TrimSpace(p[2]), 64)
					if err == nil {
						spec = doc.ScrewSpec{Std: strings.TrimSpace(p[0]), Size: strings.TrimSpace(p[1]), Length: l}
					}
				}
			}
			f, err := doc.NewScrew(spec)
			if err != nil {
				a.notify("Screw: %v", err)
				return
			}
			if rim != nil {
				// head on the rim's face, shank along the hole (into the part)
				o, n := place.O, place.Z
				rx := -math.Asin(math.Max(-1, math.Min(1, n.Y)))
				ry := math.Atan2(n.X, n.Z)
				f.Inputs["Move X"], f.Inputs["Move Y"], f.Inputs["Move Z"] = fnum(o.X), fnum(o.Y), fnum(o.Z)
				f.Inputs["Rotate X"], f.Inputs["Rotate Y"] = fnum(rx*180/math.Pi), fnum(ry*180/math.Pi)
			}
			a.doc.D.Insert(f)
			a.doc.Commit()
			a.sel = a.sel[:0]
			a.browserSel, a.multi = bodyKey(f.ID, 0), nil
			if f.Err != "" {
				a.notify("%s: %s", f.Name, f.Err)
			}
		}})
}

func fnum(x float64) string {
	if math.Abs(x) < 1e-9 {
		x = 0
	}
	return strconv.FormatFloat(math.Round(x*1e4)/1e4, 'f', -1, 64)
}

// selectedRim is a selected circular edge with a flat face beside it: its
// centre, and Z the flat face's outward normal (out of the part).
func (a *App) selectedRim() (geom.Frame, *brep.Edge) {
	for _, r := range a.sel {
		if r.Kind != KindEdge {
			continue
		}
		_, e := a.refEdge(r)
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
			n := f.OutwardNormal(u, v).Norm()
			return geom.Frame{O: c.F.O, Z: n}, e
		}
	}
	return geom.Frame{}, nil
}

// holeSizeAt is the screw size a hole step made the rim's hole for ("" if
// none): the rim's faces' step, by their names.
func (a *App) holeSizeAt(e *brep.Edge) string {
	for _, h := range e.HE {
		if h == nil {
			continue
		}
		name := h.Loop.Face.Name
		var id int
		if _, err := fmt.Sscanf(name, "f%d/", &id); err != nil {
			continue
		}
		if f := a.doc.D.Feature(id); f != nil && f.Kind == doc.KHole && f.Inputs["Screw"] != "" {
			return f.Inputs["Screw"]
		}
	}
	return ""
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
