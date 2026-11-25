package doc

import (
	"fmt"
	"math"
	"sync"

	"forma/brep"
	"forma/geom"
	"forma/ops"
)

// Washers and nuts (M20), components like the screws: plain washers (ISO
// 7089, and the large series ISO 7093-1) and hex nuts (ISO 4032, thin ISO
// 4035), M2 to M12, built from the standards' tables. A component's inputs
// "Washer" or "Nut" (the standard) and "Size" make its design, so a size
// changed in its properties rebuilds it. Its frame: the axis along Z, the
// face it bears on at z = 0, the part rising to z = its thickness. The nut's
// thread isn't modelled: it is bored at the nominal diameter.

// WasherStd is a kind of washer: inner and outer diameter and thickness
// by size.
type WasherStd struct {
	ID, Label string
	dims      map[string][3]float64 // d1, d2, h
}

// NutStd is a kind of nut: width across flats and height by size.
type NutStd struct {
	ID, Label string
	dims      map[string][2]float64 // s, m
}

var WasherStds = []*WasherStd{
	{ID: "ISO 7089", Label: "Plain washer", dims: map[string][3]float64{
		"M2": {2.2, 5, 0.3}, "M2.5": {2.7, 6, 0.5}, "M3": {3.2, 7, 0.5}, "M4": {4.3, 9, 0.8},
		"M5": {5.3, 10, 1}, "M6": {6.4, 12, 1.6}, "M8": {8.4, 16, 1.6}, "M10": {10.5, 20, 2}, "M12": {13, 24, 2.5}}},
	{ID: "ISO 7093-1", Label: "Large washer", dims: map[string][3]float64{
		"M3": {3.2, 9, 0.8}, "M4": {4.3, 12, 1}, "M5": {5.3, 15, 1.2}, "M6": {6.4, 18, 1.6},
		"M8": {8.4, 24, 2}, "M10": {10.5, 30, 2.5}, "M12": {13, 37, 3}}},
}

var NutStds = []*NutStd{
	{ID: "ISO 4032", Label: "Hex nut", dims: map[string][2]float64{
		"M2": {4, 1.6}, "M2.5": {5, 2}, "M3": {5.5, 2.4}, "M4": {7, 3.2}, "M5": {8, 4.7},
		"M6": {10, 5.2}, "M8": {13, 6.8}, "M10": {16, 8.4}, "M12": {18, 10.8}}},
	{ID: "ISO 4035", Label: "Thin hex nut", dims: map[string][2]float64{
		"M2": {4, 1.2}, "M2.5": {5, 1.6}, "M3": {5.5, 1.8}, "M4": {7, 2.2}, "M5": {8, 2.7},
		"M6": {10, 3.2}, "M8": {13, 4}, "M10": {16, 5}, "M12": {18, 6}}},
}

// PartSpec is a washer or a nut: Kind "washer" or "nut", its standard and
// size.
type PartSpec struct {
	Kind, Std, Size string
	// Modelled: a nut's thread is modelled (else it is bored plain at
	// the nominal size)
	Modelled bool
}

// Label is how it is called in a parts list: "ISO 7089 M5".
func (p PartSpec) Label() string { return p.Std + " " + p.Size }

// Stds are the standards of its kind, with their labels.
func (p PartSpec) Stds() (ids, labels []string) {
	if p.Kind == "washer" {
		for _, s := range WasherStds {
			ids, labels = append(ids, s.ID), append(labels, s.Label)
		}
	} else {
		for _, s := range NutStds {
			ids, labels = append(ids, s.ID), append(labels, s.Label)
		}
	}
	return
}

// Sizes are the sizes its standard has.
func (p PartSpec) Sizes() []string {
	var out []string
	for _, z := range ScrewSizes {
		if _, err := p.dims(z); err == nil {
			out = append(out, z)
		}
	}
	return out
}

// dims are its standard's numbers for size z (washer: d1, d2, h; nut: s,
// m, d).
func (p PartSpec) dims(z string) ([3]float64, error) {
	switch p.Kind {
	case "washer":
		for _, s := range WasherStds {
			if s.ID == p.Std {
				if v, ok := s.dims[z]; ok {
					return v, nil
				}
				return [3]float64{}, fmt.Errorf("%s has no size %s", p.Std, z)
			}
		}
	case "nut":
		for _, s := range NutStds {
			if s.ID == p.Std {
				if v, ok := s.dims[z]; ok {
					return [3]float64{v[0], v[1], metrics[z].d}, nil
				}
				return [3]float64{}, fmt.Errorf("%s has no size %s", p.Std, z)
			}
		}
	default:
		return [3]float64{}, fmt.Errorf("no fastener kind %q", p.Kind)
	}
	return [3]float64{}, fmt.Errorf("no %s standard %q", p.Kind, p.Std)
}

