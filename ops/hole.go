package ops

import (
	"fmt"
	"math"

	"forma/brep"
	"forma/geom"
	"forma/sketch"
)

// HoleSpec describes a drilled hole.
type HoleSpec struct {
	Diameter float64
	Depth    float64 // of the full diameter, from the face
	Tip      bool    // a 118° drill point at the bottom
	// Counterbore: a wider bore at the top
	BoreDiameter, BoreDepth float64
	// Countersink: a 90° cone at the top, this wide at the face
	SinkDiameter float64
}

// HoleTool is the body a hole cuts: a revolve about the line through at
// along dir (into the material), starting a little above the face so the
// cut doesn't leave a skin of coincident faces. Its faces are named
// "hole:<k>:…".
func HoleTool(at, dir, xref geom.Vec3, h HoleSpec, k int) (*brep.Body, error) {
	r := h.Diameter / 2
	if r <= geom.Tol || h.Depth <= geom.Tol {
		return nil, fmt.Errorf("a hole needs a diameter and a depth")
	}
	lift := math.Max(0.5, r)
	type pt = geom.Vec2
	// the profile in (radius, depth) with depth growing into the material
	pts := []pt{{X: 0, Y: -lift}}
	switch {
	case h.BoreDiameter > 0:
		rb := h.BoreDiameter / 2
		if rb <= r || h.BoreDepth <= 0 || h.BoreDepth >= h.Depth {
			return nil, fmt.Errorf("the counterbore must be wider than the hole and shallower")
		}
		pts = append(pts, pt{X: rb, Y: -lift}, pt{X: rb, Y: h.BoreDepth}, pt{X: r, Y: h.BoreDepth})
	case h.SinkDiameter > 0:
		rs := h.SinkDiameter / 2
		if rs <= r || rs-r >= h.Depth {
			return nil, fmt.Errorf("the countersink must be wider than the hole and shallower")
		}
		// 90°: as deep as it narrows; carried on up past the face
		pts = append(pts, pt{X: rs + lift, Y: -lift}, pt{X: r, Y: rs - r})
	default:
		pts = append(pts, pt{X: r, Y: -lift})
	}
	pts = append(pts, pt{X: r, Y: h.Depth})
	if h.Tip {
		pts = append(pts, pt{X: 0, Y: h.Depth + r/math.Tan(59*math.Pi/180)})
	} else {
		pts = append(pts, pt{X: 0, Y: h.Depth})
	}
	// as a region: counter-clockwise in a frame whose X is the radius and
	// Y points out of the hole (−depth)
	var segs []sketch.Seg
	flip := func(p pt) pt { return pt{X: p.X, Y: -p.Y} }
	n := len(pts)
	for i := n - 1; i >= 0; i-- {
		a, b := flip(pts[i]), flip(pts[(i-1+n)%n])
		segs = append(segs, sketch.Seg{A: a, B: b, Src: n - i})
	}
	reg := sketch.Region{Outer: segs}
	dir = dir.Norm()
	X := xref.Sub(dir.Scale(xref.Dot(dir))).Norm()
	if X.Len() < 0.5 {
		X = dir.AnyPerp()
	}
	Y := dir.Neg()
	fr := geom.Frame{O: at, X: X, Y: Y, Z: X.Cross(Y)}
	b, err := Revolve("hole", reg, fr, geom.Vec2{}, geom.V2(0, 1), 2*math.Pi)
	if err != nil {
		return nil, err
	}
	for _, f := range b.Faces() {
		f.Name = fmt.Sprintf("hole:%d:%s", k, f.Name)
	}
	return b, nil
}
