package doc

import (
	"fmt"
	"math"
	"sort"

	"forma/brep"
	"forma/geom"
	"forma/ops"
)

// Inputs of the modelling features, in display order, with defaults.
var (
	MoveInputs = [][2]string{{"Move X", "0"}, {"Move Y", "0"}, {"Move Z", "0"},
		{"Rotate X", "0"}, {"Rotate Y", "0"}, {"Rotate Z", "0"},
		{"Pivot X", "0"}, {"Pivot Y", "0"}, {"Pivot Z", "0"}}
	LinearInputs   = [][2]string{{"Count", "3"}, {"Spacing", "20"}, {"Count 2", "1"}, {"Spacing 2", "20"}}
	CircularInputs = [][2]string{{"Count", "6"}, {"Angle", "360"}, {"Center X", "0"}, {"Center Y", "0"}, {"Center Z", "0"}}
	FilletInputs   = [][2]string{{"Radius", "3"}}
	ChamferInputs  = [][2]string{{"Distance", "2"}}
	HoleInputs     = [][2]string{{"Diameter", "6"}, {"Depth", "10"}, {"Bore diameter", "11"}, {"Bore depth", "4"}, {"Sink diameter", "12"}}
	ShellInputs    = [][2]string{{"Thickness", "1"}}
)

// Choices for the modelling features' combo inputs.
var (
	PatternTypes  = []string{"Linear", "Circular"}
	Axes          = []string{"X", "Y", "Z"}
	Axes2         = []string{"None", "X", "Y", "Z"}
	CopyModes     = []string{"Join", "New body"}
	HoleTypes     = []string{"Simple", "Counterbore", "Countersink"}
	HoleExtents   = []string{"Distance", "Through all"}
	HoleBottoms   = []string{"Drill point", "Flat"}
	MirrorPlanes  = []string{"YZ", "XZ", "XY"}
	stdMirrorName = map[string]string{"YZ": "yz", "XZ": "xz", "XY": "xy"}
)

func withInputs(f *Feature, in ...[][2]string) *Feature {
	if f.Inputs == nil {
		f.Inputs = map[string]string{}
	}
	for _, list := range in {
		for _, kv := range list {
			if _, ok := f.Inputs[kv[0]]; !ok {
				f.Inputs[kv[0]] = kv[1]
			}
		}
	}
	return f
}

// NewMove moves and turns bodies (pivot: where it turns about).
func NewMove(targets []int, pivot geom.Vec3) *Feature {
	f := withInputs(&Feature{Kind: KMove, Targets: targets}, append(append([][2]string(nil), MoveInputs...), [2]string{"Scale", "1"}))
	f.Inputs["Pivot X"] = fmtNum(pivot.X)
	f.Inputs["Pivot Y"] = fmtNum(pivot.Y)
	f.Inputs["Pivot Z"] = fmtNum(pivot.Z)
	return f
}

// NewMirror mirrors bodies, or (sources) features, in a standard plane.
func NewMirror(targets, sources []int) *Feature {
	f := &Feature{Kind: KMirror, Targets: targets, Sources: sources, Plane: &PlaneRef{Std: "yz"},
		Inputs: map[string]string{"Operation": CopyModes[0]}}
	return f
}

// NewPattern repeats bodies or features in a row (or grid) or round an axis.
func NewPattern(targets, sources []int, circular bool) *Feature {
	t := PatternTypes[0]
	if circular {
		t = PatternTypes[1]
	}
	f := withInputs(&Feature{Kind: KPattern, Targets: targets, Sources: sources,
		Inputs: map[string]string{"Type": t, "Direction": "X", "Direction 2": "None", "Axis": "Z", "Operation": CopyModes[0]}},
		LinearInputs, CircularInputs)
	return f
}

// NewFillet rounds (or chamfers) edges.
func NewFillet(edges []EdgeRef, chamfer bool) *Feature {
	if chamfer {
		return withInputs(&Feature{Kind: KChamfer, Edges: edges}, ChamferInputs)
	}
	return withInputs(&Feature{Kind: KFillet, Edges: edges}, FilletInputs)
}

// NewHole drills holes at points of a sketch on a face.
func NewHole(sketchID int, points []int) *Feature {
	return withInputs(&Feature{Kind: KHole, Ref: sketchID, Points: points,
		Inputs: map[string]string{"Type": HoleTypes[0], "Extent": HoleExtents[0], "Bottom": HoleBottoms[0]}}, HoleInputs)
}

