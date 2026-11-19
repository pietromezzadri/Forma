package io

// STEP assembly structure: a design with components is written as an
// assembly product whose parts are placed by NEXT_ASSEMBLY_USAGE_OCCURRENCEs
// (one per instance, each with an ITEM_DEFINED_TRANSFORMATION between an
// axis of the part and one of the assembly, as OpenCASCADE writes them);
// reading walks those placements so every solid comes in where its
// occurrences put it.

import (
	"bufio"
	"fmt"
	stdio "io"
	"time"

	"forma/brep"
	"forma/geom"
)

// StepPart is a part of an assembly: bodies in its own frame.
type StepPart struct {
	Name   string
	Bodies []*brep.Body
}

// StepInstance places a part (by index) in the assembly.
type StepInstance struct {
	Part  int
	Name  string
	Place geom.Xform // rigid
}

// StepAssembly is a design: its own bodies (written as a part placed where
// it is) and its components' instances.
type StepAssembly struct {
	Name      string
	Bodies    []*brep.Body // not in any component
	Parts     []StepPart
	Instances []StepInstance
}

// WriteSTEPAssembly writes an assembly; with no instances it is WriteSTEP.
func WriteSTEPAssembly(w stdio.Writer, a StepAssembly, opt StepOptions) error {
	if len(a.Instances) == 0 {
		return WriteSTEP(w, a.Bodies, opt)
	}
	if opt.Name == "" {
		opt.Name = a.Name
	}
	if opt.Name == "" {
		opt.Name = "Forma assembly"
	}
	if opt.Time.IsZero() {
		opt.Time = time.Now()
	}
	s := newStepWriter()
	ctx := s.context()
	origin := s.axis(geom.Vec3{}, geom.ZAxis, geom.XAxis)

	type placed struct {
		rep, def int
		axis     int // its origin axis (in its representation)
		name     string
	}
	part := func(name string, bodies []*brep.Body) (placed, error) {
		ax := s.axis(geom.Vec3{}, geom.ZAxis, geom.XAxis)
		items := []int{ax}
		for _, b := range bodies {
			ids, err := s.solids(b)
			if err != nil {
				return placed{}, fmt.Errorf("%s: body %q: %w", name, b.Name, err)
			}
			items = append(items, ids...)
		}
		rep := s.add("ADVANCED_BREP_SHAPE_REPRESENTATION(%s,%s,#%d)", str(name), refs(items), ctx)
		return placed{rep: rep, def: s.product(name, rep), axis: ax, name: name}, nil
	}
	type use struct {
		p     placed
		place geom.Xform
		name  string
	}
	var uses []use
	if len(a.Bodies) > 0 { // the design's own bodies: a part where they are
		own, err := part(opt.Name+" bodies", a.Bodies)
		if err != nil {
			return err
		}
		uses = append(uses, use{own, geom.Identity, own.name})
	}
	var parts []placed
	for _, p := range a.Parts {
		pp, err := part(p.Name, p.Bodies)
		if err != nil {
			return err
		}
		parts = append(parts, pp)
	}
	for _, in := range a.Instances {
		if in.Part < 0 || in.Part >= len(parts) {
			return fmt.Errorf("instance %q: no part %d", in.Name, in.Part)
		}
		uses = append(uses, use{parts[in.Part], in.Place, in.Name})
	}
	items := []int{origin}
	axes := make([]int, len(uses))
	for i, u := range uses {
		x := u.place
		axes[i] = s.axis(x.T, x.R[2], x.R[0])
		items = append(items, axes[i])
	}
	top := s.add("SHAPE_REPRESENTATION(%s,%s,#%d)", str(opt.Name), refs(items), ctx)
	topDef := s.product(opt.Name, top)
	for i, u := range uses {
		nauo := s.add("NEXT_ASSEMBLY_USAGE_OCCURRENCE('%d',%s,'',#%d,#%d,$)", i+1, str(u.name), topDef, u.p.def)
		pds := s.add("PRODUCT_DEFINITION_SHAPE('Placement','Placement of an item',#%d)", nauo)
		idt := s.add("ITEM_DEFINED_TRANSFORMATION('','',#%d,#%d)", u.p.axis, axes[i])
		rel := s.add("( REPRESENTATION_RELATIONSHIP('','',#%d,#%d) REPRESENTATION_RELATIONSHIP_WITH_TRANSFORMATION(#%d) SHAPE_REPRESENTATION_RELATIONSHIP() )",
			u.p.rep, top, idt)
		s.add("CONTEXT_DEPENDENT_SHAPE_REPRESENTATION(#%d,#%d)", rel, pds)
	}
	return s.write(w, opt)
}

