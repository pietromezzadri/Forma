package doc

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strconv"

	"forma/brep"
	"forma/expr"
	"forma/geom"
	"forma/ops"
	"forma/sketch"
)

// Regenerate rebuilds every feature in order up to the rollback marker. A
// feature whose definition, parameters and inputs are as last time keeps
// its results (and its Gen), so dragging a sketch only rebuilds what
// follows from it.
func (d *Document) Regenerate() {
	d.evalParams()
	active := d.ActiveCount()
	sigs := map[int]uint64{}
	var state []*BodyRef
	d.regen = true
	defer func() { d.regen = false }()
	for i, f := range d.Features {
		f.Active = i < active && !f.Off
		if !f.Active {
			if f.Bodies != nil || f.Regions != nil || f.After != nil || f.RefMesh != nil {
				f.Bodies, f.Regions, f.After, f.RefMesh = nil, nil, nil, nil
				f.Gen++
			}
			f.sig = 0
			continue
		}
		in := stateSig(state)
		sig := mix(d.signature(f, sigs), in)
		sigs[f.ID] = sig
		if sig == f.sig && f.Err == "" {
			if f.makesBodies() {
				state = f.After
			}
			continue
		}
		f.Gen++
		f.Err = ""
		f.Bodies, f.Regions, f.RefMesh, f.Warn = nil, nil, nil, ""
		f.Local = nil
		d.state = state
		if err := d.build(f); err != nil {
			f.Err = err.Error()
			f.Bodies = nil
		}
		if f.makesBodies() {
			state = d.apply(f, state)
			f.After = state
		}
		// building may settle the definition (a sketch solves): sign what it is now
		f.sig = mix(d.signature(f, sigs), in)
		sigs[f.ID] = f.sig
	}
	d.Bodies = state
	d.state = nil
}

// BodyRef is a body of the design as it stands after some feature: made by
// feature Owner (its Key is Owner × 16 + its index there), changed by later
// joins and cuts (each change gives it a new Gen).
type BodyRef struct {
	Key   int
	Name  string
	Body  *brep.Body
	Owner int
	Gen   int
	// a component's body: which, its body in the component's frame, and
	// where the component is (Body = Place(Local)); cleared once a join or
	// cut changes it
	Comp  int
	Local *brep.Body
	Place geom.Xform
}

func (f *Feature) makesBodies() bool {
	switch f.Kind {
	case KSketch, KMesh:
		return false
	}
	return true
}

func mix(a, b uint64) uint64 { return a*1099511628211 ^ b }

func stateSig(s []*BodyRef) uint64 {
	h := fnv.New64a()
	for _, b := range s {
		fmt.Fprintf(h, "%d:%d;", b.Key, b.Gen)
	}
	return h.Sum64()
}

func (d *Document) nextGen() int { d.gen++; return d.gen }

// Operations a body-making feature can do (input "Operation").
var Operations = []string{"New body", "Join", "Cut", "Intersect"}

// apply puts a feature's bodies into the design: as new bodies, or joined
// to, cut from or intersected with the bodies they touch.
func (d *Document) apply(f *Feature, state []*BodyRef) []*BodyRef {
	out := append([]*BodyRef(nil), state...)
	if f.Err != "" {
		return out
	}
	if f.Kind == KComponent {
		return d.addComponent(f, state)
	}
	if f.modifies() {
		r, err := d.modify(f, state)
		if err != nil {
			f.Err = err.Error()
			return out
		}
		return r
	}
	if len(f.Bodies) == 0 {
		return out
	}
	k := 0
	r, err := d.applyOp(opOf(f), f.Bodies, state, func(b *brep.Body) *BodyRef {
		ref := &BodyRef{Key: BodyKey(f.ID, k), Name: f.BodyName(k), Body: b, Owner: f.ID, Gen: d.nextGen()}
		k++
		return ref
	})
	if err != nil {
		f.Err = err.Error()
		return out
	}
	return r
}

// applyOp puts tool bodies into the bodies there are: as new bodies (made
// by fresh), or joined to, cut from or intersected with the bodies they
// touch.
func (d *Document) applyOp(op string, tools []*brep.Body, state []*BodyRef, fresh func(*brep.Body) *BodyRef) ([]*BodyRef, error) {
	out := append([]*BodyRef(nil), state...)
	touching := func(tool *brep.Body) []int {
		tb := tool.Bounds()
		var hit []int
		for i, r := range out {
			if overlaps(tb, r.Body.Bounds()) {
				hit = append(hit, i)
			}
		}
		return hit
	}
	changed := func(r *BodyRef, b *brep.Body) *BodyRef {
		b.Name = r.Body.Name
		return &BodyRef{Key: r.Key, Name: r.Name, Body: b, Owner: r.Owner, Gen: d.nextGen()}
	}
	switch op {
	case "Join":
		for _, tool := range tools {
			hit := touching(tool)
			if len(hit) == 0 {
				out = append(out, fresh(tool))
				continue
			}
			acc := tool
			for _, i := range hit {
				r, err := brep.Boolean(out[i].Body, acc, brep.Union)
				if err != nil {
					return nil, err
				}
				acc = r
			}
			first := hit[0]
			out[first] = changed(out[first], acc)
			out = removeAt(out, hit[1:])
		}
	case "Cut", "Intersect":
		bop := brep.Subtract
		if op == "Intersect" {
			bop = brep.Intersect
		}
		any := false
		for _, tool := range tools {
			hit := touching(tool)
			var gone []int
			for _, i := range hit {
				any = true
				r, err := brep.Boolean(out[i].Body, tool, bop)
				if err == brep.ErrEmpty {
					gone = append(gone, i)
					continue
				}
				if err != nil {
					return nil, err
				}
				out[i] = changed(out[i], r)
			}
			out = removeAt(out, gone)
		}
		if !any {
			return nil, fmt.Errorf("it doesn't touch any body")
		}
	default:
		for _, b := range tools {
			out = append(out, fresh(b))
		}
	}
	return out, nil
}

