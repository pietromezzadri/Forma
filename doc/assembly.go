package doc

import (
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"path/filepath"

	"forma/brep"
	"forma/geom"
	fio "forma/io"
)

// Assemblies: a component is another design inserted into this one, its
// bodies kept apart (never joined or cut by the insert) and placed by a
// rigid motion; a joint moves one component so a face or edge of it meets
// one of another, then turns or slides it by the joint's motion.

var (
	// ComponentInputs: the source file's name and the placement (as Move's).
	ComponentInputs = append([][2]string{{"File", ""}, {"Grounded", "0"}}, MoveInputs...)
	JointInputs     = [][2]string{{"Type", "Rigid"}, {"Angle", "0"}, {"Distance", "0"}, {"Flip", "0"}}
	JointTypes      = []string{"Rigid", "Revolute", "Slider", "Cylindrical"}
)

// JointRef is where a joint attaches: a face (its centre and normal, or a
// round face's axis), an edge (a circle's centre and axis, a line's middle
// and direction) or a vertex, on a body of a component (or any body).
// Points are in the component's own frame, so they stay put when it moves.
type JointRef struct {
	Feature int        `json:"feature"` // the component, or the feature that made the body
	Key     int        `json:"key"`     // the body
	Face    string     `json:"face,omitempty"`
	Edge    *EdgeRef   `json:"edge,omitempty"`
	Point   *geom.Vec3 `json:"point,omitempty"`
	// Out turns a round edge's frame so its Z points out of the part (along
	// the outward normal of the flat face beside it): a hole's rim, whichever
	// way its circle runs.
	Out bool `json:"out,omitempty"`
	// Frame is a frame fixed in the component's own frame (a screw's seat:
	// where its head sits, Z down the shank).
	Frame *geom.Frame `json:"frame,omitempty"`
}

// NewComponent inserts a design (a .forma file's text) as a component.
func NewComponent(file string, design []byte) *Feature {
	return withInputs(&Feature{Kind: KComponent, Name: baseName(file), Data: string(design),
		Inputs: map[string]string{"File": filepath.Base(file)}}, ComponentInputs)
}

// NewInstance places another copy of component src (sharing its design).
func NewInstance(src *Feature) *Feature {
	return withInputs(&Feature{Kind: KComponent, Name: src.Name, Ref: src.ID,
		Inputs: map[string]string{"File": src.Inputs["File"]}}, ComponentInputs)
}

// NewJoint joins moving (b) to fixed (a).
func NewJoint(a, b JointRef, typ string) *Feature {
	f := withInputs(&Feature{Kind: KJoint, Joint: []JointRef{a, b}}, JointInputs)
	f.Inputs["Type"] = typ
	return f
}

// MotionInput is the input a joint's motion drives (revolute: Angle,
// slider: Distance), "" for a rigid one.
func MotionInput(f *Feature) string {
	switch f.Inputs["Type"] {
	case "Revolute", "Cylindrical":
		return "Angle"
	case "Slider":
		return "Distance"
	}
	return ""
}

// componentData is a component's design text (an instance's from its source).
func (d *Document) componentData(f *Feature) (string, error) {
	seen := map[int]bool{}
	for f.Ref != 0 {
		if seen[f.ID] {
			return "", fmt.Errorf("instances refer to each other in a ring")
		}
		seen[f.ID] = true
		src := d.Feature(f.Ref)
		if src == nil || src.Kind != KComponent {
			return "", fmt.Errorf("the component it is a copy of is gone")
		}
		f = src
	}
	return f.Data, nil
}

// subBodies regenerates a component's design (once per distinct design)
// and returns its bodies as they stand there.
func (d *Document) subBodies(data string) ([]*brep.Body, []string, error) {
	h := fnv.New64a()
	h.Write([]byte(data))
	key := h.Sum64()
	if c, ok := d.subCache[key]; ok {
		return c.bodies, c.names, c.err
	}
	var c subDesign
	sub, err := Unmarshal([]byte(data))
	if err != nil {
		c.err = fmt.Errorf("the component's design: %w", err)
	} else {
		sub.Regenerate()
		for _, r := range sub.Bodies {
			if f := sub.Feature(r.Owner); f != nil && f.Hidden {
				continue
			}
			c.bodies = append(c.bodies, r.Body)
			c.names = append(c.names, r.Name)
		}
		if len(c.bodies) == 0 {
			c.err = fmt.Errorf("the component's design has no bodies")
		}
	}
	if d.subCache == nil {
		d.subCache = map[uint64]subDesign{}
	}
	if len(d.subCache) > 32 {
		d.subCache = map[uint64]subDesign{}
	}
	d.subCache[key] = c
	return c.bodies, c.names, c.err
}

