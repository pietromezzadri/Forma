// Package doc is a Forma design: named parameters and an ordered history
// of features (primitives, sketches, extrudes, revolves) that regenerates
// into bodies. It has no OpenGL, so designs can be built and tested headless.
package doc

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"forma/brep"
	"forma/drawing"
	"forma/expr"
	"forma/geom"
	"forma/sketch"
)

// Version is the file format version.
const Version = 1

// Param is a named value other expressions can use.
type Param struct {
	Name  string  `json:"name"`
	Expr  string  `json:"expr"`
	Value float64 `json:"-"`
	Err   string  `json:"-"`
	// Text is a text parameter's value (its expression in quotes: "Luna")
	Text   string `json:"-"`
	IsText bool   `json:"-"`
}

// Feature kinds.
const (
	KBox       = "box"
	KCylinder  = "cylinder"
	KCone      = "cone"
	KSphere    = "sphere"
	KTorus     = "torus"
	KSketch    = "sketch"
	KExtrude   = "extrude"
	KRevolve   = "revolve"
	KMove      = "move"
	KMirror    = "mirror"
	KPattern   = "pattern"
	KFillet    = "fillet"
	KChamfer   = "chamfer"
	KHole      = "hole"
	KShell     = "shell"
	KImport    = "import"    // solids read from a STEP file
	KMesh      = "mesh"      // a reference mesh read from an STL file
	KComponent = "component" // another design inserted and placed
	KJoint     = "joint"     // moves a component onto another
	KEmboss    = "emboss"    // profiles raised, engraved or inlaid on a face
	KCombine   = "combine"   // bodies joined, cut or intersected with a target body
	KSplit     = "split"     // a body cut in two by a plane
)

// KeysPerFeature: a body's key is its feature's ID × KeysPerFeature + its
// index there (so a pattern may make that many new bodies).
const KeysPerFeature = 256

// BodyKey is the key of feature fid's k-th body.
func BodyKey(fid, k int) int { return fid*KeysPerFeature + k }

// FaceRef names a face by its persistent name, on the body feature Feature
// made (its Body-th) as the design stands where it is used.
type FaceRef struct {
	Feature int    `json:"feature"`
	Body    int    `json:"body,omitempty"`
	Face    string `json:"face"`
}

// EdgeRef names an edge by the faces either side of it, and a point near
// it to pick between edges the same two faces share.
type EdgeRef struct {
	A string    `json:"a"`
	B string    `json:"b"`
	P geom.Vec3 `json:"p"`
}

// PlaneRef is where a sketch lies: a standard plane ("xy", "xz", "yz") or a
// face of an earlier feature, found by its persistent name: on a flat face
// its plane, on a curved one the plane touching it at At.
type PlaneRef struct {
	Std     string     `json:"std,omitempty"`
	Feature int        `json:"feature,omitempty"`
	Body    int        `json:"body,omitempty"`
	Face    string     `json:"face,omitempty"`
	At      *geom.Vec3 `json:"at,omitempty"`
}

// ProfileRef picks a region of a sketch by the entities round it, so it is
// found again when the sketch changes (indices would shift).
type ProfileRef struct {
	Sketch int     `json:"sketch"`
	Outer  []int   `json:"outer"`
	Holes  [][]int `json:"holes,omitempty"`
	// Text, when set, is a text entity: every letter of it, whatever it
	// says (so editing the text carries on through)
	Text int `json:"text,omitempty"`
}

// AxisRef is a revolve's axis: the sketch's X or Y axis, or one of its lines.
type AxisRef struct {
	Kind string `json:"kind"` // "x", "y", "line"
	Line int    `json:"line,omitempty"`
}