func removeAt(s []*BodyRef, idx []int) []*BodyRef {
	if len(idx) == 0 {
		return s
	}
	drop := map[int]bool{}
	for _, i := range idx {
		drop[i] = true
	}
	var out []*BodyRef
	for i, r := range s {
		if !drop[i] {
			out = append(out, r)
		}
	}
	return out
}

func overlaps(a, b geom.Box) bool {
	m := 1e-6 * (1 + a.Diagonal() + b.Diagonal())
	return a.Min.X <= b.Max.X+m && b.Min.X <= a.Max.X+m &&
		a.Min.Y <= b.Max.Y+m && b.Min.Y <= a.Max.Y+m &&
		a.Min.Z <= b.Max.Z+m && b.Min.Z <= a.Max.Z+m
}

// qualify names a feature's faces f<ID>/<role>, unique across the design so
// they stay findable after joins and cuts.
func qualify(f *Feature) {
	for _, b := range f.Bodies {
		for _, fc := range b.Faces() {
			fc.Name = fmt.Sprintf("f%d/%s", f.ID, fc.Name)
		}
	}
}

// signature hashes what a feature's results depend on.
func (d *Document) signature(f *Feature, sigs map[int]uint64) uint64 {
	h := fnv.New64a()
	b, _ := json.Marshal(f)
	h.Write(b)
	// parameter values (any of them: cheap and safe)
	names := make([]string, 0, len(d.vars))
	for k := range d.vars {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		fmt.Fprintf(h, "%s=%g;", k, d.vars[k])
	}
	for _, dep := range f.deps() {
		fmt.Fprintf(h, "dep%d=%d;", dep, sigs[dep])
		if g := d.Feature(dep); g != nil {
			fmt.Fprintf(h, "gen%d;", g.Gen)
		}
	}
	return h.Sum64()
}

func (d *Document) build(f *Feature) error {
	switch f.Kind {
	case KBox, KCylinder, KCone, KSphere, KTorus:
		b, err := d.buildPrim(f)
		if err != nil {
			return err
		}
		f.Bodies = []*brep.Body{b}
		qualify(f)
	case KSketch:
		return d.buildSketch(f)
	case KExtrude, KRevolve:
		err := d.buildSweep(f)
		qualify(f)
		return err
	case KHole:
		err := d.buildHole(f)
		qualify(f)
		return err
	case KImport:
		err := d.buildImport(f)
		qualify(f)
		return err
	case KMesh:
		return d.buildMesh(f)
	case KComponent:
		return d.buildComponent(f)
	case KMove, KMirror, KPattern, KFillet, KChamfer, KShell, KJoint:
		return nil // they work on the bodies there are (modify)
	default:
		return fmt.Errorf("unknown feature kind %q", f.Kind)
	}
	return nil
}

func (d *Document) buildPrim(f *Feature) (*brep.Body, error) {
	v := map[string]float64{}
	for _, kv := range PrimInputs[f.Kind] {
		x, err := d.input(f, kv[0])
		if err != nil {
			return nil, err
		}
		v[kv[0]] = x
	}
	at := geom.V3(v["X"], v["Y"], v["Z"])
	fr := geom.World
	fr.O = at
	var b *brep.Body
	var err error
	switch f.Kind {
	case KBox:
		b, err = ops.Box(at, at.Add(geom.V3(v["Width"], v["Depth"], v["Height"])))
	case KCylinder:
		b, err = ops.Cylinder(fr, v["Radius"], v["Height"])
	case KCone:
		b, err = ops.Cone(fr, v["Base radius"], v["Top radius"], v["Height"])
	case KSphere:
		b, err = ops.Sphere(at, v["Radius"])
	case KTorus:
		b, err = ops.Torus(fr, v["Major radius"], v["Tube radius"])
	}
	if b != nil {
		b.Name = f.Name
	}
	return b, err
}

