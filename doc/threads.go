package doc

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"forma/brep"
	"forma/drawing"
	"forma/geom"
)

// Threads and hole callouts on drawings (M20). The threads aren't modelled:
// a tapped hole is drilled at the tap size and a screw's shank is a plain
// cylinder at its nominal diameter, so a drawing shows them the way ISO
// 6410 draws threads, with thin lines — a tapped hole's at its major
// (nominal) diameter, a screw's at its minor — worked out here from the
// faces the hole step and the screw made. A hole step's callout gives its
// size, depth and head recess the way a drawing states them.

// threadsOf are a body's threads: its tapped holes' and, a screw's, its
// shank's.
func (d *Document) threadsOf(r *BodyRef) []drawing.Thread {
	var out []drawing.Thread
	add := func(t drawing.Thread) {
		// a face split in two gives one thread: the same axis and radius
		// run together
		z := t.B.Sub(t.A).Norm()
		for i := range out {
			o := &out[i]
			oz := o.B.Sub(o.A).Norm()
			if math.Abs(o.R-t.R) > 1e-6 || o.External != t.External || oz.Cross(z).Len() > 1e-9 || t.A.Sub(o.A).Cross(oz).Len() > 1e-6 {
				continue
			}
			lo, hi := 0.0, o.B.Sub(o.A).Dot(oz)
			ta, tb := t.A.Sub(o.A).Dot(oz), t.B.Sub(o.A).Dot(oz)
			a, b := math.Min(lo, math.Min(ta, tb)), math.Max(hi, math.Max(ta, tb))
			o.A, o.B = o.A.Add(oz.Scale(a)), o.A.Add(oz.Scale(b))
			return
		}
		out = append(out, t)
	}
	// a screw: its shank
	var screw *ScrewSpec
	if r.Comp != 0 {
		if s, ok := ScrewOf(d.Feature(r.Comp)); ok {
			screw = &s
		}
	}
	// modelled threads: their thin lines and outlines
	out = append(out, d.modelledThreads(r)...)
	if screw != nil && screw.Modelled {
		screw = nil // its plain shank isn't threaded
	}
	for _, f := range r.Body.Faces() {
		cy, ok := f.Surf.(geom.Cylinder)
		if !ok {
			continue
		}
		t0, t1 := faceSpan(f, cy.F)
		if t1-t0 < geom.Tol {
			continue
		}
		a, b := cy.F.O.Add(cy.F.Z.Scale(t0)), cy.F.O.Add(cy.F.Z.Scale(t1))
		// a tapped hole: the drill's wall, its thread at the nominal size
		if step := d.holeStepOfFace(f.Name); step != nil && step.Inputs["Fit"] == "Tapped" && !ModelledThread(step) {
			if m, ok := metrics[step.Inputs["Screw"]]; ok {
				dia, _, _, _, err := ScrewHoleSizes(step.Inputs["Screw"], "Tapped")
				if err == nil && math.Abs(cy.R-dia/2) < 1e-6 {
					add(drawing.Thread{A: a, B: b, R: m.d / 2})
				}
			}
			continue
		}
		if screw == nil {
			continue
		}
		m := metrics[screw.Size]
		if math.Abs(cy.R-m.d/2) > 1e-6 {
			continue
		}
		// the shank: threaded from its tip (the end furthest from the
		// screw's head) up b = 2d + 12 for socket and countersunk heads,
		// all the way for button and hex heads; drawn at the minor diameter
		tip := r.Place.Pt(geom.V3(0, 0, -screw.Length))
		if a.Dist(tip) < b.Dist(tip) {
			a, b = b, a
		}
		t := drawing.Thread{A: a, B: b, R: (m.d - 1.22687*m.pitch) / 2, External: true, Face: f.Name}
		if screw.Std == "ISO 4762" || screw.Std == "ISO 10642" {
			if l := 2*m.d + 12; l < a.Dist(b)-1e-6 {
				t.A = b.Add(a.Sub(b).Norm().Scale(l))
				t.Limit = m.d / 2
			}
		}
		add(t)
	}
	return out
}

// faceSpan is how far along frame fr's Z a face reaches (its edges).
func faceSpan(f *brep.Face, fr geom.Frame) (float64, float64) {
	t0, t1 := math.Inf(1), math.Inf(-1)
	for _, l := range f.Loops {
		for _, h := range l.HalfEdges() {
			e := h.Edge
			for _, t := range geom.SampleParams(e.Curve, e.T0, e.T1, 0.05, 1e9) {
				z := e.Curve.Eval(t).Sub(fr.O).Dot(fr.Z)
				t0, t1 = math.Min(t0, z), math.Max(t1, z)
			}
		}
	}
	return t0, t1
}

// holeStepOfFace is the hole step that made a face of this design (by its
// name, "f<step>/hole:…"), nil for any other face.
func (d *Document) holeStepOfFace(name string) *Feature {
	if !strings.Contains(name, "/hole:") && !drawing.ThreadFacet(name) {
		return nil
	}
	if f := d.Feature(nameFeature(name)); f != nil && f.Kind == KHole {
		return f
	}
	return nil
}