// NewShell hollows a body, its open faces left open.
// Combine operations and inputs.
var (
	CombineOps    = []string{"Join", "Cut", "Intersect"}
	CombineInputs = [][2]string{{"Operation", "Join"}, {"Keep tools", "0"}}
)

// NewCombine joins, cuts or intersects bodies (Targets[1:], the tools)
// with a target body (Targets[0]).
func NewCombine(target int, tools []int) *Feature {
	return withInputs(&Feature{Kind: KCombine, Targets: append([]int{target}, tools...)}, CombineInputs)
}

func NewShell(target int, open []FaceRef) *Feature {
	return withInputs(&Feature{Kind: KShell, Targets: []int{target}, Faces: open}, ShellInputs)
}

func fmtNum(x float64) string {
	s := fmt.Sprintf("%.4f", x)
	for len(s) > 1 && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}
	if s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	if s == "-0" {
		s = "0"
	}
	return s
}

// modifies: features that change the bodies already there rather than
// adding tool bodies of their own.
func (f *Feature) modifies() bool {
	switch f.Kind {
	case KMove, KMirror, KPattern, KFillet, KChamfer, KShell, KJoint, KEmboss, KCombine:
		return true
	}
	return false
}

// MoveXform is a move feature's transform: turned about the pivot by
// Rotate X, then Y, then Z, then moved.
func (d *Document) MoveXform(f *Feature) (geom.Xform, error) {
	var v [9]float64
	for i, kv := range MoveInputs {
		x, err := d.input(f, kv[0])
		if err != nil {
			return geom.Identity, err
		}
		v[i] = x
	}
	pivot := geom.V3(v[6], v[7], v[8])
	deg := math.Pi / 180
	x := geom.Rotate(pivot, geom.XAxis, v[3]*deg).
		Then(geom.Rotate(pivot, geom.YAxis, v[4]*deg)).
		Then(geom.Rotate(pivot, geom.ZAxis, v[5]*deg)).
		Then(geom.Translate(geom.V3(v[0], v[1], v[2])))
	return x, nil
}

func axisVec(name string) geom.Vec3 {
	switch name {
	case "X":
		return geom.XAxis
	case "Y":
		return geom.YAxis
	}
	return geom.ZAxis
}

// instances are a pattern's transforms, the original's (identity) left out.
func (d *Document) instances(f *Feature) ([]geom.Xform, error) {
	n, err := d.input(f, "Count")
	if err != nil {
		return nil, err
	}
	count := int(math.Round(n))
	if count < 1 || count > 1000 {
		return nil, fmt.Errorf("Count must be 1 to 1000")
	}
	var out []geom.Xform
	if f.Inputs["Type"] == "Circular" {
		ang, err := d.input(f, "Angle")
		if err != nil {
			return nil, err
		}
		var c geom.Vec3
		for i, k := range []string{"Center X", "Center Y", "Center Z"} {
			x, err := d.input(f, k)
			if err != nil {
				return nil, err
			}
			switch i {
			case 0:
				c.X = x
			case 1:
				c.Y = x
			default:
				c.Z = x
			}
		}
		dir := axisVec(f.Inputs["Axis"])
		if f.AxisFace != nil {
			fc := d.FindFace(f.AxisFace.Feature, f.AxisFace.Body, f.AxisFace.Face)
			if fc == nil {
				return nil, fmt.Errorf("its axis face is gone")
			}
			fr, ok := axisOfFace(fc)
			if !ok {
				return nil, fmt.Errorf("its axis face isn't round")
			}
			c, dir = fr.O, fr.Z
		}
		step := ang * math.Pi / 180 / float64(count)
		if math.Abs(math.Abs(ang)-360) > 1e-9 && count > 1 {
			step = ang * math.Pi / 180 / float64(count-1)
		}
		for k := 1; k < count; k++ {
			out = append(out, geom.Rotate(c, dir, step*float64(k)))
		}
		return out, nil
	}
	sp, err := d.input(f, "Spacing")
	if err != nil {
		return nil, err
	}
	dir := axisVec(f.Inputs["Direction"])
	var dir2 geom.Vec3
	count2 := 1
	sp2 := 0.0
	if a2 := f.Inputs["Direction 2"]; a2 != "" && a2 != "None" {
		dir2 = axisVec(a2)
		n2, err := d.input(f, "Count 2")
		if err != nil {
			return nil, err
		}
		count2 = int(math.Round(n2))
		if count2 < 1 || count*count2 > 1000 {
			return nil, fmt.Errorf("Count 2 must be 1 or more (1000 in all)")
		}
		if sp2, err = d.input(f, "Spacing 2"); err != nil {
			return nil, err
		}
	}
	for j := 0; j < count2; j++ {
		for k := 0; k < count; k++ {
			if j == 0 && k == 0 {
				continue
			}
			out = append(out, geom.Translate(dir.Scale(sp*float64(k)).Add(dir2.Scale(sp2*float64(j)))))
		}
	}
	return out, nil
}