type subDesign struct {
	bodies []*brep.Body
	names  []string
	err    error
}

func (d *Document) buildComponent(f *Feature) error {
	// a screw: its design from its standard, size and length (so an edit of
	// them rebuilds it)
	if spec, ok := ScrewOf(f); ok && f.Ref == 0 {
		data, err := ScrewData(spec)
		if err != nil {
			return err
		}
		f.Data = data
		f.Inputs["File"] = spec.Label() + ".forma"
	}
	data, err := d.componentData(f)
	if err != nil {
		return err
	}
	bodies, names, err := d.subBodies(data)
	if err != nil {
		return err
	}
	place, err := d.MoveXform(f)
	if err != nil {
		return err
	}
	prefix := fmt.Sprintf("f%d/", f.ID)
	f.Local, f.Bodies = nil, nil
	for k, b := range bodies {
		l, err := brep.Transform(b, geom.Identity, func(n string) string { return prefix + n })
		if err != nil {
			return err
		}
		l.Name = names[k]
		w, err := brep.Transform(l, place, nil)
		if err != nil {
			return err
		}
		w.Name = f.Name
		if len(bodies) > 1 {
			w.Name = f.Name + ": " + names[k]
		}
		f.Local = append(f.Local, l)
		f.Bodies = append(f.Bodies, w)
	}
	f.Place = place
	return nil
}

// addComponent puts a component's bodies into the design as its own.
func (d *Document) addComponent(f *Feature, state []*BodyRef) []*BodyRef {
	out := append([]*BodyRef(nil), state...)
	for k, b := range f.Bodies {
		out = append(out, &BodyRef{Key: BodyKey(f.ID, k), Name: b.Name, Body: b, Owner: f.ID, Gen: d.nextGen(),
			Comp: f.ID, Local: f.Local[k], Place: f.Place})
	}
	return out
}

// moveRefs moves bodies rigidly, keeping what component they are.
func (d *Document) moveRefs(out []*BodyRef, idx []int, x geom.Xform) error {
	for _, i := range idx {
		r := out[i]
		b, err := brep.Transform(r.Body, x, nil)
		if err != nil {
			return err
		}
		b.Name = r.Body.Name
		out[i] = &BodyRef{Key: r.Key, Name: r.Name, Body: b, Owner: r.Owner, Gen: d.nextGen(),
			Comp: r.Comp, Local: r.Local, Place: r.Place.Then(x)}
	}
	return nil
}

// refBodies are the bodies a joint end moves with: its component's, or
// the one body.
func refBodies(state []*BodyRef, j JointRef) []int {
	var out []int
	for i, r := range state {
		if (r.Comp != 0 && r.Comp == j.Feature) || r.Key == j.Key {
			out = append(out, i)
		}
	}
	return out
}

// placeOf is the placement of the component (or body) a joint end is on.
func placeOf(state []*BodyRef, j JointRef) geom.Xform {
	for _, i := range refBodies(state, j) {
		if state[i].Comp != 0 {
			return state[i].Place
		}
	}
	return geom.Identity
}

func (d *Document) joint(f *Feature, out []*BodyRef) ([]*BodyRef, error) {
	if len(f.Joint) != 2 {
		return nil, fmt.Errorf("pick a face, edge or vertex on each of two components")
	}
	a, b := f.Joint[0], f.Joint[1]
	// a grounded component stays put: the other end moves instead
	if d.groundedRef(out, b) {
		if d.groundedRef(out, a) {
			return nil, fmt.Errorf("both components are grounded: unground one")
		}
		a, b = b, a
	}
	moving := refBodies(out, b)
	if len(moving) == 0 {
		return nil, fmt.Errorf("the moving component is gone")
	}
	for _, i := range refBodies(out, a) {
		for _, k := range moving {
			if i == k {
				return nil, fmt.Errorf("both ends are on the same component")
			}
		}
	}
	fa, err := d.jointFrame(out, a)
	if err != nil {
		return nil, fmt.Errorf("first end: %w", err)
	}
	fb, err := d.jointFrame(out, b)
	if err != nil {
		return nil, fmt.Errorf("second end: %w", err)
	}
	f.Warn = ""
	ang, err := d.limited(f, "Angle")
	if err != nil {
		return nil, err
	}
	dist, err := d.limited(f, "Distance")
	if err != nil {
		return nil, err
	}
	x := JointXform(fa, fb, ang*math.Pi/180, dist, f.Inputs["Flip"] == "1")
	if err := d.moveRefs(out, moving, x); err != nil {
		return nil, err
	}
	f.Moved, f.JointAxis = b.Feature, fa
	return out, nil
}