// HoleCallout is the callout of the hole a face belongs to: how many, the
// drill (and the thread of a tapped hole), how deep, and a counterbore or
// countersink, a line each; nil if no hole step made the face.
func (d *Document) HoleCallout(face string) []string {
	f := d.holeStepOfFace(face)
	if f == nil {
		return nil
	}
	spec, err := d.holeSpec(f)
	if err != nil {
		return nil
	}
	depth := " THRU"
	if f.Inputs["Extent"] != "Through all" {
		v, err := d.input(f, "Depth")
		if err != nil {
			return nil
		}
		depth = " {depth}" + callNum(v)
	}
	count := ""
	if n := len(f.Points); n > 1 {
		count = fmt.Sprintf("%d× ", n)
	}
	lines := []string{count + "Ø" + callNum(spec.Diameter) + depth}
	if size := f.Inputs["Screw"]; size != "" && f.Inputs["Fit"] == "Tapped" {
		if m, ok := metrics[size]; ok {
			lines = append(lines, size+"×"+callNum(m.pitch)+depth)
		}
	}
	switch f.Inputs["Type"] {
	case "Counterbore":
		lines = append(lines, "{cbore} Ø"+callNum(spec.BoreDiameter)+" {depth}"+callNum(spec.BoreDepth))
	case "Countersink":
		lines = append(lines, "{csink} Ø"+callNum(spec.SinkDiameter)+" × 90°")
	}
	return lines
}

// callNum is a size in a callout: two decimals at most, trailing zeros off.
func callNum(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

// modelledThreads are a body's modelled threads as a drawing shows them:
// a thin line where the cosmetic thread's would be and a wide outline
// where the plain hole's or shank's edges would be (the thread's facets'
// edges aren't drawn).
func (d *Document) modelledThreads(r *BodyRef) []drawing.Thread {
	var out []drawing.Thread
	facet := ""
	for _, f := range r.Body.Faces() {
		if drawing.ThreadFacet(f.Name) {
			facet = f.Name
			break
		}
	}
	if facet == "" {
		return nil
	}
	if c := d.Feature(r.Comp); c != nil {
		switch {
		case func() bool { s, ok := ScrewOf(c); return ok && s.Modelled }():
			s, _ := ScrewOf(c)
			m := metrics[s.Size]
			tip := r.Place.Pt(geom.V3(0, 0, -s.Length))
			b := s.ThreadLength()
			t := drawing.Thread{A: r.Place.Pt(geom.V3(0, 0, -s.Length+b)), B: tip, R: (m.d - 1.22687*m.pitch) / 2,
				External: true, Outline: m.d / 2, Face: facet}
			if _, _, h, err := s.check(); err == nil {
				shank := s.Length
				if ScrewStdByID(s.Std).Sunk {
					shank -= h.k
				}
				if b < shank-1e-6 {
					t.Limit = m.d / 2
				}
			}
			out = append(out, t)
		case func() bool { p, ok := FastenerOf(c); return ok && p.Kind == "nut" && p.Modelled }():
			p, _ := FastenerOf(c)
			m := metrics[p.Size]
			out = append(out, drawing.Thread{A: r.Place.Pt(geom.Vec3{}), B: r.Place.Pt(geom.V3(0, 0, p.Thickness())), R: m.d / 2,
				Outline: m.d/2 - 0.541266*m.pitch, Face: facet})
		}
		return out
	}
	// tapped holes of this design: each thread's span is its facets'
	byStep := map[int][]*brep.Face{}
	for _, f := range r.Body.Faces() {
		if drawing.ThreadFacet(f.Name) {
			if s := d.holeStepOfFace(f.Name); s != nil {
				byStep[s.ID] = append(byStep[s.ID], f)
			}
		}
	}
	for id, faces := range byStep {
		step := d.Feature(id)
		for _, t := range step.Threads {
			R := t.D / 2
			lo, hi := math.Inf(1), math.Inf(-1)
			name := ""
			for _, f := range faces {
				for _, l := range f.Loops {
					for _, h := range l.HalfEdges() {
						p := h.Edge.Verts[0].P.Sub(t.F.O)
						z := p.Dot(t.F.Z)
						if p.Sub(t.F.Z.Scale(z)).Len() > R+1e-6 {
							continue
						}
						lo, hi = math.Min(lo, z), math.Max(hi, z)
						if name == "" {
							name = f.Name
						}
					}
				}
			}
			if hi <= lo {
				continue
			}
			out = append(out, drawing.Thread{A: t.F.O.Add(t.F.Z.Scale(lo)), B: t.F.O.Add(t.F.Z.Scale(hi)), R: R,
				Outline: R - 0.541266*t.P, Face: name})
		}
	}
	return out
}
