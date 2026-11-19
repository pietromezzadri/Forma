package ops

import (
	"fmt"
	"math"

	"forma/brep"
	"forma/geom"
	"forma/sketch"
)

// EdgeTool makes the body that rounds (or bevels) an edge of a solid: the
// corner between the edge and the rolling ball's cylinder (or the chamfer's
// plane), swept along the edge. A convex edge has it cut away, a concave one
// filled in (op says which). It handles straight edges between flat faces
// and circles where a flat face meets a cylinder or cone square on (the
// sweep is then a revolve). The rounding face is named "round:<k>" (or
// "chamfer:<k>").
//
// size is the radius, or for a chamfer the distance back along each face.
// A cut is carried on past an end of the edge where ext says (where another
// rounded edge meets it, so the two meet in a mitre); elsewhere it stops
// square at the end.
func EdgeTool(e *brep.Edge, size float64, chamfer bool, k int, ext [2]bool) (*brep.Body, brep.BoolOp, error) {
	if size <= geom.Tol {
		return nil, 0, fmt.Errorf("size must be more than 0")
	}
	if e.HE[0] == nil || e.HE[1] == nil {
		return nil, 0, fmt.Errorf("edge %d is open", e.ID)
	}
	f1, f2 := e.HE[0].Loop.Face, e.HE[1].Loop.Face
	if f1 == f2 {
		return nil, 0, fmt.Errorf("edge %d is a seam", e.ID)
	}
	p1, ok1 := f1.Surf.(geom.Plane)
	p2, ok2 := f2.Surf.(geom.Plane)
	switch {
	case ok1 && ok2:
		_ = p1
		_ = p2
		return straightTool(e, size, chamfer, k, ext)
	case ok1 || ok2:
		return circleTool(e, size, chamfer, k, ext)
	}
	return nil, 0, fmt.Errorf("only edges where a flat face meets a flat face, a cylinder or a cone can be %s", verb(chamfer))
}

func verb(chamfer bool) string {
	if chamfer {
		return "chamfered"
	}
	return "filleted"
}

// sideDir is the direction into face h.Loop.Face from the edge at p,
// across the edge (the face lies left of its loops' travel).
func sideDir(h *brep.HalfEdge, p geom.Vec3) geom.Vec3 {
	e := h.Edge
	t := e.Curve.Project(p)
	if c, ok := e.Curve.(geom.Circle); ok {
		t = nearAngle(c, e, p)
	}
	d := e.Curve.Deriv(t).Norm()
	if e.T1 < e.T0 {
		d = d.Neg()
	}
	if !h.Fwd {
		d = d.Neg()
	}
	f := h.Loop.Face
	u, v := f.Surf.Project(p)
	n := f.OutwardNormal(u, v)
	return n.Cross(d).Norm()
}

func nearAngle(c geom.Circle, e *brep.Edge, p geom.Vec3) float64 {
	t := c.Project(p)
	lo := math.Min(e.T0, e.T1)
	for t < lo-1e-9 {
		t += 2 * math.Pi
	}
	return t
}

// cornerRegion is the 2D region between the corner p, the two faces'
// directions u1, u2 (unit, at angle α < π) and the round (or chamfer).
func cornerRegion(p, u1, u2 geom.Vec2, size float64, chamfer bool, src int) (sketch.Region, error) {
	cos := u1.Dot(u2)
	alpha := math.Acos(math.Max(-1, math.Min(1, cos)))
	if alpha < 1e-3 || alpha > math.Pi-1e-3 {
		return sketch.Region{}, fmt.Errorf("the faces meet too flat (or fold back) to round")
	}
	ccw := u1.Cross(u2) > 0
	if !ccw {
		u1, u2 = u2, u1
	}
	var segs []sketch.Seg
	if chamfer {
		t1, t2 := p.Add(u1.Scale(size)), p.Add(u2.Scale(size))
		segs = []sketch.Seg{{A: p, B: t1, Src: src}, {A: t1, B: t2, Src: src + 1}, {A: t2, B: p, Src: src + 2}}
	} else {
		d := size / math.Tan(alpha/2)
		t1, t2 := p.Add(u1.Scale(d)), p.Add(u2.Scale(d))
		c := p.Add(u1.Add(u2).Norm().Scale(size / math.Sin(alpha/2)))
		segs = []sketch.Seg{
			{A: p, B: t1, Src: src},
			{Arc: true, A: t1, B: t2, C: c, R: size, CCW: false, Src: src + 1},
			{A: t2, B: p, Src: src + 2},
		}
	}
	return sketch.Region{Outer: segs}, nil
}