// Thickness is how far it stands off the face it bears on.
func (p PartSpec) Thickness() float64 {
	v, err := p.dims(p.Size)
	if err != nil {
		return 0
	}
	if p.Kind == "washer" {
		return v[2]
	}
	return v[1]
}

// FastenerDesign builds a washer's or nut's own design.
func FastenerDesign(p PartSpec) (*Document, error) {
	v, err := p.dims(p.Size)
	if err != nil {
		return nil, err
	}
	d := New()
	set := func(f *Feature, kv ...any) {
		for i := 0; i+1 < len(kv); i += 2 {
			switch x := kv[i+1].(type) {
			case float64:
				f.Inputs[kv[i].(string)] = num(x)
			case string:
				f.Inputs[kv[i].(string)] = x
			}
		}
	}
	var bore, h float64
	if p.Kind == "washer" {
		ring := d.Insert(NewPrim(KCylinder))
		ring.Name = "Washer"
		set(ring, "Radius", v[1]/2, "Height", v[2], "X", 0.0, "Y", 0.0, "Z", 0.0)
		bore, h = v[0]/2, v[2]
	} else {
		sk := d.Insert(NewSketch(PlaneRef{Std: "xy"}))
		sk.Name = "Hexagon"
		R := v[0] / math.Sqrt(3)
		var pts, ids []int
		for k := 0; k < 6; k++ {
			a := float64(k) * math.Pi / 3
			pts = append(pts, sk.Sketch.AddPoint(geom.V2(R*math.Cos(a), R*math.Sin(a))).ID)
		}
		for k := range pts {
			ids = append(ids, sk.Sketch.AddLine(pts[k], pts[(k+1)%6]).ID)
		}
		e := d.Insert(NewExtrude([]ProfileRef{{Sketch: sk.ID, Outer: ids}}))
		e.Name = "Nut"
		set(e, "Distance", v[1])
		sk.Hidden = true
		bore, h = v[2]/2, v[1]
	}
	if p.Kind == "nut" && p.Modelled {
		// bored a hair under the thread's crests, then the thread cut
		m := metrics[p.Size]
		bore = ops.ThreadSpec{D: m.d, P: m.pitch, Internal: true}.Crest() - 0.01
	}
	hole := d.Insert(NewPrim(KCylinder))
	hole.Name = "Bore"
	set(hole, "Radius", bore, "Height", h+2, "X", 0.0, "Y", 0.0, "Z", -1.0, "Operation", "Cut")
	if p.Kind == "nut" && p.Modelled {
		m := metrics[p.Size]
		th := d.Insert(NewPrim(KThread))
		th.Name = "Thread"
		set(th, "Diameter", m.d, "Pitch", m.pitch, "Height", h+2, "Internal", "1", "X", 0.0, "Y", 0.0, "Z", -1.0, "Operation", "Cut")
	}
	d.Regenerate()
	for _, f := range d.Features {
		if f.Err != "" {
			return nil, fmt.Errorf("%s: %s", f.Name, f.Err)
		}
	}
	if len(d.Bodies) != 1 {
		return nil, fmt.Errorf("the %s came out as %d bodies", p.Kind, len(d.Bodies))
	}
	d.Bodies[0].Name = p.Label()
	return d, nil
}

var fastenerCache sync.Map // spec → design data

// FastenerData is a washer's or nut's design, as a component holds it.
func FastenerData(p PartSpec) (string, error) {
	if v, ok := fastenerCache.Load(p); ok {
		return v.(string), nil
	}
	d, err := FastenerDesign(p)
	if err != nil {
		return "", err
	}
	data := string(d.Marshal())
	fastenerCache.Store(p, data)
	return data, nil
}

// NewFastener is a washer or nut as a component.
func NewFastener(p PartSpec) (*Feature, error) {
	data, err := FastenerData(p)
	if err != nil {
		return nil, err
	}
	f := NewComponent(p.Label()+".forma", []byte(data))
	f.Name = p.Label()
	f.Inputs[fastenerInput(p.Kind)], f.Inputs["Size"] = p.Std, p.Size
	if p.Modelled {
		f.Inputs["Thread"] = "Modelled"
	}
	return f, nil
}

func fastenerInput(kind string) string {
	if kind == "washer" {
		return "Washer"
	}
	return "Nut"
}

