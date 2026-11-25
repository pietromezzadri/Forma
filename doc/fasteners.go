package doc

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	"forma/brep"
	"forma/geom"
)

// Fasteners (M20): ISO metric screws as components, generated from the
// standards' dimensions, and holes made to fit them. A screw is a
// component whose design is built from its inputs (Screw, Size, Length),
// so changing the size in its properties rebuilds it; as a component it
// takes joints, explodes and is counted in the bill of materials (two of
// the same are one row). Its frame: the axis along Z, the bearing face
// under the head at z = 0 (a countersunk screw: the top of its head), the
// shank down to z = −Length. The thread isn't modelled: the shank is a
// plain cylinder at the nominal diameter.

// ScrewStd is a kind of screw and its standard.
type ScrewStd struct {
	ID, Label string
	// head: socket (a hex key socket in the top) or hex (a hex head)
	Hex bool
	// Sunk: the head is countersunk (Length includes it)
	Sunk bool
	// Button: a domed head
	Button bool
	dims   map[string]headDims
	minLen map[string]float64
}

// headDims: head diameter (or a hex head's width across flats), head
// height, the hex key's size and the socket's depth.
type headDims struct{ dk, k, s, t float64 }

// metric: nominal diameter, coarse pitch, clearance holes (ISO 273 fine,
// medium, coarse), a socket head's counterbore diameter (DIN 974-1) and
// the longest standard length.
type metric struct {
	d, pitch            float64
	close, normal, loose float64
	cbore               float64
	maxLen              float64
}

// ScrewSizes are the sizes, M2 to M12.
var ScrewSizes = []string{"M2", "M2.5", "M3", "M4", "M5", "M6", "M8", "M10", "M12"}

var metrics = map[string]metric{
	"M2":   {2, 0.4, 2.2, 2.4, 2.6, 4.3, 20},
	"M2.5": {2.5, 0.45, 2.7, 2.9, 3.1, 5, 25},
	"M3":   {3, 0.5, 3.2, 3.4, 3.6, 6.5, 30},
	"M4":   {4, 0.7, 4.3, 4.5, 4.8, 8, 40},
	"M5":   {5, 0.8, 5.3, 5.5, 5.8, 10, 50},
	"M6":   {6, 1, 6.4, 6.6, 7, 11, 60},
	"M8":   {8, 1.25, 8.4, 9, 10, 15, 80},
	"M10":  {10, 1.5, 10.5, 11, 12, 18, 100},
	"M12":  {12, 1.75, 13, 13.5, 14.5, 20, 120},
}

// ScrewLengths are the standard lengths (each size uses those from its
// shortest to its longest).
var ScrewLengths = []float64{3, 4, 5, 6, 8, 10, 12, 16, 20, 25, 30, 35, 40, 45, 50, 55, 60, 65, 70, 80, 90, 100, 110, 120}

// ScrewStds are the kinds of screw.
var ScrewStds = []*ScrewStd{
	{ID: "ISO 4762", Label: "Socket head cap screw", dims: map[string]headDims{
		"M2": {3.8, 2, 1.5, 1}, "M2.5": {4.5, 2.5, 2, 1.1}, "M3": {5.5, 3, 2.5, 1.3},
		"M4": {7, 4, 3, 2}, "M5": {8.5, 5, 4, 2.5}, "M6": {10, 6, 5, 3},
		"M8": {13, 8, 6, 4}, "M10": {16, 10, 8, 5}, "M12": {18, 12, 10, 6}},
		minLen: map[string]float64{"M2": 3, "M2.5": 4, "M3": 4, "M4": 5, "M5": 6, "M6": 8, "M8": 10, "M10": 12, "M12": 16}},
	{ID: "ISO 7380", Label: "Button head screw", Button: true, dims: map[string]headDims{
		"M3": {5.7, 1.65, 2, 1.04}, "M4": {7.6, 2.2, 2.5, 1.3}, "M5": {9.5, 2.75, 3, 1.56},
		"M6": {10.5, 3.3, 4, 2.08}, "M8": {14, 4.4, 5, 2.6}, "M10": {17.5, 5.5, 6, 3.12}, "M12": {21, 6.6, 8, 4.16}},
		minLen: map[string]float64{"M3": 4, "M4": 6, "M5": 8, "M6": 10, "M8": 12, "M10": 16, "M12": 20}},
	{ID: "ISO 10642", Label: "Countersunk head screw", Sunk: true, dims: map[string]headDims{
		"M3": {6.72, 1.86, 2, 1.1}, "M4": {8.96, 2.48, 2.5, 1.5}, "M5": {11.2, 3.1, 3, 1.9},
		"M6": {13.44, 3.72, 4, 2.2}, "M8": {17.92, 4.96, 5, 3}, "M10": {22.4, 6.2, 6, 3.6}, "M12": {26.88, 7.44, 8, 4.3}},
		minLen: map[string]float64{"M3": 8, "M4": 8, "M5": 8, "M6": 8, "M8": 10, "M10": 12, "M12": 20}},
	{ID: "ISO 4017", Label: "Hex head screw", Hex: true, dims: map[string]headDims{
		"M2": {4, 1.4, 0, 0}, "M2.5": {5, 1.7, 0, 0}, "M3": {5.5, 2, 0, 0}, "M4": {7, 2.8, 0, 0},
		"M5": {8, 3.5, 0, 0}, "M6": {10, 4, 0, 0}, "M8": {13, 5.3, 0, 0}, "M10": {16, 6.4, 0, 0}, "M12": {18, 7.5, 0, 0}},
		minLen: map[string]float64{"M2": 4, "M2.5": 5, "M3": 6, "M4": 8, "M5": 10, "M6": 12, "M8": 16, "M10": 20, "M12": 25}},
}