// convex: the material lies between the two faces (the edge is cut away).
func convex(h1, h2 *brep.HalfEdge, p geom.Vec3) bool {
	f1 := h1.Loop.Face
	u, v := f1.Surf.Project(p)
	n1 := f1.OutwardNormal(u, v)
	return sideDir(h2, p).Dot(n1) < 0
}

func straightTool(e *brep.Edge, size float64, chamfer bool, k int, ext [2]bool) (*brep.Body, brep.BoolOp, error) {
	a, b := e.Verts[0].P, e.Verts[1].P
	L := a.Dist(b)
	if L < geom.Tol {
		return nil, 0, fmt.Errorf("edge %d is closed", e.ID)
	}
	mid := e.Curve.Eval((e.T0 + e.T1) / 2)
	if dev := lineDist(a, b, mid); dev > 1e-6*(1+L) {
		return nil, 0, fmt.Errorf("edge %d isn't straight", e.ID)
	}
	t := b.Sub(a).Scale(1 / L)
	u1 := sideDir(e.HE[0], mid)
	u2 := sideDir(e.HE[1], mid)
	cut := convex(e.HE[0], e.HE[1], mid)
	// the cross-section's frame at a: X = u1, Y the part of u2 across it
	X := u1
	Y := u2.Sub(X.Scale(u2.Dot(X))).Norm()
	fr := geom.Frame{O: a, X: X, Y: Y, Z: X.Cross(Y)}
	reg, err := cornerRegion(geom.Vec2{}, geom.V2(1, 0), geom.V2(u2.Dot(X), u2.Dot(Y)), size, chamfer, 1)
	if err != nil {
		return nil, 0, err
	}
	// past the ends where it is cut away (no faces to coincide), exactly to
	// them where it fills in (so it doesn't stand out of the end faces)
	m0, m1 := 0.0, 0.0
	if cut && ext[0] {
		m0 = 2*size + 0.01*L
	}
	if cut && ext[1] {
		m1 = 2*size + 0.01*L
	}
	d0, d1 := -m0, L+m1
	if fr.Z.Dot(t) < 0 {
		d0, d1 = -L-m1, m0
	}
	body, err := Extrude("tool", reg, fr, d0, d1)
	if err != nil {
		return nil, 0, err
	}
	nameTool(body, chamfer, k)
	op := brep.Union
	if cut {
		op = brep.Subtract
	}
	return body, op, nil
}

func lineDist(a, b, p geom.Vec3) float64 {
	ab := b.Sub(a)
	l := ab.Len()
	if l == 0 {
		return p.Dist(a)
	}
	return ab.Cross(p.Sub(a)).Len() / l
}