// Grounded reports a component fixed where it is: joints move the other
// end, and nothing moves it.
func Grounded(f *Feature) bool {
	return f != nil && f.Kind == KComponent && f.Inputs["Grounded"] == "1"
}

// groundedRef: a joint end lies on a grounded component.
func (d *Document) groundedRef(state []*BodyRef, j JointRef) bool {
	for _, i := range refBodies(state, j) {
		if c := state[i].Comp; c != 0 && Grounded(d.Feature(c)) {
			return true
		}
	}
	return false
}

// Limits are the range a joint's motion input ("Angle" or "Distance") may
// take (its "<input> min" and "<input> max"); ok is false when it has none.
func (d *Document) Limits(f *Feature, input string) (lo, hi float64, ok bool) {
	lo, hi = math.Inf(-1), math.Inf(1)
	if s := f.Inputs[input+" min"]; s != "" {
		if v, err := d.Eval(s); err == nil {
			lo, ok = v, true
		}
	}
	if s := f.Inputs[input+" max"]; s != "" {
		if v, err := d.Eval(s); err == nil {
			hi, ok = v, true
		}
	}
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi, ok
}

// limited is a joint input held within its limits (noted in Warn when
// it had to be).
func (d *Document) limited(f *Feature, input string) (float64, error) {
	v, err := d.input(f, input)
	if err != nil {
		return 0, err
	}
	lo, hi, ok := d.Limits(f, input)
	if !ok {
		return v, nil
	}
	c := math.Max(lo, math.Min(hi, v))
	if c != v {
		unit := " mm"
		if input == "Angle" {
			unit = "°"
		}
		f.Warn = fmt.Sprintf("%s held at its limit, %g%s", input, c, unit)
	}
	return c, nil
}

// JointXform moves frame b onto frame a, facing it (Z against Z) unless
// flipped, then turns it by angle about a's Z and slides it dist along it.
func JointXform(a, b geom.Frame, angle, dist float64, flip bool) geom.Xform {
	t := a
	if !flip { // turned half round a's X: the two faces meet
		t.Y, t.Z = a.Y.Neg(), a.Z.Neg()
	}
	motion := geom.Rotate(a.O, a.Z, angle).Then(geom.Translate(a.Z.Scale(dist)))
	return frameXform(b).Inverse().Then(frameXform(t)).Then(motion)
}

func frameXform(f geom.Frame) geom.Xform {
	return geom.Xform{R: [3]geom.Vec3{f.X, f.Y, f.Z}, T: f.O}
}

// square makes a frame right-handed and orthonormal about its Z.
func square(o, z, x geom.Vec3) geom.Frame {
	return geom.FrameZ(o, z, x)
}

// jointFrame is the frame of a joint end as the design stands.
func (d *Document) jointFrame(state []*BodyRef, j JointRef) (geom.Frame, error) {
	idx := refBodies(state, j)
	if len(idx) == 0 {
		return geom.Frame{}, fmt.Errorf("its body is gone")
	}
	place := placeOf(state, j)
	world := func(p geom.Vec3) geom.Vec3 { return place.Pt(p) }
	switch {
	case j.Frame != nil:
		fr := *j.Frame
		return square(place.Pt(fr.O), place.Dir(fr.Z), place.Dir(fr.X)), nil
	case j.Face != "":
		for _, i := range idx {
			for _, fc := range state[i].Body.Faces() {
				if fc.Name == j.Face {
					return FaceFrame(state[i].Body, fc), nil
				}
			}
		}
		return geom.Frame{}, fmt.Errorf("face %s is gone", shortName(j.Face))
	case j.Edge != nil:
		ref := *j.Edge
		ref.P = world(ref.P)
		var sub []*BodyRef
		for _, i := range idx {
			sub = append(sub, state[i])
		}
		_, e := d.findEdge(sub, ref)
		if e == nil {
			return geom.Frame{}, fmt.Errorf("edge between %s and %s is gone", shortName(ref.A), shortName(ref.B))
		}
		if j.Out {
			return outwardFrame(e, EdgeFrame(e)), nil
		}
		return EdgeFrame(e), nil
	case j.Point != nil:
		p := world(*j.Point)
		best, bd := geom.Vec3{}, math.Inf(1)
		for _, i := range idx {
			for _, v := range state[i].Body.Vertices() {
				if dd := v.P.Dist(p); dd < bd {
					best, bd = v.P, dd
				}
			}
		}
		if math.IsInf(bd, 1) {
			return geom.Frame{}, fmt.Errorf("no vertex")
		}
		z, x := place.Dir(geom.ZAxis), place.Dir(geom.XAxis)
		return square(best, z, x), nil
	}
	return geom.Frame{}, fmt.Errorf("nothing picked")
}