// axisOfFace is the axis frame (O on it, Z along it) of a cylinder, cone,
// sphere or torus face.
func axisOfFace(fc *brep.Face) (geom.Frame, bool) {
	switch s := fc.Surf.(type) {
	case geom.Cylinder:
		return s.F, true
	case geom.Cone:
		return s.F, true
	case geom.Torus:
		return s.F, true
	case geom.Sphere:
		return s.F, true
	}
	return geom.Frame{}, false
}

// mirrorXform is a mirror feature's plane as a reflection.
func (d *Document) mirrorXform(f *Feature) (geom.Xform, error) {
	if f.Plane == nil {
		return geom.Identity, fmt.Errorf("no mirror plane")
	}
	var fr geom.Frame
	if f.Plane.Std != "" {
		pl, ok := StdPlane(f.Plane.Std)
		if !ok {
			return geom.Identity, fmt.Errorf("unknown plane %q", f.Plane.Std)
		}
		fr = pl
	} else {
		fc := d.FindFace(f.Plane.Feature, f.Plane.Body, f.Plane.Face)
		if fc == nil {
			return geom.Identity, fmt.Errorf("its mirror face is gone")
		}
		pl, ok := PlaneOf(fc)
		if !ok {
			return geom.Identity, fmt.Errorf("its mirror face isn't flat")
		}
		fr = pl
	}
	return geom.Mirror(fr.O, fr.Z), nil
}

// opOf is the boolean a body-making feature does with its tools.
func opOf(f *Feature) string {
	if f.Kind == KHole {
		return "Cut"
	}
	if op := f.Inputs["Operation"]; op != "" {
		return op
	}
	return "New body"
}

func findRef(state []*BodyRef, key int) int {
	for i, r := range state {
		if r.Key == key {
			return i
		}
	}
	return -1
}

func prefixFaces(b *brep.Body, prefix string) {
	for _, fc := range b.Faces() {
		fc.Name = prefix + fc.Name
	}
}

