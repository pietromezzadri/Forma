package ops

import (
	"fmt"
	"math"

	"forma/brep"
	"forma/geom"
)

// Modelled threads: ISO 68-1's basic profile (60° flanks) swept along a
// helix, right-handed. The kernel has no helical surfaces, so a threaded
// length is a solid of flat facets — Steps of them a turn — built whole: a
// rod whose side is the thread (a bolt's teeth, or the space a nut's teeth
// leave), cut off flat at both ends. A bolt's rod is joined to the plain
// shank on the flat end; a tapped hole's rod is cut from the part where
// a drill a hair narrower than its crests already went, so no facet of it
// crosses a face of the part but at the hole's ends.

// ThreadSpec is a thread on the axis of frame F: nominal (major) diameter D
// and pitch P, from Z0 to Z1 along F's Z (Z0 < Z1).
type ThreadSpec struct {
	F        geom.Frame
	D, P     float64
	Z0, Z1   float64
	Internal bool // a nut's or tapped hole's (the rod is the hole's shape)
	Steps    int  // facets a turn (0: 48)
}

func (s ThreadSpec) steps() int {
	if s.Steps <= 0 {
		return 48
	}
	return s.Steps
}

// profile is the rod's radius over one pitch, as breakpoints (offset along
// the axis, radius) from 0 to P (the last back at the first's radius).
func (s ThreadSpec) profile() [][2]float64 {
	t30 := math.Tan(math.Pi / 6)
	R, P := s.D/2, s.P
	if !s.Internal {
		// a bolt: crest P/8 wide at the major diameter (a hundredth of the
		// pitch under it, as a bolt's is: its rod then sits inside a shank of
		// the nominal size without touching its rim), flanks down to the
		// minor d3 = d − 1.22687 P, the root flat between
		r3 := R - 0.613435*P
		R -= 0.01 * P
		f := P/16 + (R-r3)*t30
		return [][2]float64{{0, R}, {P / 16, R}, {f, r3}, {P - f, r3}, {P - P/16, R}}
	}
	// a nut's hole: its root P/8 wide at the major diameter, flanks in to
	// the minor D1 = D − 1.082532 P, its crest flat P/4 there
	r1 := R - 0.541266*P
	f := P/16 + (R-r1)*t30
	return [][2]float64{{0, R}, {P / 16, R}, {f, r1}, {P - f, r1}, {P - P/16, R}}
}

// Crest is the radius of the thread's innermost facets (a bolt's root, a
// nut's crest) at their nearest the axis: a hole drilled under it is
// cleared by the thread's facets.
func (s ThreadSpec) Crest() float64 {
	r := math.Inf(1)
	for _, p := range s.profile() {
		r = math.Min(r, p[1])
	}
	return r * math.Cos(math.Pi/float64(s.steps()))
}