// FaceFrame is a joint frame on a face: a flat face's centre and outward
// normal; a round face's axis through the middle of it.
func FaceFrame(b *brep.Body, fc *brep.Face) geom.Frame {
	c := faceCentre(b, fc)
	switch s := fc.Surf.(type) {
	case geom.Plane:
		n := s.F.Z
		if fc.Reversed {
			n = n.Neg()
		}
		return square(c, n, s.F.X)
	case geom.Cylinder:
		return axisFrame(s.F, c)
	case geom.Cone:
		return axisFrame(s.F, c)
	case geom.Sphere:
		return square(s.F.O, s.F.Z, s.F.X)
	case geom.Torus:
		return square(s.F.O, s.F.Z, s.F.X)
	}
	return square(c, fc.OutwardNormal(fc.Surf.Project(c)), geom.XAxis)
}

func axisFrame(f geom.Frame, c geom.Vec3) geom.Frame {
	o := f.O.Add(f.Z.Scale(c.Sub(f.O).Dot(f.Z)))
	return square(o, f.Z, f.X)
}

// faceCentre is the centre (of area) of a face.
func faceCentre(b *brep.Body, fc *brep.Face) geom.Vec3 {
	m := brep.Tessellate(b, brep.DefaultMeshOptions(b))
	var c geom.Vec3
	area := 0.0
	for ti, t := range m.Tris {
		if m.TriFace[ti] != fc.ID {
			continue
		}
		p, q, r := m.Pos[t[0]], m.Pos[t[1]], m.Pos[t[2]]
		a := q.Sub(p).Cross(r.Sub(p)).Len() / 2
		c = c.Add(p.Add(q).Add(r).Scale(a / 3))
		area += a
	}
	if area == 0 {
		return b.Bounds().Center()
	}
	return c.Scale(1 / area)
}

// EdgeFrame is a joint frame on an edge: a circle's centre and axis (also a
// polyline lying on a circle), a line's middle and direction.
func EdgeFrame(e *brep.Edge) geom.Frame {
	if c, ok := fio.EdgeCircle(e); ok {
		return square(c.F.O, c.F.X.Cross(c.F.Y), c.F.X)
	}
	a, b := e.Verts[0].P, e.Verts[1].P
	if l, ok := e.Curve.(geom.Line); ok {
		mid := e.Curve.Eval((e.T0 + e.T1) / 2)
		d := l.D
		if e.T1 < e.T0 {
			d = d.Neg()
		}
		return square(mid, d, perpTo(d))
	}
	mid := e.Curve.Eval((e.T0 + e.T1) / 2)
	d := b.Sub(a)
	if d.Len() < geom.Tol {
		d = e.Curve.Deriv((e.T0 + e.T1) / 2)
	}
	d = d.Norm()
	return square(mid, d, perpTo(d))
}

// outwardFrame turns an edge's frame half round its X when its Z points
// into the part: against the outward normal of a flat face beside it.
func outwardFrame(e *brep.Edge, fr geom.Frame) geom.Frame {
	if n, ok := rimNormal(e); ok && n.Dot(fr.Z) < 0 {
		fr.Y, fr.Z = fr.Y.Neg(), fr.Z.Neg()
	}
	return fr
}

// rimNormal is the outward normal of a flat face beside an edge.
func rimNormal(e *brep.Edge) (geom.Vec3, bool) {
	for _, h := range e.HE {
		if h == nil {
			continue
		}
		f := h.Loop.Face
		if _, ok := f.Surf.(geom.Plane); !ok {
			continue
		}
		p := e.Curve.Eval((e.T0 + e.T1) / 2)
		return f.OutwardNormal(f.Surf.Project(p)).Norm(), true
	}
	return geom.Vec3{}, false
}

func perpTo(d geom.Vec3) geom.Vec3 {
	if math.Abs(d.Z) < 0.9 {
		return geom.ZAxis.Cross(d)
	}
	return geom.XAxis.Cross(d)
}