// FastenerOf is a washer or nut component's spec (false for anything else).
func FastenerOf(f *Feature) (PartSpec, bool) {
	if f == nil || f.Kind != KComponent {
		return PartSpec{}, false
	}
	for _, k := range []string{"washer", "nut"} {
		if s := f.Inputs[fastenerInput(k)]; s != "" {
			return PartSpec{Kind: k, Std: s, Size: f.Inputs["Size"], Modelled: f.Inputs["Thread"] == "Modelled"}, true
		}
	}
	return PartSpec{}, false
}

// FastenerInHole joins a screw, washer or nut component to the hole whose
// rim is edge rim of body key: on whatever is already stacked on that rim
// (a washer, or a nut), else on the hole's seat (see holeSeat) — a revolute
// joint from its seat (its frame's origin, looking down its axis), so it
// follows the hole and turns in it with the joint's Angle.
func (d *Document) FastenerInHole(f *Feature, key int, rim *brep.Edge) (*Feature, error) {
	_, screw := ScrewOf(f)
	if _, ok := FastenerOf(f); !ok && !screw {
		return nil, fmt.Errorf("%s isn't a screw, washer or nut", f.Name)
	}
	var ref *BodyRef
	for _, r := range d.Bodies {
		if r.Key == key {
			ref = r
		}
	}
	if ref == nil {
		return nil, fmt.Errorf("the hole's body is gone")
	}
	if _, ok := rim.Curve.(geom.Circle); !ok {
		return nil, fmt.Errorf("pick a hole's round rim")
	}
	if _, ok := rimNormal(rim); !ok {
		return nil, fmt.Errorf("the rim has no flat face beside it")
	}
	seat := d.holeSeat(ref.Body, rim)
	fr := square(geom.Vec3{}, geom.ZAxis.Neg(), geom.XAxis)
	me := JointRef{Feature: f.ID, Key: BodyKey(f.ID, 0), Frame: &fr}
	top := d.stackTop(ref, seat)
	if c := d.Feature(ref.Comp); c != nil {
		if _, ok := FastenerOf(c); ok {
			// a washer's or nut's rim picked: on top of it (and of what is
			// on it)
			top = d.climb(c)
		}
	}
	if top != nil {
		p, ok := FastenerOf(top)
		if !ok {
			return nil, fmt.Errorf("%s is already in that hole: put washers and nuts on before the screw, or on the other side", top.Name)
		}
		on := square(geom.V3(0, 0, p.Thickness()), geom.ZAxis, geom.XAxis)
		j := NewJoint(JointRef{Feature: top.ID, Key: BodyKey(top.ID, 0), Frame: &on}, me, "Revolute")
		j.Name = f.Name + " on " + top.Name
		return j, nil
	}
	c := seat.Curve.(geom.Circle)
	hole, err := d.JointRefFor(key, nil, seat, nil, c.Eval(seat.T0))
	if err != nil {
		return nil, err
	}
	hole.Out = true
	j := NewJoint(hole, me, "Revolute")
	j.Name = f.Name + " in its hole"
	return j, nil
}

// stackTop is the last of the components jointed one on another onto the
// seat (edge e of the body), nil if nothing is.
func (d *Document) stackTop(ref *BodyRef, e *brep.Edge) *Feature {
	for _, j := range d.Features {
		if j.Kind != KJoint || !j.Active || j.Err != "" || len(j.Joint) != 2 {
			continue
		}
		a := j.Joint[0]
		if !a.Out || a.Edge == nil || a.Key != ref.Key {
			continue
		}
		// the edge the joint finds now (as it finds it: by its faces, nearest
		// its point) is this one
		r := *a.Edge
		if ref.Comp != 0 {
			r.P = ref.Place.Pt(r.P)
		}
		if _, got := d.findEdge([]*BodyRef{ref}, r); got != e {
			continue
		}
		return d.climb(d.Feature(j.Joint[1].Feature))
	}
	return nil
}

// climb is the last of the components jointed one on another onto top.
func (d *Document) climb(top *Feature) *Feature {
	seen := map[int]bool{}
	for top != nil && !seen[top.ID] {
		seen[top.ID] = true
		var next *Feature
		for _, j := range d.Features {
			if j.Kind == KJoint && j.Active && j.Err == "" && len(j.Joint) == 2 && j.Joint[0].Feature == top.ID && j.Joint[0].Frame != nil {
				next = d.Feature(j.Joint[1].Feature)
			}
		}
		if next == nil {
			break
		}
		top = next
	}
	return top
}