// modify runs a feature that changes the bodies there are.
func (d *Document) modify(f *Feature, state []*BodyRef) ([]*BodyRef, error) {
	out := append([]*BodyRef(nil), state...)
	changed := func(i int, b *brep.Body) {
		r := out[i]
		out[i] = &BodyRef{Key: r.Key, Name: r.Name, Body: b, Owner: r.Owner, Gen: d.nextGen()}
	}
	fresh := 0
	tooMany := false
	makeRef := func(b *brep.Body) *BodyRef {
		name := fmt.Sprintf("%s #%d", f.Name, fresh+1)
		if fresh >= KeysPerFeature {
			tooMany = true
		}
		b.Name = name
		r := &BodyRef{Key: BodyKey(f.ID, fresh%KeysPerFeature), Name: name, Body: b, Owner: f.ID, Gen: d.nextGen()}
		fresh++
		return r
	}
	newBody := func(b *brep.Body) { out = append(out, makeRef(b)) }
	target := func(key int) (int, error) {
		i := findRef(out, key)
		if i < 0 {
			return -1, fmt.Errorf("its body is gone (joined into another, or rolled back)")
		}
		return i, nil
	}
	switch f.Kind {
	case KCombine:
		if len(f.Targets) < 2 {
			return nil, fmt.Errorf("pick a target body and at least one tool body")
		}
		ti, err := target(f.Targets[0])
		if err != nil {
			return nil, err
		}
		op := brep.Union
		switch f.Inputs["Operation"] {
		case "Cut":
			op = brep.Subtract
		case "Intersect":
			op = brep.Intersect
		}
		b := out[ti].Body
		var gone []int
		for k, key := range f.Targets[1:] {
			i, err := target(key)
			if err != nil {
				return nil, fmt.Errorf("tool %d: %v", k+1, err)
			}
			if i == ti {
				return nil, fmt.Errorf("tool %d is the target body itself", k+1)
			}
			r, err := brep.Boolean(b, out[i].Body, op)
			if err == brep.ErrEmpty {
				return nil, fmt.Errorf("nothing is left: the bodies don't overlap (tool %d)", k+1)
			}
			if err != nil {
				return nil, fmt.Errorf("tool %d: %v", k+1, err)
			}
			b = r
			gone = append(gone, i)
		}
		b.Name = out[ti].Body.Name
		changed(ti, b)
		if f.Inputs["Keep tools"] != "1" {
			// the tools are used up
			sort.Sort(sort.Reverse(sort.IntSlice(gone)))
			for _, i := range gone {
				out = append(out[:i], out[i+1:]...)
			}
		}
		return out, nil
	case KMove:
		x, err := d.MoveXform(f)
		if err != nil {
			return nil, err
		}
		if len(f.Targets) == 0 {
			return nil, fmt.Errorf("nothing to move")
		}
		for _, key := range f.Targets {
			i, err := target(key)
			if err != nil {
				return nil, err
			}
			if c := d.Feature(out[i].Comp); out[i].Comp != 0 && Grounded(c) {
				return nil, fmt.Errorf("%s is grounded", c.Name)
			}
			if err := d.scaleRef(out, i, f); err != nil {
				return nil, err
			}
			if err := d.moveRefs(out, []int{i}, x); err != nil {
				return nil, err
			}
		}
		return out, nil

	case KMirror, KPattern:
		var xs []geom.Xform
		if f.Kind == KMirror {
			x, err := d.mirrorXform(f)
			if err != nil {
				return nil, err
			}
			xs = []geom.Xform{x}
		} else {
			var err error
			if xs, err = d.instances(f); err != nil {
				return nil, err
			}
		}
		if len(f.Targets) == 0 && len(f.Sources) == 0 {
			return nil, fmt.Errorf("nothing to repeat: pick bodies or features")
		}
		tag := func(k int) string {
			if f.Kind == KMirror {
				return fmt.Sprintf("f%d/m/", f.ID)
			}
			return fmt.Sprintf("f%d/%d/", f.ID, k+1)
		}
		// features: their tools again, put in as they were
		for _, sid := range f.Sources {
			src := d.Feature(sid)
			if src == nil || d.Index(sid) > d.Index(f.ID) {
				return nil, fmt.Errorf("a feature it repeats is gone")
			}
			if !src.Active || src.Err != "" {
				return nil, fmt.Errorf("%s is suppressed, rolled back or failing", src.Name)
			}
			if src.modifies() || len(src.Bodies) == 0 {
				return nil, fmt.Errorf("%s isn't a feature that adds or cuts a shape", src.Name)
			}
			copies := make([][]*brep.Body, len(xs))
			for k, x := range xs {
				for _, t := range src.Bodies {
					c, err := brep.Transform(t, x, func(n string) string { return tag(k) + n })
					if err != nil {
						return nil, err
					}
					copies[k] = append(copies[k], c)
				}
			}
			var err error
			if out, err = d.applyCopies(opOf(src), copies, out, makeRef); err != nil {
				return nil, err
			}
		}
		// bodies: copies joined to them or as bodies of their own
		for _, key := range f.Targets {
			i, err := target(key)
			if err != nil {
				return nil, err
			}
			orig := out[i].Body
			acc := orig
			for k, x := range xs {
				c, err := brep.Transform(orig, x, func(n string) string { return tag(k) + n })
				if err != nil {
					return nil, err
				}
				if f.Inputs["Operation"] == "New body" {
					newBody(c)
					continue
				}
				if acc, err = brep.Boolean(acc, c, brep.Union); err != nil {
					return nil, err
				}
			}
			if acc != orig {
				acc.Name = orig.Name
				changed(i, acc)
			}
		}
		if tooMany {
			return nil, fmt.Errorf("more than %d new bodies", KeysPerFeature)
		}
		return out, nil

	case KFillet, KChamfer:
		chamfer := f.Kind == KChamfer
		in := "Radius"
		if chamfer {
			in = "Distance"
		}
		size, err := d.input(f, in)
		if err != nil {
			return nil, err
		}
		if len(f.Edges) == 0 {
			return nil, fmt.Errorf("no edges")
		}
		// the edges, by body
		type pick struct {
			e *brep.Edge
			k int
		}
		byBody := map[int][]pick{}
		var order []int
		for k, ref := range f.Edges {
			i, e := d.findEdge(out, ref)
			if e == nil {
				return nil, fmt.Errorf("edge %d (between %s and %s) is gone", k+1, shortName(ref.A), shortName(ref.B))
			}
			if _, ok := byBody[i]; !ok {
				order = append(order, i)
			}
			byBody[i] = append(byBody[i], pick{e, k})
		}
		for _, i := range order {
			picks := byBody[i]
			// carried past a vertex where another picked edge meets it at an
			// angle (the two then meet in a mitre); stopped square where it
			// runs on smoothly (a straight edge into the arc of a rounded corner)
			at := map[*brep.Vertex][]*brep.Edge{}
			for _, p := range picks {
				at[p.e.Verts[0]] = append(at[p.e.Verts[0]], p.e)
				at[p.e.Verts[1]] = append(at[p.e.Verts[1]], p.e)
			}
			corner := func(e *brep.Edge, v *brep.Vertex) bool {
				for _, o := range at[v] {
					if o != e && awayFrom(e, v).Dot(awayFrom(o, v)) > -0.999 {
						return true
					}
				}
				return false
			}
			b := out[i].Body
			var tools []*brep.Body
			var opsList []brep.BoolOp
			for _, p := range picks {
				ext := [2]bool{corner(p.e, p.e.Verts[0]), corner(p.e, p.e.Verts[1])}
				if p.e.Verts[0] == p.e.Verts[1] {
					ext = [2]bool{}
				}
				t, op, err := ops.EdgeTool(p.e, size, chamfer, p.k+1, ext)
				if err != nil {
					return nil, fmt.Errorf("edge %d: %v", p.k+1, err)
				}
				prefixFaces(t, fmt.Sprintf("f%d/", f.ID))
				tools = append(tools, t)
				opsList = append(opsList, op)
			}
			for k, t := range tools {
				r, err := brep.Boolean(b, t, opsList[k])
				if err != nil {
					return nil, fmt.Errorf("edge %d: %v", picks[k].k+1, err)
				}
				b = r
			}
			b.Name = out[i].Body.Name
			changed(i, b)
		}
		return out, nil

	case KShell:
		t, err := d.input(f, "Thickness")
		if err != nil {
			return nil, err
		}
		if len(f.Targets) != 1 {
			return nil, fmt.Errorf("pick one body")
		}
		i, err := target(f.Targets[0])
		if err != nil {
			return nil, err
		}
		b := out[i].Body
		open := map[*brep.Face]bool{}
		for k, r := range f.Faces {
			var fc *brep.Face
			for _, c := range b.Faces() {
				if c.Name == r.Face {
					fc = c
				}
			}
			if fc == nil {
				return nil, fmt.Errorf("open face %d (%s) isn't on the body any more", k+1, shortName(r.Face))
			}
			open[fc] = true
		}
		s, err := ops.Shell(b, open, t)
		if err != nil {
			return nil, err
		}
		for _, fc := range s.Faces() {
			if len(fc.Name) > 3 && fc.Name[:3] == "in/" {
				fc.Name = fmt.Sprintf("f%d/%s", f.ID, fc.Name)
			}
		}
		s.Name = b.Name
		changed(i, s)
		return out, nil

	case KJoint:
		return d.joint(f, out)
	case KEmboss:
		return d.emboss(f, out)
	}
	return nil, fmt.Errorf("unknown feature kind %q", f.Kind)
}