// Feature is one step of the design.
type Feature struct {
	ID       int               `json:"id"`
	Name     string            `json:"name"`
	Kind     string            `json:"kind"`
	Inputs   map[string]string `json:"inputs,omitempty"`
	Sketch   *sketch.Sketch    `json:"sketch,omitempty"`
	Plane    *PlaneRef         `json:"plane,omitempty"`
	Profiles []ProfileRef      `json:"profiles,omitempty"`
	Axis     *AxisRef          `json:"axis,omitempty"`
	Targets  []int             `json:"targets,omitempty"` // bodies (keys) a move, mirror, pattern or shell works on
	Sources  []int             `json:"sources,omitempty"` // features a pattern or mirror repeats
	Edges    []EdgeRef         `json:"edges,omitempty"`   // a fillet's or chamfer's edges
	Faces    []FaceRef         `json:"faces,omitempty"`   // a shell's open faces
	AxisFace *FaceRef          `json:"axisFace,omitempty"`
	Ref      int               `json:"ref,omitempty"`    // a hole's sketch
	Points   []int             `json:"points,omitempty"` // its points
	Data     string            `json:"data,omitempty"`   // an import's file (STEP text, STL as base64)
	Joint    []JointRef        `json:"joint,omitempty"`  // a joint's fixed and moving ends
	Circuit  *Circuit          `json:"circuit,omitempty"`
	Hidden   bool              `json:"hidden,omitempty"`
	XRay     bool              `json:"xray,omitempty"`  // its bodies drawn see-through
	Color    string            `json:"color,omitempty"` // its bodies' colour, "#rrggbb" ("": the default grey)
	Off      bool              `json:"suppressed,omitempty"`

	// results of the last regeneration
	Bodies  []*brep.Body    `json:"-"`
	Regions []sketch.Region `json:"-"`
	Err     string          `json:"-"`
	Warn    string          `json:"-"` // what an import left out (a joint: held at a limit)
	RefMesh *brep.Mesh      `json:"-"` // a mesh feature's triangles
	Local   []*brep.Body    `json:"-"` // a component's bodies in its own frame
	Place   geom.Xform      `json:"-"` // and where it put them
	Active  bool            `json:"-"` // before the rollback marker and not suppressed
	After   []*BodyRef      `json:"-"` // the design's bodies after it
	Gen     int             `json:"-"` // bumped whenever the results change
	// a joint: the component (or body's feature) it moved, and its axis
	// (where the ends met, and the fixed end's Z) in the design
	Moved     int        `json:"-"`
	JointAxis geom.Frame `json:"-"`
	sig       uint64
}

// Document is a design.
type Document struct {
	Version  int        `json:"version"`
	Params   []*Param   `json:"params"`
	Features []*Feature `json:"features"`
	// Rollback is how many features are in play (the timeline marker sits
	// after the Rollback-th); -1 means all of them.
	Rollback int `json:"rollback"`
	NextID   int `json:"next"`
	// Drawings are the design's drawing sheets (M16): made from its
	// bodies whenever they are shown, so only what is on them is kept.
	Drawings []*drawing.Sheet `json:"drawings,omitempty"`

	// Bodies are the design's bodies after the last feature in play.
	Bodies []*BodyRef `json:"-"`

	vars  map[string]float64
	texts map[string]string
	state []*BodyRef // the bodies so far, while regenerating
	regen bool
	gen   int

	subCache map[uint64]subDesign // components' designs, regenerated
}

func New() *Document {
	return &Document{Version: Version, Rollback: -1}
}

func (d *Document) id() int { d.NextID++; return d.NextID }

// Feature finds one by ID.
func (d *Document) Feature(id int) *Feature {
	for _, f := range d.Features {
		if f.ID == id {
			return f
		}
	}
	return nil
}

// Index of a feature in the history (-1 if none).
func (d *Document) Index(id int) int {
	for i, f := range d.Features {
		if f.ID == id {
			return i
		}
	}
	return -1
}

// Active is how many features are before the rollback marker.
func (d *Document) ActiveCount() int {
	if d.Rollback < 0 || d.Rollback > len(d.Features) {
		return len(d.Features)
	}
	return d.Rollback
}