// buildSketch places the sketch on its plane, re-reads dimensions written
// with parameters, solves it and finds its profiles.
func (d *Document) buildSketch(f *Feature) error {
	s := f.Sketch
	if s == nil {
		return fmt.Errorf("no sketch")
	}
	s.Name = f.Name
	if f.Plane != nil {
		switch {
		case f.Plane.Std != "":
			if pl, ok := StdPlane(f.Plane.Std); ok {
				s.Plane = pl
			}
		case f.Plane.Feature != 0:
			fc := d.FindFace(f.Plane.Feature, f.Plane.Body, f.Plane.Face)
			if fc == nil {
				s.Solve()
				f.Regions = s.Regions()
				return fmt.Errorf("its face %q is gone", f.Plane.Face)
			}
			pl, ok := PlaneOf(fc)
			if !ok {
				return fmt.Errorf("face %q is not flat", f.Plane.Face)
			}
			s.Plane = pl
		}
	}
	var dimErr error
	for _, c := range s.Cons {
		if c.Expr == "" || !c.Kind.Dimensional() {
			continue
		}
		v, err := expr.Eval(c.Expr, d.vars)
		if err != nil {
			dimErr = fmt.Errorf("dimension %q: %v", c.Expr, err)
			continue
		}
		v = DimValue(c, v)
		if math.Abs(v-c.Value) > 1e-12 {
			if err := s.SetValue(c.ID, v, c.Expr); err != nil {
				dimErr = fmt.Errorf("dimension %q = %s: %v", c.Expr, strconv.FormatFloat(v, 'g', 6, 64), err)
			}
		}
	}
	s.Solve()
	f.Regions = s.Regions()
	return dimErr
}

// DimValue turns a typed value (mm, or degrees for angles) into a
// constraint's value, keeping the sign the dimension was made with.
func DimValue(c *sketch.Constraint, v float64) float64 {
	switch c.Kind {
	case sketch.Angle:
		v = v * math.Pi / 180
		if c.Value < 0 {
			v = -v
		}
	case sketch.HDistance, sketch.VDistance:
		if c.Value < 0 {
			v = -v
		}
	}
	return v
}

func (d *Document) buildSweep(f *Feature) error {
	if len(f.Profiles) == 0 {
		return fmt.Errorf("no profile")
	}
	for i, ref := range f.Profiles {
		sk := d.Feature(ref.Sketch)
		if sk == nil || sk.Kind != KSketch {
			return fmt.Errorf("its sketch is gone")
		}
		if d.Index(sk.ID) > d.Index(f.ID) {
			return fmt.Errorf("its sketch comes after it in the timeline")
		}
		if !sk.Active {
			return fmt.Errorf("%s is suppressed or rolled back", sk.Name)
		}
		ri, ok := matchRegion(ref, sk.Regions)
		if !ok {
			return fmt.Errorf("profile %d of %s is gone (is the loop still closed?)", i+1, sk.Name)
		}
		reg := sk.Regions[ri]
		plane := sk.Sketch.Plane
		var b *brep.Body
		var err error
		if f.Kind == KExtrude {
			d0, d1, e := d.extent(f)
			if e != nil {
				return e
			}
			b, err = ops.Extrude(f.Name, reg, plane, d0, d1)
		} else {
			ang, e := d.input(f, "Angle")
			if e != nil {
				return e
			}
			p, dir, e := axisOf(f.Axis, sk.Sketch)
			if e != nil {
				return e
			}
			b, err = ops.Revolve(f.Name, reg, plane, p, dir, ang*math.Pi/180)
		}
		if err != nil {
			return err
		}
		b.Name = f.BodyName(i)
		f.Bodies = append(f.Bodies, b)
	}
	return nil
}

// extent is an extrude's span along the sketch normal.
func (d *Document) extent(f *Feature) (float64, float64, error) {
	dist, err := d.input(f, "Distance")
	if err != nil {
		return 0, 0, err
	}
	flip := f.Inputs["Flip"] == "1"
	switch f.Inputs["Direction"] {
	case "Symmetric":
		dist = math.Abs(dist)
		return -dist / 2, dist / 2, nil
	case "Two sides":
		d2, err := d.input(f, "Distance 2")
		if err != nil {
			return 0, 0, err
		}
		a, b := -math.Abs(d2), math.Abs(dist)
		if flip {
			a, b = -b, -a
		}
		return a, b, nil
	}
	if flip {
		dist = -dist
	}
	if dist < 0 {
		return dist, 0, nil
	}
	return 0, dist, nil
}

func axisOf(a *AxisRef, s *sketch.Sketch) (geom.Vec2, geom.Vec2, error) {
	if a == nil {
		return geom.Vec2{}, geom.Vec2{Y: 1}, nil
	}
	switch a.Kind {
	case "x":
		return geom.Vec2{}, geom.Vec2{X: 1}, nil
	case "line":
		e := s.Ent(a.Line)
		if e == nil || e.Kind != sketch.Line {
			return geom.Vec2{}, geom.Vec2{}, fmt.Errorf("its axis line is gone")
		}
		p, q := s.P(e.Pts[0]), s.P(e.Pts[1])
		return p, q.Sub(p), nil
	}
	return geom.Vec2{}, geom.Vec2{Y: 1}, nil
}