// shortName drops a face name's "f<ID>/".
func shortName(n string) string {
	for i := 0; i < len(n); i++ {
		if n[i] == '/' {
			return n[i+1:]
		}
	}
	return n
}

// findEdge finds the edge an EdgeRef means among the bodies: between faces
// named A and B, the nearest P.
func (d *Document) findEdge(state []*BodyRef, ref EdgeRef) (int, *brep.Edge) {
	bi, best := -1, (*brep.Edge)(nil)
	bd := math.Inf(1)
	for i, r := range state {
		for _, e := range r.Body.Edges() {
			if e.HE[0] == nil || e.HE[1] == nil {
				continue
			}
			a, b := e.HE[0].Loop.Face.Name, e.HE[1].Loop.Face.Name
			if !(a == ref.A && b == ref.B) && !(a == ref.B && b == ref.A) {
				continue
			}
			if dd := edgeDist(e, ref.P); dd < bd {
				bi, best, bd = i, e, dd
			}
		}
	}
	return bi, best
}

func edgeDist(e *brep.Edge, p geom.Vec3) float64 {
	d := math.Inf(1)
	ts := geom.SampleParams(e.Curve, e.T0, e.T1, 0.05, 1e9)
	for i := 0; i+1 < len(ts); i++ {
		a, b := e.Curve.Eval(ts[i]), e.Curve.Eval(ts[i+1])
		ab := b.Sub(a)
		s := 0.0
		if l2 := ab.Dot(ab); l2 > 0 {
			s = math.Max(0, math.Min(1, p.Sub(a).Dot(ab)/l2))
		}
		d = math.Min(d, a.Add(ab.Scale(s)).Dist(p))
	}
	return d
}

