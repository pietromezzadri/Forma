package ops

// Extruding between two surfaces: a region swept along its plane's normal,
// every point of it from where its line along the normal meets one surface
// to where it meets the other. Built face by face like Extrude, with the
// caps on those surfaces and the cap edges the outline projected onto them
// (exact points, a polyline between), so no boolean is needed: a letter
// standing on a curved face, its foot sunk into it.

import (
	"fmt"
	"math"

	"forma/brep"
	"forma/geom"
	"forma/sketch"
)

// ExtrudeBetween sweeps reg along plane.Z from surface s0 to surface s1,
// each met where the line along the normal through a point crosses it
// nearest guess (an offset along the normal from the sketch plane). The
// walls are those of Extrude; the caps lie on s0 and s1.
func ExtrudeBetween(name string, reg sketch.Region, plane geom.Frame, s0, s1 geom.Surface, guess float64) (*brep.Body, error) {
	if len(reg.Outer) < 1 {
		return nil, fmt.Errorf("extrude: empty profile")
	}
	N := plane.Z
	b := brep.NewBuilder(name)
	reach := 1e3
	hit := func(s geom.Surface, q geom.Vec2, from float64) (geom.Vec3, float64, error) {
		p := plane.At2(q)
		t, ok := crossAlong(s, p, N, from, reach)
		if !ok {
			return geom.Vec3{}, 0, fmt.Errorf("the profile doesn't reach the surface along its normal")
		}
		return p.Add(N.Scale(t)), t, nil
	}
	// which surface comes first along the normal
	_, ta, err := hit(s0, reg.Outer[0].A, guess)
	if err != nil {
		return nil, err
	}
	_, tb, err := hit(s1, reg.Outer[0].A, guess)
	if err != nil {
		return nil, err
	}
	if math.Abs(tb-ta) < geom.Tol {
		return nil, fmt.Errorf("extrude: the surfaces meet there")
	}
	lo, hi := s0, s1
	if tb < ta {
		lo, hi = s1, s0
		ta, tb = tb, ta
	}
	pieces := map[int]int{}
	type ring struct {
		segs     []sketch.Seg
		bot, top []*brep.Edge
		lat      []*brep.Edge
	}
	loops := append([][]sketch.Seg{reg.Outer}, reg.Holes...)
	var rings []*ring
	lastLo, lastHi := ta, tb
	for _, segs := range loops {
		r := &ring{segs: segs}
		n := len(segs)
		vb := make([]*brep.Vertex, n)
		vt := make([]*brep.Vertex, n)
		for i, s := range segs {
			pb, t0, err := hit(lo, s.A, lastLo)
			if err != nil {
				return nil, err
			}
			pt, t1, err := hit(hi, s.A, lastHi)
			if err != nil {
				return nil, err
			}
			if t1-t0 < 1e-6 {
				return nil, fmt.Errorf("extrude: the surfaces cross inside the profile")
			}
			lastLo, lastHi = t0, t1
			vb[i], vt[i] = b.Vertex(pb), b.Vertex(pt)
		}
		for i := range segs {
			r.lat = append(r.lat, b.LineEdge(vb[i], vt[i]))
		}
		for i, s := range segs {
			j := (i + 1) % n
			wall := wallSurface(plane, s)
			eb, err := projEdge(b, plane, s, lo, wall, vb[i], vb[j])
			if err != nil {
				return nil, err
			}
			et, err := projEdge(b, plane, s, hi, wall, vt[i], vt[j])
			if err != nil {
				return nil, err
			}
			r.bot = append(r.bot, eb)
			r.top = append(r.top, et)
		}
		rings = append(rings, r)
	}
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
	// the caps' outward normals: along the normal on top, against it below
	facing := func(s geom.Surface, p geom.Vec3, want geom.Vec3) bool {
		u, v := s.Project(p)
		return s.Normal(u, v).Dot(want) < 0
	}
	b.Face(hi, facing(hi, rings[0].top[0].Verts[0].P, N), "top", topLoops...)
	b.Face(lo, facing(lo, rings[0].bot[0].Verts[0].P, N.Neg()), "bottom", botLoops...)
	for _, r := range rings {
		n := len(r.segs)
		for i, s := range r.segs {
			j := (i + 1) % n
			surf, rev := wallSurface(plane, s), false
			if s.Ctrl == nil && s.Arc {
				rev = !s.CCW
			}
			k := pieces[s.Src]
			pieces[s.Src]++
			b.Face(surf, rev, fmt.Sprintf("side:%d:%d", s.Src, k),
				[]brep.Use{brep.F(r.bot[i]), brep.F(r.lat[j]), brep.R(r.top[i]), brep.R(r.lat[i])})
		}
	}
	return b.Finish()
}