// JointRefFor makes a joint end on a face, edge or vertex of a body of the
// design (key), with its points in the component's own frame.
func (d *Document) JointRefFor(key int, face *brep.Face, edge *brep.Edge, vertex *brep.Vertex, at geom.Vec3) (JointRef, error) {
	var r *BodyRef
	for _, b := range d.Bodies {
		if b.Key == key {
			r = b
		}
	}
	if r == nil {
		return JointRef{}, fmt.Errorf("no such body")
	}
	j := JointRef{Feature: r.Owner, Key: key}
	if r.Comp != 0 {
		j.Feature = r.Comp
	}
	local := geom.Identity
	if r.Comp != 0 {
		local = r.Place.Inverse()
	}
	switch {
	case face != nil:
		j.Face = face.Name
	case edge != nil:
		ref, ok := RefForEdge(edge, at)
		if !ok {
			return JointRef{}, fmt.Errorf("that edge can't be referred to")
		}
		ref.P = local.Pt(ref.P)
		j.Edge = &ref
	case vertex != nil:
		p := local.Pt(vertex.P)
		j.Point = &p
	default:
		return JointRef{}, fmt.Errorf("pick a face, edge or vertex")
	}
	return j, nil
}

// ComponentOf is the component a body of the design belongs to (0: none).
func (d *Document) ComponentOf(key int) int {
	for _, b := range d.Bodies {
		if b.Key == key {
			return b.Comp
		}
	}
	return 0
}

// StepAssembly is the design as an assembly for STEP: each distinct
// component design a part (its bodies in its own frame), each component an
// instance where it stands; bodies in no component (or reshaped since it
// was inserted) are the design's own. keep picks the bodies to write.
func (d *Document) StepAssembly(name string, keep func(*BodyRef) bool) fio.StepAssembly {
	a := fio.StepAssembly{Name: name}
	partOf := map[string]int{}
	byComp := map[int][]*BodyRef{}
	var comps []int
	for _, r := range d.Bodies {
		if !keep(r) {
			continue
		}
		if r.Comp == 0 || r.Local == nil {
			a.Bodies = append(a.Bodies, r.Body)
			continue
		}
		if _, ok := byComp[r.Comp]; !ok {
			comps = append(comps, r.Comp)
		}
		byComp[r.Comp] = append(byComp[r.Comp], r)
	}
	for _, c := range comps {
		refs := byComp[c]
		f := d.Feature(c)
		data, err := d.componentData(f)
		whole := err == nil && len(refs) == len(f.Local)
		for _, r := range refs {
			if r.Place != refs[0].Place {
				whole = false
			}
		}
		if !whole { // moved apart or partly hidden: as plain bodies
			for _, r := range refs {
				a.Bodies = append(a.Bodies, r.Body)
			}
			continue
		}
		p, ok := partOf[data]
		if !ok {
			p = len(a.Parts)
			partOf[data] = p
			src := f
			for src.Ref != 0 {
				src = d.Feature(src.Ref)
			}
			a.Parts = append(a.Parts, fio.StepPart{Name: src.Name, Bodies: f.Local})
		}
		a.Instances = append(a.Instances, fio.StepInstance{Part: p, Name: f.Name, Place: refs[0].Place})
	}
	return a
}

// Clash is where two bodies of the design overlap.
type Clash struct {
	A, B   *BodyRef
	Volume float64
	Shape  *brep.Body // the overlap (nil when it couldn't be worked out)
	Err    error
}

// Interference finds the bodies (of different components, or not in the
// same one) that overlap.
func (d *Document) Interference(keep func(*BodyRef) bool) []Clash {
	var bodies []*BodyRef
	for _, r := range d.Bodies {
		if keep(r) {
			bodies = append(bodies, r)
		}
	}
	var out []Clash
	for i := 0; i < len(bodies); i++ {
		for j := i + 1; j < len(bodies); j++ {
			a, b := bodies[i], bodies[j]
			if a.Comp != 0 && a.Comp == b.Comp {
				continue
			}
			if !overlaps(a.Body.Bounds(), b.Body.Bounds()) {
				continue
			}
			s, err := brep.Boolean(a.Body, b.Body, brep.Intersect)
			if errors.Is(err, brep.ErrEmpty) {
				continue // touching, or boxes only overlapping
			}
			if err != nil {
				out = append(out, Clash{A: a, B: b, Err: err})
				continue
			}
			if len(s.Faces()) == 0 {
				continue
			}
			v := brep.Mass(s, 2e-4).Volume
			if v > 1e-6*math.Min(a.Body.Bounds().Diagonal(), b.Body.Bounds().Diagonal()) {
				out = append(out, Clash{A: a, B: b, Volume: v, Shape: s})
			}
		}
	}
	return out
}
