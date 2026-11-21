package doc

// Exploded views: every component pushed away from what it is joined to,
// along the joint's axis, so a stack of parts comes apart the way it goes
// together; a component joined to nothing moves out from the assembly's
// middle (and so does a body of the design's own), and a grounded one
// stays. A joint's moving component also takes the push of the one it is
// joined to (a pin in a wheel on an axle moves out with the wheel, and
// further). Nothing in the design changes: these are offsets for drawing.

import (
	"math"

	"forma/geom"
)

// Explode is how far each part moves at amount (0: assembled; 1: by the
// assembly's size per step away from its ground), by ExplodeGroup: a
// component (all its bodies together) or a body of the design's own,
// which moves out from the middle like a loose component.
func (d *Document) Explode(amount float64) map[int]geom.Vec3 {
	out := map[int]geom.Vec3{}
	if amount == 0 {
		return out
	}
	boxes := map[int]*geom.Box{}
	var all geom.Box
	var order []int
	for _, r := range d.Bodies {
		g := ExplodeGroup(r.Comp, r.Key)
		b := boxes[g]
		if b == nil {
			b = &geom.Box{}
			boxes[g] = b
			order = append(order, g)
		}
		b.Union(r.Body.Bounds())
		all.Union(r.Body.Bounds())
	}
	if len(order) < 2 {
		return out
	}
	size := all.Diagonal() * amount
	mid := all.Center()
	// what each component is joined to (the last active joint moving it)
	parent := map[int]int{}
	axis := map[int]geom.Vec3{}
	for _, f := range d.Features {
		if f.Kind != KJoint || !f.Active || f.Err != "" || f.Moved == 0 || len(f.Joint) != 2 {
			continue
		}
		fixed := f.Joint[0].Feature
		if fixed == f.Moved {
			fixed = f.Joint[1].Feature
		}
		if boxes[f.Moved] != nil && boxes[fixed] != nil {
			parent[f.Moved], axis[f.Moved] = fixed, f.JointAxis.Z
		}
	}
	var place func(c int, depth int) geom.Vec3
	place = func(c int, depth int) geom.Vec3 {
		if v, ok := out[c]; ok {
			return v
		}
		if Grounded(d.Feature(c)) || depth > len(order) {
			out[c] = geom.Vec3{}
			return out[c]
		}
		at := boxes[c].Center()
		p, ok := parent[c]
		if !ok {
			dir := at.Sub(mid)
			if dir.Len() < 1e-9*all.Diagonal() {
				dir = geom.ZAxis
			}
			out[c] = dir.Norm().Scale(size)
			return out[c]
		}
		base := place(p, depth+1)
		// along the joint's axis, away from the part it is joined to
		dir := axis[c]
		away := at.Sub(boxes[p].Center())
		if math.Abs(dir.Dot(away)) < 1e-6*all.Diagonal() {
			dir = away // across the axis (a pin centred in a hole's middle): straight away
			if dir.Len() < 1e-9 {
				dir = axis[c]
			}
		} else if dir.Dot(away) < 0 {
			dir = dir.Neg()
		}
		out[c] = base.Add(dir.Norm().Scale(size))
		return out[c]
	}
	for _, c := range order {
		place(c, 0)
	}
	return out
}

// ExplodeGroup is what moves together in an exploded view: a component, or
// a body of the design's own (by its key, negated).
func ExplodeGroup(comp, key int) int {
	if comp != 0 {
		return comp
	}
	return -key
}