// ThreadedRod is the threaded length as a solid: the thread's surface
// round the axis from Z0 to Z1, flat ends there.
func ThreadedRod(s ThreadSpec) (*brep.Body, error) {
	if s.D <= 0 || s.P <= 0 || s.Z1-s.Z0 < s.P/4 {
		return nil, fmt.Errorf("a thread needs a diameter, a pitch and a length")
	}
	n := s.steps()
	prof := s.profile()
	m := len(prof)
	// the band: angle step i, breakpoint j; (i, m) is (i + n, 0). It runs
	// from a turn under Z0 to a turn over Z1.
	// (shifted a fraction of a step so that no vertex lies on an end's
	// plane: clipping there would leave edges of no length)
	zs := s.Z0 - 2*s.P
	for _, f := range []float64{0.37, 0.61, 0.13, 0.83} {
		zs = s.Z0 - 2*s.P - f*s.P/float64(n)
		if clear(zs, s, prof, n) {
			break
		}
	}
	K := int(math.Ceil((s.Z1-zs)/s.P)+2) * n
	F := s.F
	pt := func(i, j int) geom.Vec3 {
		a := 2 * math.Pi * float64(i) / float64(n)
		z := zs + prof[j][0] + s.P*float64(i)/float64(n)
		r := prof[j][1]
		return geom.V3(r*math.Cos(a), r*math.Sin(a), z)
	}
	type vtx = geom.Vec3
	var tris [][3]vtx
	for i := 0; i < K; i++ {
		for j := 0; j < m; j++ {
			a, b := pt(i, j), pt(i+1, j)
			var c, d vtx
			if j+1 < m {
				c, d = pt(i+1, j+1), pt(i, j+1)
			} else {
				c, d = pt(i+1+n, 0), pt(i+n, 0)
			}
			tris = append(tris, [3]vtx{a, b, c}, [3]vtx{a, c, d})
		}
	}
	// clipped to Z0 ≤ z ≤ Z1, the cut edges kept for the ends
	tris, cut0 := clipZ(tris, s.Z0, true)
	tris, cut1 := clipZ(tris, s.Z1, false)
	// the side faces out (away from the axis) or in: the ends follow it
	out := 0.0
	for _, t := range tris {
		nrm := t[1].Sub(t[0]).Cross(t[2].Sub(t[0]))
		c := t[0].Add(t[1]).Add(t[2]).Scale(1.0 / 3)
		out += nrm.Dot(geom.V3(c.X, c.Y, 0))
	}
	if out < 0 {
		for i := range tris {
			tris[i][1], tris[i][2] = tris[i][2], tris[i][1]
		}
		for i := range cut0 {
			cut0[i][0], cut0[i][1] = cut0[i][1], cut0[i][0]
		}
		for i := range cut1 {
			cut1[i][0], cut1[i][1] = cut1[i][1], cut1[i][0]
		}
	}
	// the ends: a fan from the axis to each cut edge, the other way round
	for _, e := range cut0 {
		tris = append(tris, [3]vtx{geom.V3(0, 0, s.Z0), e[1], e[0]})
	}
	for _, e := range cut1 {
		tris = append(tris, [3]vtx{geom.V3(0, 0, s.Z1), e[1], e[0]})
	}
	pos := make([]geom.Vec3, 0, 3*len(tris))
	ix := make([][3]int, len(tris))
	for i, t := range tris {
		for k := 0; k < 3; k++ {
			p := t[k]
			pos = append(pos, F.O.Add(F.X.Scale(p.X)).Add(F.Y.Scale(p.Y)).Add(F.Z.Scale(p.Z)))
			ix[i][k] = 3*i + k
		}
	}
	return MeshSolid("thread", pos, ix)
}

// clipZ keeps the triangles' parts above z (keepAbove) or below it, and
// returns the edges where they were cut, each as the kept triangle runs
// along it.
func clipZ(tris [][3]geom.Vec3, z float64, keepAbove bool) ([][3]geom.Vec3, [][2]geom.Vec3) {
	in := func(p geom.Vec3) bool {
		if keepAbove {
			return p.Z >= z
		}
		return p.Z <= z
	}
	// where the edge p–q meets the plane, worked out the same whichever
	// way round it is given (so neighbours share the point exactly)
	cross := func(p, q geom.Vec3) geom.Vec3 {
		if q.Z < p.Z || (q.Z == p.Z && (q.X < p.X || (q.X == p.X && q.Y < p.Y))) {
			p, q = q, p
		}
		t := (z - p.Z) / (q.Z - p.Z)
		return geom.V3(p.X+t*(q.X-p.X), p.Y+t*(q.Y-p.Y), z)
	}
	var out [][3]geom.Vec3
	var cut [][2]geom.Vec3
	for _, t := range tris {
		var poly []geom.Vec3
		var onCut [][2]geom.Vec3
		for k := 0; k < 3; k++ {
			p, q := t[k], t[(k+1)%3]
			ip, iq := in(p), in(q)
			if ip {
				poly = append(poly, p)
			}
			if ip != iq {
				poly = append(poly, cross(p, q))
			}
		}
		if len(poly) < 3 {
			continue
		}
		// the edge of the polygon lying on the plane (from a point going
		// out to the next coming in) is a cut edge
		allIn := in(t[0]) && in(t[1]) && in(t[2])
		if !allIn {
			for k := range poly {
				a, b := poly[k], poly[(k+1)%len(poly)]
				if a.Z == z && b.Z == z && a != b {
					onCut = append(onCut, [2]geom.Vec3{a, b})
				}
			}
		}
		for k := 1; k+1 < len(poly); k++ {
			out = append(out, [3]geom.Vec3{poly[0], poly[k], poly[k+1]})
		}
		cut = append(cut, onCut...)
	}
	return out, cut
}

// clear: no vertex of the band starting at zs lies within a hair of
// either end's plane.
func clear(zs float64, s ThreadSpec, prof [][2]float64, n int) bool {
	eps := 1e-6 * s.P
	for _, end := range []float64{s.Z0, s.Z1} {
		for _, p := range prof {
			// i/n·P + p + zs = end, for a whole i?
			x := (end - zs - p[0]) / s.P * float64(n)
			if math.Abs(x-math.Round(x))*s.P/float64(n) < eps {
				return false
			}
		}
	}
	return true
}