// circleTool rounds the circle (or arc) where a flat face meets a cylinder
// or cone whose axis is square to it.
func circleTool(e *brep.Edge, size float64, chamfer bool, k int, ext [2]bool) (*brep.Body, brep.BoolOp, error) {
	var axis geom.Frame
	var plane geom.Plane
	var curved geom.Surface
	for _, h := range e.HE {
		switch s := h.Loop.Face.Surf.(type) {
		case geom.Plane:
			plane = s
		case geom.Cylinder:
			axis, curved = s.F, s
		case geom.Cone:
			axis, curved = s.F, s
		default:
			return nil, 0, fmt.Errorf("only edges where a flat face meets a flat face, a cylinder or a cone can be %s", verb(chamfer))
		}
	}
	A := axis.Z
	if math.Abs(math.Abs(A.Dot(plane.F.Z))-1) > 1e-7 {
		return nil, 0, fmt.Errorf("the cylinder isn't square to the flat face")
	}
	// the circle's centre: the axis where it meets the plane
	O := axis.O.Add(A.Scale(plane.F.O.Sub(axis.O).Dot(plane.F.Z) / A.Dot(plane.F.Z)))
	// a point of the circle: the edge's middle put exactly on both faces
	// (a boolean's edge is a polyline whose chords cut inside the circle)
	mid := e.Curve.Eval((e.T0 + e.T1) / 2)
	if p, ok := geom.OntoBoth(plane, curved, mid, 1e-12*(1+mid.Len())); ok {
		mid = p
	}
	rv := mid.Sub(O)
	rv = rv.Sub(A.Scale(rv.Dot(A)))
	R := rv.Len()
	if R < geom.Tol {
		return nil, 0, fmt.Errorf("edge %d is on the axis", e.ID)
	}
	X := rv.Scale(1 / R)
	// the meridian plane through mid: (radius, height)
	fr := geom.Frame{O: O, X: X, Y: A, Z: X.Cross(A)}
	to2 := func(d geom.Vec3) geom.Vec2 { return geom.V2(d.Dot(X), d.Dot(A)).Norm() }
	u1 := to2(sideDir(e.HE[0], mid))
	u2 := to2(sideDir(e.HE[1], mid))
	cut := convex(e.HE[0], e.HE[1], mid)
	reg, err := cornerRegion(geom.V2(R, 0), u1, u2, size, chamfer, 1)
	if err != nil {
		return nil, 0, err
	}
	for _, s := range reg.Outer {
		for _, q := range sketch.SegPoints(s, 0.01) {
			if q.X < -geom.Tol {
				return nil, 0, fmt.Errorf("a radius of %.4g is too big for a circle of %.4g", size, R)
			}
		}
	}
	angle := 2 * math.Pi
	if e.Verts[0] != e.Verts[1] {
		// an arc: from one end to the other the way through mid, and a little past
		a0 := angleAbout(fr, e.Verts[0].P)
		a1 := angleAbout(fr, e.Verts[1].P)
		// mid is at angle 0: the arc runs from a0 down through 0 to a1, or up
		lo, hi := math.Min(a0, a1), math.Max(a0, a1)
		if !(lo < 0 && hi > 0) {
			return nil, 0, fmt.Errorf("can't follow arc %d", e.ID)
		}
		m := 0.0
		if cut {
			m = math.Min(0.2, (2*math.Pi-(hi-lo))/4)
		}
		mlo, mhi := 0.0, 0.0
		// ext is by vertex: which end is at lo
		e0lo := a0 <= a1
		if (e0lo && ext[0]) || (!e0lo && ext[1]) {
			mlo = m
		}
		if (e0lo && ext[1]) || (!e0lo && ext[0]) {
			mhi = m
		}
		angle = hi - lo + mlo + mhi
		// turn the sketch frame to the start
		start := lo - mlo
		rot := geom.Rotate(O, A, start)
		fr.X = rot.Dir(fr.X)
		fr.Z = fr.X.Cross(fr.Y)
	}
	body, err := Revolve("tool", reg, fr, geom.Vec2{}, geom.V2(0, 1), angle)
	if err != nil {
		return nil, 0, err
	}
	nameTool(body, chamfer, k)
	op := brep.Union
	if cut {
		op = brep.Subtract
	}
	return body, op, nil
}

// angleAbout is p's angle round the frame's Y axis from X toward −Z
// (the way Revolve turns, right-handed about Y).
func angleAbout(fr geom.Frame, p geom.Vec3) float64 {
	l := fr.ToLocal(p)
	return math.Atan2(-l.Z, l.X)
}

// nameTool names the tool's faces: its rounding face (from segment 2)
// "round:k" / "chamfer:k", the rest "tool:k:…".
func nameTool(b *brep.Body, chamfer bool, k int) {
	for _, f := range b.Faces() {
		switch f.Name {
		case "side:2:0", "rev:2:0":
			if chamfer {
				f.Name = fmt.Sprintf("chamfer:%d", k)
			} else {
				f.Name = fmt.Sprintf("round:%d", k)
			}
		default:
			f.Name = fmt.Sprintf("tool:%d:%s", k, f.Name)
		}
	}
}