// RefForEdge makes the persistent reference of an edge, near p.
func RefForEdge(e *brep.Edge, p geom.Vec3) (EdgeRef, bool) {
	if e.HE[0] == nil || e.HE[1] == nil {
		return EdgeRef{}, false
	}
	return EdgeRef{A: e.HE[0].Loop.Face.Name, B: e.HE[1].Loop.Face.Name, P: p}, true
}

// FindEdgeNow finds an edge reference in the design as it stands.
func (d *Document) FindEdgeNow(ref EdgeRef) *brep.Edge {
	state := d.Bodies
	if d.regen {
		state = d.state
	}
	_, e := d.findEdge(state, ref)
	return e
}

// buildHole makes a hole feature's tools at its sketch's points.
func (d *Document) buildHole(f *Feature) error {
	sk := d.Feature(f.Ref)
	if sk == nil || sk.Kind != KSketch {
		return fmt.Errorf("its sketch is gone")
	}
	if !sk.Active {
		return fmt.Errorf("%s is suppressed or rolled back", sk.Name)
	}
	if len(f.Points) == 0 {
		return fmt.Errorf("no points: pick sketch points to drill at")
	}
	var spec ops.HoleSpec
	var err error
	if spec.Diameter, err = d.input(f, "Diameter"); err != nil {
		return err
	}
	switch f.Inputs["Type"] {
	case "Counterbore":
		if spec.BoreDiameter, err = d.input(f, "Bore diameter"); err != nil {
			return err
		}
		if spec.BoreDepth, err = d.input(f, "Bore depth"); err != nil {
			return err
		}
	case "Countersink":
		if spec.SinkDiameter, err = d.input(f, "Sink diameter"); err != nil {
			return err
		}
	}
	s := sk.Sketch
	pl := s.Plane
	through := f.Inputs["Extent"] == "Through all"
	var all geom.Box
	for _, r := range d.state {
		all.Union(r.Body.Bounds())
	}
	for k, pid := range f.Points {
		e := s.Ent(pid)
		if e == nil {
			return fmt.Errorf("point %d is gone from %s", k+1, sk.Name)
		}
		at := pl.At2(s.P(pid))
		h := spec
		if through {
			h.Depth = all.Diagonal() + at.Dist(all.Center()) + 1
		} else {
			if h.Depth, err = d.input(f, "Depth"); err != nil {
				return err
			}
			h.Tip = f.Inputs["Bottom"] != "Flat"
		}
		b, err := ops.HoleTool(at, pl.Z.Neg(), pl.X, h, k+1)
		if err != nil {
			return err
		}
		b.Name = f.Name
		f.Bodies = append(f.Bodies, b)
	}
	return nil
}

// awayFrom is the direction an edge leaves its vertex v in.
func awayFrom(e *brep.Edge, v *brep.Vertex) geom.Vec3 {
	sgn := 1.0
	if e.T1 < e.T0 {
		sgn = -1
	}
	if v == e.Verts[0] {
		return e.Curve.Deriv(e.T0).Norm().Scale(sgn)
	}
	return e.Curve.Deriv(e.T1).Norm().Scale(-sgn)
}

