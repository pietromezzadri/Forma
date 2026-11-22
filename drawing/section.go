package drawing

import (
	"fmt"
	"math"
	"sort"

	"forma/brep"
	"forma/geom"
	"forma/ops"
)

// Section cuts the bodies by the plane through o with normal n and keeps
// what lies beyond it, the side n points to (the view looks along n, so
// the cut faces it). The faces on the plane come back as regions to hatch.
func Section(bodies []Body, o, n geom.Vec3, fr Frame, opt Options) *Projection {
	n = n.Norm()
	var all geom.Box
	for _, b := range bodies {
		all.Union(b.Body.Bounds())
	}
	L := math.Max(all.Diagonal(), 1) * 2
	var kept []Body
	var warn []string
	for _, b := range bodies {
		lo, hi := spanAlong(b.Body.Bounds(), o, n)
		switch {
		case hi <= 1e-9:
			continue // all of it cut away
		case lo >= -1e-9:
			kept = append(kept, b)
			continue
		}
		cut, err := halfOf(b.Body, o, n, L)
		if err != nil {
			// a cut through faces lying on the plane can trip the booleans:
			// try a hair beyond it
			cut, err = halfOf(b.Body, o.Add(n.Scale(1e-3)), n, L)
		}
		if err != nil {
			warn = append(warn, fmt.Sprintf("%s: %v", b.Name, err))
			continue
		}
		kept = append(kept, Body{Key: b.Key, Name: b.Name, Body: cut, Gen: b.Gen})
	}
	pr := Project(kept, fr, opt)
	pr.Bodies = kept
	if len(warn) > 0 {
		pr.Warn = "not cut: " + joinComma(warn)
	}
	tol := math.Max(all.Diagonal(), 1) * 1e-6
	for _, b := range kept {
		for _, f := range b.Body.Faces() {
			pl, ok := f.Surf.(geom.Plane)
			if !ok || math.Abs(pl.F.Z.Dot(n)) < 1-1e-9 || math.Abs(pl.F.O.Sub(o).Dot(n)) > 2e-3+tol {
				continue
			}
			r := Region{Body: b.Key}
			for _, l := range f.Loops {
				if lp := loopPoints(l, fr, tol); len(lp) >= 3 {
					r.Loops = append(r.Loops, lp)
				}
			}
			if len(r.Loops) > 0 {
				pr.Regions = append(pr.Regions, r)
			}
		}
	}
	return pr
}

func joinComma(s []string) string {
	out := ""
	for i, x := range s {
		if i > 0 {
			out += "; "
		}
		out += x
	}
	return out
}

// spanAlong is how far a box reaches either side of the plane.
func spanAlong(b geom.Box, o, n geom.Vec3) (float64, float64) {
	lo, hi := math.Inf(1), math.Inf(-1)
	for i := 0; i < 8; i++ {
		p := geom.V3(pick(i&1, b.Min.X, b.Max.X), pick(i&2, b.Min.Y, b.Max.Y), pick(i&4, b.Min.Z, b.Max.Z))
		d := p.Sub(o).Dot(n)
		lo, hi = math.Min(lo, d), math.Max(hi, d)
	}
	return lo, hi
}

func pick(bit int, a, b float64) float64 {
	if bit != 0 {
		return b
	}
	return a
}

// halfOf is body ∩ the half-space beyond the plane.
func halfOf(b *brep.Body, o, n geom.Vec3, L float64) (*brep.Body, error) {
	f := geom.FrameZ(o, n, n.AnyPerp())
	box, err := ops.Box(geom.V3(-L, -L, 0), geom.V3(L, L, L))
	if err != nil {
		return nil, err
	}
	box, err = brep.Transform(box, geom.Xform{R: [3]geom.Vec3{f.X, f.Y, f.Z}, T: f.O}, nil)
	if err != nil {
		return nil, err
	}
	return brep.Boolean(b, box, brep.Intersect)
}

// loopPoints is a face loop as a polygon in the view.
func loopPoints(l *brep.Loop, fr Frame, tol float64) []geom.Vec2 {
	var out []geom.Vec2
	for _, h := range l.HalfEdges() {
		e := h.Edge
		ts := geom.SampleParams(e.Curve, e.T0, e.T1, 0.01, 1e9)
		if !h.Fwd {
			for i, j := 0, len(ts)-1; i < j; i, j = i+1, j-1 {
				ts[i], ts[j] = ts[j], ts[i]
			}
		}
		for _, t := range ts[:len(ts)-1] {
			p := fr.P(e.Curve.Eval(t))
			if len(out) == 0 || out[len(out)-1].Dist(p) > tol {
				out = append(out, p)
			}
		}
	}
	return out
}

// Hatch fills regions with lines at angle (radians) spacing apart, even–odd
// between the loops (holes stay clear). Points are in whatever units the
// loops are.
func Hatch(loops [][]geom.Vec2, angle, spacing float64) [][2]geom.Vec2 {
	if spacing <= 0 {
		return nil
	}
	d := geom.V2(math.Cos(angle), math.Sin(angle)) // along the lines
	nrm := d.Perp()                                // across them
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, l := range loops {
		for _, p := range l {
			s := p.Dot(nrm)
			lo, hi = math.Min(lo, s), math.Max(hi, s)
		}
	}
	if lo > hi {
		return nil
	}
	var out [][2]geom.Vec2
	start := math.Ceil(lo/spacing) * spacing
	for s := start; s <= hi; s += spacing {
		var xs []float64
		for _, l := range loops {
			for i := range l {
				a, b := l[i], l[(i+1)%len(l)]
				sa, sb := a.Dot(nrm)-s, b.Dot(nrm)-s
				if (sa > 0) == (sb > 0) {
					continue
				}
				t := sa / (sa - sb)
				xs = append(xs, a.Lerp(b, t).Dot(d))
			}
		}
		sort.Float64s(xs)
		for i := 0; i+1 < len(xs); i += 2 {
			if xs[i+1]-xs[i] < 1e-9 {
				continue
			}
			base := nrm.Scale(s)
			out = append(out, [2]geom.Vec2{base.Add(d.Scale(xs[i])), base.Add(d.Scale(xs[i+1]))})
		}
	}
	return out
}