// wallSurface is the side face Extrude makes for a profile segment.
func wallSurface(plane geom.Frame, s sketch.Seg) geom.Surface {
	N := plane.Z
	switch {
	case s.Ctrl != nil:
		return geom.Extruded{C: bezier3(plane, s.Ctrl, 0), D: N}
	case s.Arc:
		f := plane
		f.O = plane.At2(s.C)
		return geom.Cylinder{F: f, R: s.R}
	}
	a3, b3 := plane.At2(s.A), plane.At2(s.B)
	d := b3.Sub(a3).Norm()
	return geom.Plane{F: geom.FrameZ(a3, d.Cross(N), d)}
}

// projEdge is segment s projected along the plane's normal onto surface
// cap, between vertices va and vb (already on it): a polyline through
// exact points, toleranced by how far its chords stray from the cap and
// the wall.
func projEdge(b *brep.Builder, plane geom.Frame, s sketch.Seg, cap, wall geom.Surface, va, vb *brep.Vertex) (*brep.Edge, error) {
	N := plane.Z
	qs := sketch.SegPoints(s, 2e-4)
	if len(qs) < 2 {
		qs = []geom.Vec2{s.A, s.B}
	}
	pts := make([]geom.Vec3, len(qs))
	pts[0], pts[len(pts)-1] = va.P, vb.P
	from := va.P.Sub(plane.At2(qs[0])).Dot(N)
	for i := 1; i+1 < len(qs); i++ {
		p := plane.At2(qs[i])
		t, ok := crossAlong(cap, p, N, from, 1e3)
		if !ok {
			return nil, fmt.Errorf("the profile doesn't reach the surface along its normal")
		}
		from = t
		pts[i] = p.Add(N.Scale(t))
	}
	tol := 1e-7
	for i := 1; i < len(pts); i++ {
		m := pts[i-1].Lerp(pts[i], 0.5)
		tol = math.Max(tol, math.Max(geom.SurfaceDist(cap, m), geom.SurfaceDist(wall, m)))
	}
	if len(pts) == 2 && tol <= 4e-7 {
		return b.LineEdge(va, vb), nil
	}
	pl := geom.NewPolyline(pts, tol*1.5)
	return b.Edge(va, vb, pl, 0, pl.Length()), nil
}

// crossAlong is where the line p + t·n crosses surface s nearest t = from
// (searching out from there both ways, as far as reach), refined to the
// root.
func crossAlong(s geom.Surface, p, n geom.Vec3, from, reach float64) (float64, bool) {
	f := func(t float64) float64 { return geom.SignedDist(s, p.Add(n.Scale(t))) }
	root := func(a, b float64) float64 {
		fa := f(a)
		for i := 0; i < 100 && b-a > 1e-13*(1+math.Abs(a)); i++ {
			m := (a + b) / 2
			fm := f(m)
			if (fm < 0) == (fa < 0) {
				a, fa = m, fm
			} else {
				b = m
			}
		}
		return (a + b) / 2
	}
	f0 := f(from)
	if f0 == 0 {
		return from, true
	}
	h := math.Max(math.Abs(f0)/4, 1e-3)
	up, dn := from, from
	fu, fd := f0, f0
	for k := 0; k < 4000 && math.Abs(up-from) <= reach; k++ {
		nu, nd := up+h, dn-h
		gu, gd := f(nu), f(nd)
		if (gu < 0) != (fu < 0) {
			return root(up, nu), true
		}
		if (gd < 0) != (fd < 0) {
			return root(nd, dn), true
		}
		up, dn, fu, fd = nu, nd, gu, gd
		h = math.Min(h*1.15, 2)
	}
	return 0, false
}