// uniqueName numbers a name past those already used.
func (d *Document) uniqueName(base string) string {
	used := map[string]bool{}
	for _, f := range d.Features {
		used[f.Name] = true
	}
	for i := 1; ; i++ {
		n := fmt.Sprintf("%s %d", base, i)
		if !used[n] {
			return n
		}
	}
}

var kindLabel = map[string]string{KBox: "Box", KCylinder: "Cylinder", KCone: "Cone", KSphere: "Sphere",
	KTorus: "Torus", KSketch: "Sketch", KExtrude: "Extrude", KRevolve: "Revolve", KMove: "Move",
	KMirror: "Mirror", KPattern: "Pattern", KFillet: "Fillet", KChamfer: "Chamfer", KHole: "Hole", KShell: "Shell", KEmboss: "Emboss", KCombine: "Combine", KSplit: "Split",
	KImport: "Import", KMesh: "Mesh", KComponent: "Component", KJoint: "Joint", KCircuit: "Electronics"}

// KindLabel names a kind for the UI.
func KindLabel(k string) string { return kindLabel[k] }

// Insert adds a feature at the rollback marker (the end when it is at the
// end) and moves the marker past it.
func (d *Document) Insert(f *Feature) *Feature {
	f.ID = d.id()
	if f.Name == "" {
		f.Name = d.uniqueName(kindLabel[f.Kind])
	}
	if f.Inputs == nil {
		f.Inputs = map[string]string{}
	}
	at := d.ActiveCount()
	d.Features = append(d.Features, nil)
	copy(d.Features[at+1:], d.Features[at:])
	d.Features[at] = f
	if d.Rollback >= 0 {
		d.Rollback++
		if d.Rollback >= len(d.Features) {
			d.Rollback = -1
		}
	}
	return f
}

// Remove deletes a feature; features that used it fail until fixed.
func (d *Document) Remove(id int) {
	i := d.Index(id)
	if i < 0 {
		return
	}
	d.Features = append(d.Features[:i], d.Features[i+1:]...)
	if d.Rollback > i {
		d.Rollback--
	}
}