// ScrewStdByID finds a kind of screw by its standard.
func ScrewStdByID(id string) *ScrewStd {
	for _, s := range ScrewStds {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// Sizes are the sizes the standard has.
func (s *ScrewStd) Sizes() []string {
	var out []string
	for _, z := range ScrewSizes {
		if _, ok := s.dims[z]; ok {
			out = append(out, z)
		}
	}
	return out
}

// Lengths are the standard lengths of a size.
func (s *ScrewStd) Lengths(size string) []float64 {
	m, ok := metrics[size]
	if !ok {
		return nil
	}
	var out []float64
	for _, l := range ScrewLengths {
		if l >= s.minLen[size] && l <= m.maxLen {
			out = append(out, l)
		}
	}
	return out
}

// ScrewSpec is one screw: its standard, size and length (mm).
type ScrewSpec struct {
	Std    string
	Size   string
	Length float64
	// Modelled: its thread is modelled (a helical solid of facets) instead
	// of a plain shank drawn with thread lines
	Modelled bool
}

// Label is how it is called in a parts list: "ISO 4762 M5×16".
func (s ScrewSpec) Label() string {
	return fmt.Sprintf("%s %s×%s", s.Std, s.Size, num(s.Length))
}

func (s ScrewSpec) check() (*ScrewStd, metric, headDims, error) {
	std := ScrewStdByID(s.Std)
	if std == nil {
		return nil, metric{}, headDims{}, fmt.Errorf("no screw standard %q", s.Std)
	}
	m, ok := metrics[s.Size]
	h, ok2 := std.dims[s.Size]
	if !ok || !ok2 {
		return nil, metric{}, headDims{}, fmt.Errorf("%s has no size %s (it has %s)", s.Std, s.Size, strings.Join(std.Sizes(), ", "))
	}
	if s.Length <= 0 || (std.Sunk && s.Length <= h.k) {
		return nil, metric{}, headDims{}, fmt.Errorf("the length must be more than %s mm", num(math.Max(0, map[bool]float64{true: h.k}[std.Sunk])))
	}
	return std, m, h, nil
}

// ScrewDesign builds the screw's own design.
func ScrewDesign(s ScrewSpec) (*Document, error) {
	std, m, h, err := s.check()
	if err != nil {
		return nil, err
	}
	d := New()
	in := func(f *Feature, kv ...any) {
		for i := 0; i+1 < len(kv); i += 2 {
			switch v := kv[i+1].(type) {
			case float64:
				f.Inputs[kv[i].(string)] = num(v)
			case string:
				f.Inputs[kv[i].(string)] = v
			}
		}
	}
	r := m.d / 2
	// a hexagon across flats af, centred on the axis, in sketch f
	hexagon := func(f *Feature, af float64) []int {
		s := f.Sketch
		R := af / math.Sqrt(3)
		var pts []int
		for k := 0; k < 6; k++ {
			a := float64(k) * math.Pi / 3
			pts = append(pts, s.AddPoint(geom.V2(R*math.Cos(a), R*math.Sin(a))).ID)
		}
		var ids []int
		for k := range pts {
			ids = append(ids, s.AddLine(pts[k], pts[(k+1)%6]).ID)
		}
		return ids
	}
	// a hex prism from z0 up by h, joined or cut
	hexPrism := func(name string, af, z0, h float64, op string) {
		sk := d.Insert(NewSketch(PlaneRef{Std: "xy"}))
		sk.Name = name + " sketch"
		sk.Inputs = map[string]string{"Offset": num(z0)}
		ids := hexagon(sk, af)
		e := d.Insert(NewExtrude([]ProfileRef{{Sketch: sk.ID, Outer: ids}}))
		e.Name = name
		in(e, "Distance", h, "Operation", op)
		sk.Hidden = true
	}
	switch {
	case std.Button:
		// a spherical cap through the head's rim and top
		R := (h.dk*h.dk/4 + h.k*h.k) / (2 * h.k)
		sph := d.Insert(NewPrim(KSphere))
		sph.Name = "Head"
		in(sph, "Radius", R, "X", 0.0, "Y", 0.0, "Z", h.k-R)
		cap := d.Insert(NewPrim(KCylinder))
		cap.Name = "Head rim"
		in(cap, "Radius", h.dk/2, "Height", h.k, "X", 0.0, "Y", 0.0, "Z", 0.0, "Operation", "Intersect")
	case std.Sunk:
		cone := d.Insert(NewPrim(KCone))
		cone.Name = "Head"
		in(cone, "Base radius", r, "Top radius", h.dk/2, "Height", h.k, "X", 0.0, "Y", 0.0, "Z", -h.k)
	case std.Hex:
		hexPrism("Head", h.dk, 0, h.k, "New body")
	default:
		head := d.Insert(NewPrim(KCylinder))
		head.Name = "Head"
		in(head, "Radius", h.dk/2, "Height", h.k, "X", 0.0, "Y", 0.0, "Z", 0.0)
	}
	top := 0.0
	if std.Sunk {
		top = -h.k
	}
	plain := s.Length + top // the shank under the head
	if s.Modelled {
		// the thread from the tip, the plain shank above it
		b := s.ThreadLength()
		th := d.Insert(NewPrim(KThread))
		th.Name = "Thread"
		in(th, "Diameter", m.d, "Pitch", m.pitch, "Height", b, "X", 0.0, "Y", 0.0, "Z", -s.Length, "Operation", "Join")
		plain -= b
	}
	if plain > 1e-9 {
		shank := d.Insert(NewPrim(KCylinder))
		shank.Name = "Shank"
		in(shank, "Radius", r, "Height", plain, "X", 0.0, "Y", 0.0, "Z", top-plain, "Operation", "Join")
	}
	if !std.Hex {
		// the hex key's socket, down from the head's top
		headTop := h.k
		if std.Sunk {
			headTop = 0
		}
		hexPrism("Socket", h.s, headTop-h.t, h.t+1, "Cut")
	}
	d.Regenerate()
	for _, f := range d.Features {
		if f.Err != "" {
			return nil, fmt.Errorf("%s: %s", f.Name, f.Err)
		}
	}
	if len(d.Bodies) != 1 {
		return nil, fmt.Errorf("the screw came out as %d bodies", len(d.Bodies))
	}
	d.Bodies[0].Name = s.Label()
	return d, nil
}

var screwCache sync.Map // spec → design data

// ScrewData is the screw's design, as a component holds it.
func ScrewData(s ScrewSpec) (string, error) {
	if v, ok := screwCache.Load(s); ok {
		return v.(string), nil
	}
	d, err := ScrewDesign(s)
	if err != nil {
		return "", err
	}
	data := string(d.Marshal())
	screwCache.Store(s, data)
	return data, nil
}

// NewScrew is a screw as a component.
func NewScrew(s ScrewSpec) (*Feature, error) {
	data, err := ScrewData(s)
	if err != nil {
		return nil, err
	}
	f := NewComponent(s.Label()+".forma", []byte(data))
	f.Name = s.Label()
	f.Inputs["Screw"], f.Inputs["Size"], f.Inputs["Length"] = s.Std, s.Size, num(s.Length)
	if s.Modelled {
		f.Inputs["Thread"] = "Modelled"
	}
	return f, nil
}

// ScrewOf is a screw component's spec (false for any other component).
func ScrewOf(f *Feature) (ScrewSpec, bool) {
	if f == nil || f.Kind != KComponent || f.Inputs["Screw"] == "" {
		return ScrewSpec{}, false
	}
	l, err := strconv.ParseFloat(f.Inputs["Length"], 64)
	if err != nil {
		return ScrewSpec{}, false
	}
	return ScrewSpec{Std: f.Inputs["Screw"], Size: f.Inputs["Size"], Length: l, Modelled: f.Inputs["Thread"] == "Modelled"}, true
}

// Holes for screws: a hole step's "Screw" input (a size) sizes it from the
// tables — its "Fit" a clearance hole (Close, Normal, Loose: ISO 273) or a
// tapped one (Tapped: the tap drill, nominal less one pitch) — and a
// counterbore to take a socket head cap screw's head (DIN 974-1, the head's
// height deep and 0.4 mm more), a countersink a countersunk head's.

// HoleFits are the hole fits for a screw.
var HoleFits = []string{"Close", "Normal", "Loose", "Tapped"}

// ScrewHoleSizes are the hole's diameters for a screw of size, fit fit:
// the hole, a counterbore's diameter and depth, a countersink's diameter.
func ScrewHoleSizes(size, fit string) (dia, bore, boreDepth, sink float64, err error) {
	m, ok := metrics[size]
	if !ok {
		return 0, 0, 0, 0, fmt.Errorf("no screw size %q", size)
	}
	switch fit {
	case "Close":
		dia = m.close
	case "", "Normal":
		dia = m.normal
	case "Loose":
		dia = m.loose
	case "Tapped":
		dia = math.Round((m.d-m.pitch)*100) / 100
	default:
		return 0, 0, 0, 0, fmt.Errorf("no hole fit %q", fit)
	}
	bore = m.cbore
	if h, ok := ScrewStdByID("ISO 4762").dims[size]; ok {
		boreDepth = h.k + 0.4
	}
	sink = 2 * m.d
	if h, ok := ScrewStdByID("ISO 10642").dims[size]; ok {
		sink = h.dk + 0.2
	}
	return dia, bore, boreDepth, sink, nil
}

// SortedLengths formats lengths for a list.
func SortedLengths(ls []float64) []string {
	sort.Float64s(ls)
	var out []string
	for _, l := range ls {
		out = append(out, num(l))
	}
	return out
}

// A screw in a hole: ScrewInHole joins a screw component to the hole whose
// rim is edge rim of body key — a revolute joint (the screw turns in its
// hole with the joint's Angle) from the screw's seat (its frame's origin,
// looking down the shank) to the hole's seat, so the screw follows the
// hole when the part moves or the hole is edited. The hole's seat is the
// rim's centre, looking out of the part; a counterbore's top rim seats it
// on the counterbore's floor.
func (d *Document) ScrewInHole(screw *Feature, key int, rim *brep.Edge) (*Feature, error) {
	if _, ok := ScrewOf(screw); !ok {
		return nil, fmt.Errorf("%s isn't a screw", screw.Name)
	}
	return d.FastenerInHole(screw, key, rim)
}

// HoleScrew is the screw size the hole step that made an edge's faces was
// made for ("" if none).
func (d *Document) HoleScrew(e *brep.Edge) string {
	if f := d.holeStepOf(e); f != nil {
		return f.Inputs["Screw"]
	}
	return ""
}

// holeStepOf is the hole step that made a face beside the edge.
func (d *Document) holeStepOf(e *brep.Edge) *Feature {
	for _, h := range e.HE {
		if h == nil {
			continue
		}
		if f := d.Feature(nameFeature(h.Loop.Face.Name)); f != nil && f.Kind == KHole {
			return f
		}
	}
	return nil
}

// holeSeat is where a screw put in a rim sits: below a counterbore's top
// rim, the counterbore's floor — the nearest round edge of the same hole
// step on the rim's axis, deeper in, beside a flat face looking the same
// way; else the rim itself.
func (d *Document) holeSeat(b *brep.Body, rim *brep.Edge) *brep.Edge {
	step := d.holeStepOf(rim)
	if step == nil {
		return rim
	}
	c := rim.Curve.(geom.Circle)
	n, _ := rimNormal(rim)
	best, bd := rim, math.Inf(1)
	for _, e := range b.Edges() {
		ec, ok := e.Curve.(geom.Circle)
		if !ok || e == rim || d.holeStepOf(e) != step {
			continue
		}
		off := ec.F.O.Sub(c.F.O)
		depth := -off.Dot(n)
		if depth < 1e-6 || off.Add(n.Scale(depth)).Len() > 1e-6 || ec.R > c.R+1e-9 {
			continue
		}
		en, ok := rimNormal(e)
		if !ok || en.Dot(n) < 1-1e-6 {
			continue
		}
		if depth < bd {
			best, bd = e, depth
		}
	}
	return best
}

// ThreadLength is how much of the shank is threaded, from the tip: b = 2d +
// 12 on socket and countersunk heads (all of it when the screw is shorter
// than that), all of it on button and hex heads.
func (s ScrewSpec) ThreadLength() float64 {
	std, m, h, err := s.check()
	if err != nil {
		return 0
	}
	shank := s.Length
	if std.Sunk {
		shank -= h.k
	}
	if s.Std == "ISO 4762" || s.Std == "ISO 10642" {
		return math.Min(shank, 2*m.d+12)
	}
	return shank
}