// write puts out the file round the instances made.
func (s *stepWriter) write(w stdio.Writer, opt StepOptions) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "ISO-10303-21;\nHEADER;\n")
	fmt.Fprintf(bw, "FILE_DESCRIPTION(('Forma model'),'2;1');\n")
	fmt.Fprintf(bw, "FILE_NAME(%s,%s,(%s),(''),'Forma','Forma','');\n",
		str(opt.Name), str(opt.Time.UTC().Format("2006-01-02T15:04:05")), str(opt.Author))
	fmt.Fprintf(bw, "FILE_SCHEMA(('AUTOMOTIVE_DESIGN { 1 0 10303 214 1 1 1 1 }'));\nENDSEC;\nDATA;\n")
	for i, l := range s.lines {
		fmt.Fprintf(bw, "#%d=%s;\n", i+1, l)
	}
	fmt.Fprintf(bw, "ENDSEC;\nEND-ISO-10303-21;\n")
	return bw.Flush()
}

// placements are where each representation's contents go in the world:
// one transform per occurrence, through every level of the assembly.
func (r *stepReader) placements() map[int][]geom.Xform {
	type link struct {
		parent int
		x      geom.Xform
	}
	up := map[int][]link{}
	for _, id := range r.f.Order {
		e := r.f.Ents[id]
		rr := e.Part("REPRESENTATION_RELATIONSHIP")
		if rr == nil || len(rr.Args) < 4 {
			continue
		}
		child, parent := arg(rr.Args, 2).Ref, arg(rr.Args, 3).Ref
		x := geom.Identity
		if t := e.Part("REPRESENTATION_RELATIONSHIP_WITH_TRANSFORMATION"); t != nil {
			te, err := r.ent(arg(t.Args, 0))
			if err != nil || te.Type() != "ITEM_DEFINED_TRANSFORMATION" {
				continue
			}
			ta := te.Parts[0].Args
			f1, err1 := r.frame(arg(ta, 2))
			f2, err2 := r.frame(arg(ta, 3))
			if err1 != nil || err2 != nil {
				continue
			}
			x = frameX(f1).Inverse().Then(frameX(f2))
		}
		up[child] = append(up[child], link{parent, x})
	}
	memo := map[int][]geom.Xform{}
	var world func(rep int, depth int, seen map[int]bool) []geom.Xform
	world = func(rep int, depth int, seen map[int]bool) []geom.Xform {
		if w, ok := memo[rep]; ok {
			return w
		}
		ls := up[rep]
		if len(ls) == 0 || depth > 32 || seen[rep] {
			return []geom.Xform{geom.Identity}
		}
		seen[rep] = true
		var out []geom.Xform
		for _, l := range ls {
			for _, pw := range world(l.parent, depth+1, seen) {
				out = append(out, l.x.Then(pw))
			}
		}
		delete(seen, rep)
		memo[rep] = out
		return out
	}
	// the representations holding solids
	out := map[int][]geom.Xform{}
	for _, id := range r.f.Order {
		e := r.f.Ents[id]
		if len(e.Parts) != 1 || len(e.Parts[0].Args) < 3 {
			continue
		}
		t := e.Type()
		if t != "ADVANCED_BREP_SHAPE_REPRESENTATION" && t != "SHAPE_REPRESENTATION" && t != "MANIFOLD_SURFACE_SHAPE_REPRESENTATION" {
			continue
		}
		for _, it := range arg(e.Parts[0].Args, 1).List {
			ie := r.f.Ents[it.Ref]
			if ie != nil && (ie.Is("MANIFOLD_SOLID_BREP") || ie.Is("BREP_WITH_VOIDS")) {
				if r.solidRep == nil {
					r.solidRep = map[int]int{}
				}
				r.solidRep[it.Ref] = id
				out[it.Ref] = append(out[it.Ref], world(id, 0, map[int]bool{})...)
			}
		}
	}
	return out
}

func frameX(f geom.Frame) geom.Xform {
	return geom.Xform{R: [3]geom.Vec3{f.X, f.Y, f.Z}, T: f.O}
}

// productNames names each representation by the product whose shape it is.
func (r *stepReader) productNames() map[int]string {
	out := map[int]string{}
	for _, id := range r.f.Order {
		e := r.f.Ents[id]
		if e.Type() != "SHAPE_DEFINITION_REPRESENTATION" {
			continue
		}
		a := e.Parts[0].Args
		pds, err := r.ent(arg(a, 0))
		if err != nil {
			continue
		}
		def, err := r.ent(arg(pds.Parts[0].Args, 2))
		if err != nil || def.Type() != "PRODUCT_DEFINITION" {
			continue
		}
		form, err := r.ent(arg(def.Parts[0].Args, 2))
		if err != nil {
			continue
		}
		prod, err := r.ent(arg(form.Parts[0].Args, 2))
		if err != nil || prod.Type() != "PRODUCT" {
			continue
		}
		if n := arg(prod.Parts[0].Args, 1); n.Kind == p21Str && n.Str != "" {
			out[arg(a, 1).Ref] = n.Str
		}
	}
	return out
}