// Dependents lists the features that use feature id.
func (d *Document) Dependents(id int) []*Feature {
	var out []*Feature
	for _, f := range d.Features {
		for _, dep := range f.deps() {
			if dep == id {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

func (f *Feature) deps() []int {
	var out []int
	if f.Plane != nil && f.Plane.Feature != 0 {
		out = append(out, f.Plane.Feature)
	}
	for _, p := range f.Profiles {
		out = append(out, p.Sketch)
	}
	for _, k := range f.Targets {
		out = append(out, k/KeysPerFeature)
	}
	out = append(out, f.Sources...)
	for _, e := range f.Edges {
		out = append(out, nameFeature(e.A), nameFeature(e.B))
	}
	for _, r := range f.Faces {
		out = append(out, r.Feature)
	}
	if f.AxisFace != nil {
		out = append(out, f.AxisFace.Feature)
	}
	if f.Ref != 0 {
		out = append(out, f.Ref)
	}
	for _, j := range f.Joint {
		out = append(out, j.Feature)
		if j.Edge != nil {
			out = append(out, nameFeature(j.Edge.A), nameFeature(j.Edge.B))
		}
	}
	out = append(out, f.circuitDeps()...)
	return out
}

// nameFeature is the feature a qualified face name ("f12/top") is from.
func nameFeature(name string) int {
	var id int
	fmt.Sscanf(name, "f%d/", &id)
	return id
}

// PrimInputs are a primitive kind's inputs in display order, with defaults.
var PrimInputs = map[string][][2]string{
	KBox:      {{"Width", "40"}, {"Depth", "30"}, {"Height", "20"}, {"X", "0"}, {"Y", "0"}, {"Z", "0"}},
	KCylinder: {{"Radius", "12"}, {"Height", "30"}, {"X", "0"}, {"Y", "0"}, {"Z", "0"}},
	KCone:     {{"Base radius", "15"}, {"Top radius", "5"}, {"Height", "30"}, {"X", "0"}, {"Y", "0"}, {"Z", "0"}},
	KSphere:   {{"Radius", "15"}, {"X", "0"}, {"Y", "0"}, {"Z", "15"}},
	KTorus:    {{"Major radius", "25"}, {"Tube radius", "6"}, {"X", "0"}, {"Y", "0"}, {"Z", "6"}},
}

// Extrude and revolve inputs, in display order.
var (
	ExtrudeInputs = [][2]string{{"Distance", "20"}, {"Distance 2", "10"}}
	RevolveInputs = [][2]string{{"Angle", "360"}}
)

// Extrude directions (input "Direction").
var ExtrudeModes = []string{"One side", "Symmetric", "Two sides"}

// NewPrim makes a primitive feature with default inputs.
func NewPrim(kind string) *Feature {
	f := &Feature{Kind: kind, Inputs: map[string]string{}}
	for _, kv := range PrimInputs[kind] {
		f.Inputs[kv[0]] = kv[1]
	}
	return f
}

// NewSketch makes a sketch feature on a plane.
func NewSketch(pl PlaneRef) *Feature {
	return &Feature{Kind: KSketch, Plane: &pl, Sketch: sketch.New("", geom.World)}
}

// NewExtrude makes an extrude of profiles.
func NewExtrude(profiles []ProfileRef) *Feature {
	f := &Feature{Kind: KExtrude, Profiles: profiles, Inputs: map[string]string{"Direction": ExtrudeModes[0]}}
	for _, kv := range ExtrudeInputs {
		f.Inputs[kv[0]] = kv[1]
	}
	return f
}

// NewRevolve makes a revolve of profiles about an axis.
func NewRevolve(profiles []ProfileRef, axis AxisRef) *Feature {
	f := &Feature{Kind: KRevolve, Profiles: profiles, Axis: &axis, Inputs: map[string]string{}}
	for _, kv := range RevolveInputs {
		f.Inputs[kv[0]] = kv[1]
	}
	return f
}

// AddParam adds a parameter (a fresh name if name is empty).
func (d *Document) AddParam(name, ex string) *Param {
	if name == "" {
		used := map[string]bool{}
		for _, p := range d.Params {
			used[p.Name] = true
		}
		for i := 1; ; i++ {
			name = fmt.Sprintf("p%d", i)
			if !used[name] {
				break
			}
		}
	}
	p := &Param{Name: name, Expr: ex}
	d.Params = append(d.Params, p)
	return p
}

// Vars are the parameter values from the last regeneration.
func (d *Document) Vars() map[string]float64 { return d.vars }

// evalParams evaluates the parameters; one may use any other, so they are
// worked out in rounds until nothing more resolves (what is left is a cycle
// or an unknown name).
func (d *Document) evalParams() {
	vars := map[string]float64{}
	d.texts = map[string]string{}
	done := map[*Param]bool{}
	for _, p := range d.Params {
		p.Err = ""
		p.IsText = false
		if t, ok := quoted(p.Expr); ok && validName(p.Name) {
			p.Text, p.IsText = t, true
			d.texts[p.Name] = t
			done[p] = true
			continue
		}
		if !validName(p.Name) {
			p.Err = "not a name (letters, digits, _)"
			done[p] = true
		}
	}
	for round := 0; round <= len(d.Params); round++ {
		progress := false
		for _, p := range d.Params {
			if done[p] {
				continue
			}
			v, err := expr.Eval(p.Expr, vars)
			if err == nil {
				p.Value = v
				vars[p.Name] = v
				done[p] = true
				progress = true
			}
		}
		if !progress {
			break
		}
	}
	for _, p := range d.Params {
		if !done[p] {
			if _, err := expr.Eval(p.Expr, vars); err != nil {
				p.Err = err.Error()
			}
			if strings.Contains(p.Err, "unknown name") {
				p.Err += " (or a cycle)"
			}
		}
	}
	d.vars = vars
}

func validName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		ok := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9')
		if !ok {
			return false
		}
	}
	return s != "pi" && expr.Units[strings.ToLower(s)] == 0
}

// Eval evaluates an expression with the document's parameters.
func (d *Document) Eval(s string) (float64, error) { return expr.Eval(s, d.vars) }

func (d *Document) input(f *Feature, name string) (float64, error) {
	s, ok := f.Inputs[name]
	if !ok {
		return 0, fmt.Errorf("no %s", name)
	}
	v, err := expr.Eval(s, d.vars)
	if err != nil {
		return 0, fmt.Errorf("%s: %v", name, err)
	}
	return v, nil
}

// BodyName names a feature's k-th body.
func (f *Feature) BodyName(k int) string {
	if len(f.Bodies) <= 1 {
		return f.Name
	}
	return fmt.Sprintf("%s #%d", f.Name, k+1)
}

// FindFace finds a face by its persistent name on the body made by feature
// (its body-th), as the design stands (while regenerating: at that point of
// the timeline). A name may be given without its "f<ID>/" prefix. If that
// body is gone (joined into another) every body is searched.
func (d *Document) FindFace(feature, body int, name string) *brep.Face {
	state := d.Bodies
	if d.regen {
		state = d.state
	}
	match := func(fc *brep.Face) bool {
		return fc.Name == name || fc.Name == fmt.Sprintf("f%d/%s", feature, name)
	}
	key := BodyKey(feature, body)
	for _, pass := range []bool{true, false} {
		for _, r := range state {
			if pass && r.Key != key {
				continue
			}
			for _, fc := range r.Body.Faces() {
				if match(fc) {
					return fc
				}
			}
		}
	}
	return nil
}

// PlaneOf is the frame of a planar face, outward normal as Z, with its
// origin at the world origin's projection (so sketch coordinates on faces
// line up with the world's where they can).
func PlaneOf(fc *brep.Face) (geom.Frame, bool) {
	pl, ok := fc.Surf.(geom.Plane)
	if !ok {
		return geom.Frame{}, false
	}
	fr := pl.F
	if fc.Reversed {
		fr = fr.Flip()
	}
	l := fr.ToLocal(geom.Vec3{})
	fr.O = fr.ToWorld(geom.V3(l.X, l.Y, 0))
	return fr, true
}

// TangentPlane is the plane touching a curved face at the point of it
// nearest at, facing out of the body: on a face round an axis (cylinder,
// cone, sphere, torus) its X runs round the axis and its Y along it, so a
// text on it stands upright on an upright part.
func TangentPlane(fc *brep.Face, at geom.Vec3) (geom.Frame, bool) {
	u, v := fc.Surf.Project(at)
	p := fc.Surf.Eval(u, v)
	n := fc.OutwardNormal(u, v).Norm()
	if n.Len() < 0.5 {
		return geom.Frame{}, false
	}
	var axis geom.Vec3
	switch s := fc.Surf.(type) {
	case geom.Cylinder:
		axis = s.F.Z
	case geom.Cone:
		axis = s.F.Z
	case geom.Sphere:
		axis = s.F.Z
	case geom.Torus:
		axis = s.F.Z
	default:
		axis = geom.ZAxis
	}
	x := axis.Cross(n)
	if x.Len() < 1e-6 { // at a pole, or the axis along the normal
		x = n.AnyPerp()
	}
	x = x.Norm()
	return geom.Frame{O: p, X: x, Y: n.Cross(x), Z: n}, true
}

// Placed moves a sketch's plane by its feature's placement inputs: Offset
// along its normal, Turn about it (degrees), then Move X and Move Y along
// its (turned) axes.
func (d *Document) Placed(f *Feature, fr geom.Frame) (geom.Frame, error) {
	val := func(name string) (float64, error) {
		if s := strings.TrimSpace(f.Inputs[name]); s == "" || s == "0" {
			return 0, nil
		}
		return d.input(f, name)
	}
	off, err := val("Offset")
	if err != nil {
		return fr, err
	}
	turn, err := val("Turn")
	if err != nil {
		return fr, err
	}
	mx, err := val("Move X")
	if err != nil {
		return fr, err
	}
	my, err := val("Move Y")
	if err != nil {
		return fr, err
	}
	if turn != 0 {
		a := turn * math.Pi / 180
		c, s := math.Cos(a), math.Sin(a)
		x := fr.X.Scale(c).Add(fr.Y.Scale(s))
		fr.X, fr.Y = x, fr.Z.Cross(x)
	}
	fr.O = fr.O.Add(fr.Z.Scale(off)).Add(fr.X.Scale(mx)).Add(fr.Y.Scale(my))
	return fr, nil
}

// SketchPlacement are a sketch's placement inputs, in display order.
var SketchPlacement = []string{"Offset", "Turn", "Move X", "Move Y"}

// StdPlane is a standard sketch plane.
func StdPlane(name string) (geom.Frame, bool) {
	switch name {
	case "xy":
		return geom.World, true
	case "xz":
		return geom.Frame{X: geom.XAxis, Y: geom.ZAxis, Z: geom.YAxis.Neg()}, true
	case "yz":
		return geom.Frame{X: geom.YAxis, Y: geom.ZAxis, Z: geom.XAxis}, true
	}
	return geom.Frame{}, false
}

// RefFor makes the persistent reference of a region of a sketch.
func RefFor(sketchID int, r sketch.Region) ProfileRef {
	if r.Text != 0 && r.Ink {
		return ProfileRef{Sketch: sketchID, Outer: []int{r.Text}, Text: r.Text}
	}
	ref := ProfileRef{Sketch: sketchID, Outer: srcSet(r.Outer)}
	for _, h := range r.Holes {
		ref.Holes = append(ref.Holes, srcSet(h))
	}
	return ref
}

func srcSet(l []sketch.Seg) []int {
	m := map[int]bool{}
	for _, s := range l {
		m[s.Src] = true
	}
	var out []int
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

func jaccard(a, b []int) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	m := map[int]int{}
	for _, x := range a {
		m[x] |= 1
	}
	for _, x := range b {
		m[x] |= 2
	}
	both := 0
	for _, v := range m {
		if v == 3 {
			both++
		}
	}
	return float64(both) / float64(len(m))
}

// matchRegion finds the region a reference means: the best match of its
// outer loop's entities, then of its holes.
func matchRegion(ref ProfileRef, rs []sketch.Region) (int, bool) {
	best, bestScore := -1, 0.0
	for i, r := range rs {
		s := jaccard(ref.Outer, srcSet(r.Outer)) * 10
		var hs [][]int
		for _, h := range r.Holes {
			hs = append(hs, srcSet(h))
		}
		// holes: each ref hole's best match
		hscore := 0.0
		for _, rh := range ref.Holes {
			b := 0.0
			for _, h := range hs {
				b = math.Max(b, jaccard(rh, h))
			}
			hscore += b
		}
		if n := math.Max(float64(len(ref.Holes)), float64(len(hs))); n > 0 {
			s += hscore / n
		} else {
			s++
		}
		if s > bestScore {
			best, bestScore = i, s
		}
	}
	return best, best >= 0 && bestScore >= 5
}

// quoted reports a text parameter's expression ("…" or '…') and its text.
func quoted(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		return strings.ReplaceAll(s[1:len(s)-1], `\n`, "\n"), true
	}
	return "", false
}

// Texts are the design's text parameters by name.
func (d *Document) Texts() map[string]string { return d.texts }