// MoveScale is a move feature's scale (1 when it has none: older files).
func (d *Document) MoveScale(f *Feature) (float64, error) {
	if s, ok := f.Inputs["Scale"]; !ok || s == "" {
		return 1, nil
	}
	s, err := d.input(f, "Scale")
	if err != nil {
		return 1, err
	}
	if s <= 0 {
		return 1, fmt.Errorf("Scale must be more than 0")
	}
	return s, nil
}

// scaleRef scales body i by a move feature's Scale about its pivot (before
// the feature turns and moves it).
func (d *Document) scaleRef(out []*BodyRef, i int, f *Feature) error {
	s, err := d.MoveScale(f)
	if err != nil || s == 1 {
		return err
	}
	var p [3]float64
	for k, name := range []string{"Pivot X", "Pivot Y", "Pivot Z"} {
		if p[k], err = d.input(f, name); err != nil {
			return err
		}
	}
	r := out[i]
	b, err := brep.Scale(r.Body, geom.V3(p[0], p[1], p[2]), s)
	if err != nil {
		return err
	}
	b.Name = r.Body.Name
	// a scaled component's body is no longer its design's: plain from here
	out[i] = &BodyRef{Key: r.Key, Name: r.Name, Body: b, Owner: r.Owner, Gen: d.nextGen()}
	return nil
}

// applyCopies puts a pattern's (or mirror's) copies of a feature's tools in
// with op. Copies standing apart from each other that each touch a body go
// in together, a single boolean for a group (a ring of fins in two or three
// instead of one each); a group that fails goes in again in halves.
func (d *Document) applyCopies(op string, copies [][]*brep.Body, out []*BodyRef, fresh func(*brep.Body) *BodyRef) ([]*BodyRef, error) {
	var tools []*brep.Body
	for _, c := range copies {
		tools = append(tools, c...)
	}
	one := func(ts []*brep.Body, out []*BodyRef) ([]*BodyRef, error) {
		for _, t := range ts {
			var err error
			if out, err = d.applyOp(op, []*brep.Body{t}, out, fresh); err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	if op == "New body" || len(tools) < 3 {
		return one(tools, out)
	}
	touches := func(t *brep.Body) bool {
		for _, r := range out {
			if overlaps(t.Bounds(), r.Body.Bounds()) {
				return true
			}
		}
		return false
	}
	var alone []*brep.Body
	var groups [][]*brep.Body
	var boxes [][]geom.Box
next:
	for _, t := range tools {
		if !touches(t) {
			alone = append(alone, t)
			continue
		}
		b := t.Bounds()
		gap := 1e-3 * (1 + b.Diagonal())
		for gi := range groups {
			clear := true
			for _, o := range boxes[gi] {
				if overlaps(grow(b, gap), o) {
					clear = false
					break
				}
			}
			if clear {
				groups[gi] = append(groups[gi], t)
				boxes[gi] = append(boxes[gi], b)
				continue next
			}
		}
		groups = append(groups, []*brep.Body{t})
		boxes = append(boxes, []geom.Box{b})
	}
	// a group that fails goes in as two halves, and so on down to the copy
	// that needs going in alone (with Boolean's retries at finer
	// tolerances, which a group would make slow)
	var put func(g []*brep.Body, out []*BodyRef) ([]*BodyRef, error)
	put = func(g []*brep.Body, out []*BodyRef) ([]*BodyRef, error) {
		if len(g) == 1 {
			return one(g, out)
		}
		if all, err := brep.Gather(g[0].Name, g...); err == nil {
			if res, err := d.applyOpWith(brep.BooleanOnce, op, []*brep.Body{all}, out, fresh); err == nil {
				return res, nil
			}
		}
		out, err := put(g[:len(g)/2], out)
		if err != nil {
			return nil, err
		}
		return put(g[len(g)/2:], out)
	}
	for _, g := range groups {
		var err error
		if out, err = put(g, out); err != nil {
			return nil, err
		}
	}
	return one(alone, out)
}

func grow(b geom.Box, m float64) geom.Box {
	d := geom.V3(m, m, m)
	return geom.Box{Min: b.Min.Sub(d), Max: b.Max.Add(d)}
}
