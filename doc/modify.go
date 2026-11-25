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
	ChamferInputs  = [][2]string{{"Type", "Equal distance"}, {"Distance", "2"}, {"Distance 2", "1"}, {"Angle", "30"}, {"Swap", "0"}}
	HoleInputs     = [][2]string{{"Diameter", "6"}, {"Depth", "10"}, {"Bore diameter", "11"}, {"Bore depth", "4"}, {"Sink diameter", "12"}}
	ShellInputs    = [][2]string{{"Thickness", "1"}}
)

// Choices for the modelling features' combo inputs.
var (
	PatternTypes = []string{"Linear", "Circular"}
	Axes         = []string{"X", "Y", "Z"}
	Axes2        = []string{"None", "X", "Y", "Z"}
	CopyModes    = []string{"Join", "New body"}
	HoleTypes    = []string{"Simple", "Counterbore", "Countersink"}
	// a chamfer: the same distance back along both faces, two distances (the
	// first along one face of each edge, the second along the other; Swap
	// swaps them), or a distance and the angle from that face
	ChamferTypes  = []string{"Equal distance", "Two distances", "Distance and angle"}
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

// NewFillet rounds (or chamfers) edges, carrying on along the edges
// running on smoothly from them ("Tangent chain"; steps from before it
// was there don't, so they keep their shape).
func NewFillet(edges []EdgeRef, chamfer bool) *Feature {
	chain := map[string]string{"Tangent chain": "1"}
	if chamfer {
		return withInputs(&Feature{Kind: KChamfer, Edges: edges, Inputs: chain}, ChamferInputs)
	}
	return withInputs(&Feature{Kind: KFillet, Edges: edges, Inputs: chain}, FilletInputs)
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

// Split's inputs: how far the plane is moved along its normal, and which
// side is kept (both: the body keeps the front, the back is a new body).
var (
	SplitKeeps  = []string{"Both", "Front", "Back"}
	SplitInputs = [][2]string{{"Offset", "0"}, {"Keep", "Both"}}
)

// NewSplit cuts a body in two by a plane (a standard one or a flat face).
func NewSplit(target int, plane PlaneRef) *Feature {
	return withInputs(&Feature{Kind: KSplit, Targets: []int{target}, Plane: &plane}, SplitInputs)
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
	case KMove, KMirror, KPattern, KFillet, KChamfer, KShell, KJoint, KEmboss, KCombine, KSplit:
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
	fr, err := d.featurePlane(f, "mirror")
	if err != nil {
		return geom.Identity, err
	}
	return geom.Mirror(fr.O, fr.Z), nil
}

// featurePlane is the plane a mirror or split feature names: a standard
// one, or a flat face (what names it, in its messages).
func (d *Document) featurePlane(f *Feature, what string) (geom.Frame, error) {
	if f.Plane == nil {
		return geom.Frame{}, fmt.Errorf("no %s plane", what)
	}
	if f.Plane.Std != "" {
		pl, ok := StdPlane(f.Plane.Std)
		if !ok {
			return geom.Frame{}, fmt.Errorf("unknown plane %q", f.Plane.Std)
		}
		return pl, nil
	}
	fc := d.FindFace(f.Plane.Feature, f.Plane.Body, f.Plane.Face)
	if fc == nil {
		return geom.Frame{}, fmt.Errorf("its %s face is gone", what)
	}
	pl, ok := PlaneOf(fc)
	if !ok {
		return geom.Frame{}, fmt.Errorf("its %s face isn't flat", what)
	}
	return pl, nil
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
		// each edge set's size (set 0: the step's own Radius / Distance)
		sizes := map[int]float64{}
		for _, r := range f.Edges {
			if _, ok := sizes[r.Set]; ok {
				continue
			}
			v, err := d.input(f, SizeKey(f, r.Set))
			if err != nil {
				return nil, err
			}
			sizes[r.Set] = v
		}
		if len(f.Edges) == 0 {
			return nil, fmt.Errorf("no edges")
		}
		// the edges, by body
		byBody := map[int][]edgePick{}
		var order []int
		for k, ref := range f.Edges {
			i, e := d.findEdge(out, ref)
			if e == nil {
				return nil, fmt.Errorf("edge %d (between %s and %s) is gone", k+1, shortName(ref.A), shortName(ref.B))
			}
			if _, ok := byBody[i]; !ok {
				order = append(order, i)
			}
			byBody[i] = append(byBody[i], edgePick{e, k, sizes[ref.Set], ref, false})
		}
		for _, i := range order {
			picks := byBody[i]
			b := out[i].Body
			// the sizes, largest first: one pass if they're all one size; else
			// each size a pass of its own, largest first, so smaller rounds
			// meet the curved faces the larger ones left (their mitres would
			// touch them along lines, which the kernel can't sort out), and from
			// the second pass on carry round the arcs those rounds left on the
			// faces (an upright edge rounded first leaves the top face a curved
			// corner the top edge's round follows): Fusion's result
			var ladder []float64
			for _, p := range picks {
				found := false
				for _, s := range ladder {
					if math.Abs(s-p.size) <= 1e-9*(1+s) {
						found = true
					}
				}
				if !found {
					ladder = append(ladder, p.size)
				}
			}
			sort.Sort(sort.Reverse(sort.Float64Slice(ladder)))
			// tangent chains: a round carries on along the edges running on
			// smoothly from the picked ones (new steps; older ones only did so
			// for the arcs a mixed-size step's larger rounds left)
			follow := f.Inputs["Tangent chain"] == "1"
			extra := len(f.Edges) // the extra edges' numbers, after the step's own
			for pass := 0; pass < len(ladder); pass++ {
				s := ladder[pass]
				same := func(x float64) bool { return math.Abs(x-s) <= 1e-9*(1+s) }
				// this pass's edges and the later passes', found again on the
				// body as the earlier passes left it
				var now []edgePick
				later := map[*brep.Edge]bool{}
				for _, p := range picks {
					if p.size > s && !same(p.size) {
						continue // rounded in an earlier pass
					}
					_, e := d.findEdge([]*BodyRef{{Body: b}}, p.ref)
					if e == nil {
						return nil, fmt.Errorf("%s is gone after the larger rounds", p.label())
					}
					if same(p.size) {
						now = append(now, edgePick{e, p.k, p.size, p.ref, false})
					} else if p.size < s {
						later[e] = true
					}
				}
				if pass > 0 || follow {
					in := map[*brep.Edge]bool{}
					for _, p := range now {
						in[p.e] = true
					}
					for _, p := range append([]edgePick(nil), now...) {
						for _, ce := range brep.TangentChain(b, p.e) {
							// only edges the kernel can round (a flat face on one side:
							// one between two curved faces stops the chain there)
							if in[ce] || later[ce] || !roundable(ce) {
								continue
							}
							ref, ok := RefForEdge(ce, ce.Curve.Eval((ce.T0+ce.T1)/2))
							if !ok {
								continue
							}
							in[ce] = true
							now = append(now, edgePick{ce, extra, s, ref, true})
							extra++
						}
					}
				}
				var err error
				if b, err = d.roundEdges(f, b, now, chamfer); err != nil {
					return nil, err
				}
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

	case KSplit:
		if len(f.Targets) != 1 {
			return nil, fmt.Errorf("pick one body to split")
		}
		i, err := target(f.Targets[0])
		if err != nil {
			return nil, err
		}
		pl, err := d.featurePlane(f, "split")
		if err != nil {
			return nil, err
		}
		off, err := d.input(f, "Offset")
		if err != nil {
			return nil, err
		}
		pl.O = pl.O.Add(pl.Z.Scale(off))
		front, back, err := ops.SplitBody(out[i].Body, pl, fmt.Sprintf("f%d/split", f.ID))
		if err != nil {
			return nil, err
		}
		switch f.Inputs["Keep"] {
		case "Front":
			changed(i, front)
		case "Back":
			changed(i, back)
		default:
			// the body keeps what is in front; the back is a body of its own
			changed(i, front)
			newBody(back)
		}
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
	spec, err := d.holeSpec(f)
	if err != nil {
		return err
	}
	s := sk.Sketch
	f.Threads = nil
	var thread *ops.ThreadSpec
	if ModelledThread(f) {
		// drilled a hair under the thread's crests, then the thread cut
		m := metrics[f.Inputs["Screw"]]
		thread = &ops.ThreadSpec{D: m.d, P: m.pitch, Internal: true}
		spec.Diameter = 2 * (thread.Crest() - 0.01)
	}
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
		if thread != nil {
			// from over the face to through the far side (as far as the
			// design reaches along the hole), or a pitch short of the depth
			dir := pl.Z.Neg()
			t := *thread
			t.F = geom.FrameZ(at, dir, pl.X)
			t.Z0 = -math.Max(1, 2*t.P)
			if through {
				far := 0.0
				for i := 0; i < 8; i++ {
					c := geom.V3(pickBox(i&1, all.Min.X, all.Max.X), pickBox(i&2, all.Min.Y, all.Max.Y), pickBox(i&4, all.Min.Z, all.Max.Z))
					far = math.Max(far, c.Sub(at).Dot(dir))
				}
				t.Z1 = far + 2*t.P
			} else {
				t.Z1 = h.Depth - t.P
			}
			if t.Z1 > t.P {
				rod, err := ops.ThreadedRod(t)
				if err != nil {
					return err
				}
				rod.Name = f.Name
				f.Bodies = append(f.Bodies, rod)
				f.Threads = append(f.Threads, t)
			}
		}
	}
	return nil
}

// ModelledThread: a hole step tapped for a screw, its thread modelled.
func ModelledThread(f *Feature) bool {
	_, ok := metrics[f.Inputs["Screw"]]
	return ok && f.Inputs["Fit"] == "Tapped" && f.Inputs["Thread"] == "Modelled"
}

func pickBox(bit int, a, b float64) float64 {
	if bit != 0 {
		return b
	}
	return a
}

// holeSpec is a hole step's diameters (not its depth).
func (d *Document) holeSpec(f *Feature) (ops.HoleSpec, error) {
	var spec ops.HoleSpec
	var err error
	if spec.Diameter, err = d.input(f, "Diameter"); err != nil {
		return spec, err
	}
	switch f.Inputs["Type"] {
	case "Counterbore":
		if spec.BoreDiameter, err = d.input(f, "Bore diameter"); err != nil {
			return spec, err
		}
		if spec.BoreDepth, err = d.input(f, "Bore depth"); err != nil {
			return spec, err
		}
	case "Countersink":
		if spec.SinkDiameter, err = d.input(f, "Sink diameter"); err != nil {
			return spec, err
		}
	}
	// made for a screw: its diameters from the tables (the fields left be)
	if size := f.Inputs["Screw"]; size != "" {
		dia, bore, boreDepth, sink, err := ScrewHoleSizes(size, f.Inputs["Fit"])
		if err != nil {
			return spec, err
		}
		spec.Diameter = dia
		switch f.Inputs["Type"] {
		case "Counterbore":
			spec.BoreDiameter, spec.BoreDepth = bore, boreDepth
		case "Countersink":
			spec.SinkDiameter = sink
		}
	}
	return spec, nil
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

// chamferSizes are a chamfer's distances back along the faces of e.HE[0]
// and e.HE[1], from its type: the same distance both ways; Distance along
// the face its reference names first (A) and Distance 2 along the other;
// or Distance along A and the angle from A. Swap trades the faces.
func (d *Document) chamferSizes(f *Feature, e *brep.Edge, ref EdgeRef, dist float64) (float64, float64, error) {
	typ := f.Inputs["Type"]
	if typ == "" || typ == ChamferTypes[0] {
		return dist, dist, nil
	}
	// a and b: along face A and face B
	a, b := dist, dist
	switch typ {
	case ChamferTypes[1]:
		v, err := d.input(f, "Distance 2")
		if err != nil {
			return 0, 0, err
		}
		b = v
	case ChamferTypes[2]:
		deg, err := d.input(f, "Angle")
		if err != nil {
			return 0, 0, err
		}
		// the angle is the chamfer's from face A; the sine rule gives the
		// same distance along B whichever of the edge's sides A is
		if b, err = ops.ChamferSecond(e, dist, deg*math.Pi/180); err != nil {
			return 0, 0, err
		}
	default:
		return 0, 0, fmt.Errorf("unknown chamfer type %q", typ)
	}
	if f.Inputs["Swap"] == "1" {
		a, b = b, a
	}
	if b <= geom.Tol {
		return 0, 0, fmt.Errorf("the second distance must be more than 0")
	}
	// the edge's first side is face A unless the reference names it B
	if e.HE[0] != nil && e.HE[0].Loop.Face.Name != ref.A {
		return b, a, nil
	}
	return a, b, nil
}

// edgePick is an edge a fillet or chamfer rounds: k its index among the
// step's edges (its tool's number), size its set's, ref its reference;
// extra: not one of the step's own, but running on smoothly from them.
type edgePick struct {
	e     *brep.Edge
	k     int
	size  float64
	ref   EdgeRef
	extra bool
}

func (p edgePick) label() string {
	if p.extra {
		return "an edge running on from them"
	}
	return fmt.Sprintf("edge %d", p.k+1)
}

// roundEdges rounds (or chamfers) picks on body b: ball corners where
// three of one size meet, mitres where picked edges meet at an angle,
// square ends where one runs on smoothly into another.
func (d *Document) roundEdges(f *Feature, b *brep.Body, picks []edgePick, chamfer bool) (*brep.Body, error) {
	// carried past a vertex where another picked edge meets it at an
	// angle (the two then meet in a mitre); stopped square where it
	// runs on smoothly (a straight edge into the arc of a rounded corner)
	sizeOf := map[*brep.Edge]float64{}
	for _, p := range picks {
		sizeOf[p.e] = p.size
	}
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
	var tools []*brep.Body
	var opsList []brep.BoolOp
	// ball corners: where three rounded edges between flat faces meet
	// (a fillet's, not a chamfer's), the corner is part of the rolling
	// ball, cut first; the edges' rounds stop where it begins
	setback := map[[2]any]float64{} // (edge, vertex) → how far short
	if !chamfer {
		var verts []*brep.Vertex
		for v, es := range at {
			if len(es) == 3 {
				verts = append(verts, v)
			}
		}
		sort.Slice(verts, func(a, b int) bool { return verts[a].ID < verts[b].ID })
		type ball struct {
			v   *brep.Vertex
			c   ops.Corner
			set [3]float64
			r   float64
		}
		var balls []ball
		for _, v := range verts {
			// a ball only where the three rounds are one size (else they
			// cross in a mitre)
			es := at[v]
			r0 := sizeOf[es[0]]
			if math.Abs(sizeOf[es[1]]-r0) > 1e-9*(1+r0) || math.Abs(sizeOf[es[2]]-r0) > 1e-9*(1+r0) {
				continue
			}
			c, ok := ops.BallCornerOf(b, v, at[v])
			if !ok {
				continue
			}
			_, set, err := c.CornerFrame(r0)
			if err != nil {
				continue // too sharp: left to mitre
			}
			balls = append(balls, ball{v, c, set, r0})
			for j, e := range at[v] {
				setback[[2]any{e, v}] = set[j]
			}
		}
		// an edge too short for the corners at its ends keeps mitres
		short := map[*brep.Vertex]bool{}
		for _, p := range picks {
			s := setback[[2]any{p.e, p.e.Verts[0]}] + setback[[2]any{p.e, p.e.Verts[1]}]
			if s > 0 && s >= p.e.Verts[0].P.Dist(p.e.Verts[1].P)-1e-6 {
				short[p.e.Verts[0]], short[p.e.Verts[1]] = true, true
			}
		}
		for k, bl := range balls {
			if short[bl.v] {
				for _, e := range at[bl.v] {
					delete(setback, [2]any{e, bl.v})
				}
				continue
			}
			t, err := ops.CornerTool(bl.c, bl.r, fmt.Sprintf("ball:%d", k+1))
			if err != nil {
				return nil, fmt.Errorf("a corner: %v", err)
			}
			prefixFaces(t, fmt.Sprintf("f%d/", f.ID))
			tools = append(tools, t)
			opsList = append(opsList, brep.Subtract)
		}
	}
	nCorners := len(tools)
	for _, p := range picks {
		ext := [2]bool{corner(p.e, p.e.Verts[0]), corner(p.e, p.e.Verts[1])}
		if p.e.Verts[0] == p.e.Verts[1] {
			ext = [2]bool{}
		}
		set := [2]float64{setback[[2]any{p.e, p.e.Verts[0]}], setback[[2]any{p.e, p.e.Verts[1]}]}
		for j := range set {
			if set[j] > 0 {
				ext[j] = false // the ball takes over there
			}
		}
		// a chamfer's two distances: the first along the face the edge's
		// reference names first (A), the second along the other (Swap: the
		// other way); EdgeToolSet takes them by the edge's own sides
		s1, s2 := p.size, p.size
		if chamfer {
			var err error
			if s1, s2, err = d.chamferSizes(f, p.e, p.ref, p.size); err != nil {
				return nil, fmt.Errorf("%s: %v", p.label(), err)
			}
		}
		t, op, err := ops.EdgeToolSet(p.e, s1, s2, chamfer, p.k+1, ext, set)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", p.label(), err)
		}
		prefixFaces(t, fmt.Sprintf("f%d/", f.ID))
		tools = append(tools, t)
		opsList = append(opsList, op)
	}
	// the ball corners first, in order
	for k := 0; k < nCorners; k++ {
		r, err := brep.Boolean(b, tools[k], opsList[k])
		if err != nil {
			return nil, fmt.Errorf("a ball corner: %v", err)
		}
		b = r
	}
	// then the edges; where mitred tools cross, one can trip the kernel
	// (two passing exactly through the same point of an edge) that goes
	// in cleanly once the others are in: it waits, and the rest go on
	pending := make([]int, 0, len(tools)-nCorners)
	for k := nCorners; k < len(tools); k++ {
		pending = append(pending, k)
	}
	for len(pending) > 0 {
		var later []int
		var firstErr error
		firstK := -1
		for _, k := range pending {
			r, err := brep.Boolean(b, tools[k], opsList[k])
			if err != nil {
				if firstErr == nil {
					firstErr, firstK = err, k
				}
				later = append(later, k)
				continue
			}
			b = r
		}
		if len(later) == len(pending) {
			return nil, fmt.Errorf("%s: %v", picks[firstK-nCorners].label(), firstErr)
		}
		pending = later
	}
	return b, nil
}

// roundable: an edge the kernel can round, with a flat face on at least
// one side (straight between two flat faces, or a circle where one meets a
// cylinder or cone).
func roundable(e *brep.Edge) bool {
	if e.HE[0] == nil || e.HE[1] == nil {
		return false
	}
	for _, h := range e.HE {
		if _, ok := h.Loop.Face.Surf.(geom.Plane); ok {
			return true
		}
	}
	return false
}
