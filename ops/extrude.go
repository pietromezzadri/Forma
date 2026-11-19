package ops

import (
	"fmt"
	"math"

	"forma/brep"
	"forma/geom"
	"forma/sketch"
)

// Extrude sweeps a sketch region along its plane's normal, from offset d0
// to d1 (mm, d0 < d1). Lines become planar side faces and arcs cylindrical
// ones; the faces are named for persistent references: "bottom", "top" and
// "side:<entity>:<n>" (the n-th piece of sketch entity <entity>).
func Extrude(name string, reg sketch.Region, plane geom.Frame, d0, d1 float64) (*brep.Body, error) {
	if d1-d0 < geom.Tol {
		return nil, fmt.Errorf("extrude: zero length")
	}
	if len(reg.Outer) < 1 {
		return nil, fmt.Errorf("extrude: empty profile")
	}
	b := brep.NewBuilder(name)
	N := plane.Z
	at := func(p geom.Vec2, d float64) geom.Vec3 { return plane.At2(p).Add(N.Scale(d)) }
	pieces := map[int]int{}

	type ring struct {
		segs     []sketch.Seg
		bot, top []*brep.Edge
		lat      []*brep.Edge // lateral edge at each segment's start
	}
	loops := append([][]sketch.Seg{reg.Outer}, reg.Holes...)
	var rings []*ring
	for _, segs := range loops {
		r := &ring{segs: segs}
		n := len(segs)
		vb := make([]*brep.Vertex, n)
		vt := make([]*brep.Vertex, n)
		for i, s := range segs {
			vb[i] = b.Vertex(at(s.A, d0))
			vt[i] = b.Vertex(at(s.A, d1))
		}
		for i := range segs {
			r.lat = append(r.lat, b.LineEdge(vb[i], vt[i]))
		}
		for i, s := range segs {
			j := (i + 1) % n
			r.bot = append(r.bot, segEdge(b, plane, s, d0, vb[i], vb[j]))
			r.top = append(r.top, segEdge(b, plane, s, d1, vt[i], vt[j]))
		}
		rings = append(rings, r)
	}
	// top: every loop as it runs; bottom: reversed
	var topLoops, botLoops [][]brep.Use
	for _, r := range rings {
		var t, bt []brep.Use
		for i := range r.segs {
			t = append(t, brep.F(r.top[i]))
		}
		for i := len(r.segs) - 1; i >= 0; i-- {
			bt = append(bt, brep.R(r.bot[i]))
		}
		topLoops = append(topLoops, t)
		botLoops = append(botLoops, bt)
	}
	b.Face(geom.Plane{F: geom.FrameZ(plane.O.Add(N.Scale(d1)), N, plane.X)}, false, "top", topLoops...)
	b.Face(geom.Plane{F: geom.FrameZ(plane.O.Add(N.Scale(d0)), N.Neg(), plane.X)}, false, "bottom", botLoops...)
	// sides: material is left of the loop's travel, so the outward normal is to its right
	for _, r := range rings {
		n := len(r.segs)
		for i, s := range r.segs {
			j := (i + 1) % n
			var surf geom.Surface
			rev := false
			if s.Arc {
				f := plane
				f.O = plane.At2(s.C)
				surf = geom.Cylinder{F: f, R: s.R}
				rev = !s.CCW
			} else {
				a3, b3 := plane.At2(s.A), plane.At2(s.B)
				d := b3.Sub(a3).Norm()
				surf = geom.Plane{F: geom.FrameZ(a3, d.Cross(N), d)}
			}
			k := pieces[s.Src]
			pieces[s.Src]++
			b.Face(surf, rev, fmt.Sprintf("side:%d:%d", s.Src, k),
				[]brep.Use{brep.F(r.bot[i]), brep.F(r.lat[j]), brep.R(r.top[i]), brep.R(r.lat[i])})
		}
	}
	return b.Finish()
}

// segEdge makes the 3D edge of a profile segment at offset d.
func segEdge(b *brep.Builder, plane geom.Frame, s sketch.Seg, d float64, va, vb *brep.Vertex) *brep.Edge {
	if !s.Arc {
		return b.LineEdge(va, vb)
	}
	f := plane
	f.O = plane.At2(s.C).Add(plane.Z.Scale(d))
	a0 := s.A.Sub(s.C).Angle()
	a1 := s.B.Sub(s.C).Angle()
	var t1 float64
	if s.CCW {
		sw := geom.WrapAngle(a1 - a0)
		if sw < 1e-12 {
			sw = 2 * math.Pi
		}
		t1 = a0 + sw
	} else {
		sw := geom.WrapAngle(a0 - a1)
		if sw < 1e-12 {
			sw = 2 * math.Pi
		}
		t1 = a0 - sw
	}
	return b.Edge(va, vb, geom.Circle{F: f, R: s.R}, a0, t1)
}
