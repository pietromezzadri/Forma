package brep

// Faces of revolution sharing an axis (a revolved profile against the
// planes square to its axis, coaxial cylinders, cones, spheres, tori or
// another revolved face) meet only in circles round that axis: where the
// profile crosses or touches the other surface in a half-plane through the
// axis. Found by walking the profile, instead of contouring a signed
// distance that a profile curve only defines along its own length.

import (
	"math"
	"sort"

	"forma/geom"
)

// ssiCoaxial handles a revolved face against a face of revolution on the
// same axis; it reports whether it did.
func (c *bctx) ssiCoaxial(x, y *bface) (bool, error) {
	R, O := x, y
	rv, ok := R.s.(geom.Revolved)
	if !ok {
		R, O = y, x
		if rv, ok = R.s.(geom.Revolved); !ok {
			return false, nil
		}
	}
	o, d := rv.F.O, rv.F.Z
	if !coaxial(O.s, o, d, c.tolOn*10) {
		return false, nil
	}
	tol := c.tolOn
	g := func(t float64) float64 { return geom.SignedDist(O.s, rv.Eval(0, t)) }
	t0, t1 := rv.C.Range()
	n := 400
	ts := make([]float64, n+1)
	gs := make([]float64, n+1)
	for i := range ts {
		ts[i] = t0 + (t1-t0)*float64(i)/float64(n)
		gs[i] = g(ts[i])
	}
	var roots []float64
	add := func(t float64) {
		for _, r := range roots {
			if rv.Eval(0, r).Dist(rv.Eval(0, t)) < c.weldLoose {
				return
			}
		}
		roots = append(roots, t)
	}
	for i := 0; i < n; i++ {
		a, b := ts[i], ts[i+1]
		fa, fb := gs[i], gs[i+1]
		switch {
		case math.Abs(fa) < tol:
			add(a)
		case fa*fb < 0:
			// bisection to the crossing
			for it := 0; it < 100 && b-a > 1e-15; it++ {
				m := (a + b) / 2
				fm := g(m)
				if fa*fm <= 0 {
					b = m
				} else {
					a, fa = m, fm
				}
			}
			add((a + b) / 2)
		}
	}
	if math.Abs(gs[n]) < tol {
		add(t1)
	}
	// touches: a dip of |g| toward zero without a crossing
	for i := 1; i < n; i++ {
		if math.Abs(gs[i]) <= math.Abs(gs[i-1]) && math.Abs(gs[i]) <= math.Abs(gs[i+1]) && gs[i-1]*gs[i+1] > 0 {
			a, b := ts[i-1], ts[i+1]
			const gr = 0.6180339887498949
			for it := 0; it < 80; it++ {
				p, q := b-gr*(b-a), a+gr*(b-a)
				if math.Abs(g(p)) < math.Abs(g(q)) {
					b = q
				} else {
					a = p
				}
			}
			if t := (a + b) / 2; math.Abs(g(t)) < tol {
				add(t)
			}
		}
	}
	for _, t := range roots {
		p := rv.Eval(0, t)
		h := p.Sub(o).Dot(d)
		ctr := o.Add(d.Scale(h))
		r := p.Dist(ctr)
		if r < c.weldLoose {
			continue // a pole: no curve
		}
		f := geom.Frame{O: ctr, X: rv.F.X, Y: rv.F.Y, Z: d}
		circ := geom.Circle{F: f, R: r}
		m := int(math.Ceil(2 * math.Pi / geom.ArcStep(r, c.ctol*0.5)))
		if m < 16 {
			m = 16
		}
		// trimmed first on the face covering less of a turn (a full face
		// opens the circle at its seam, which a part face may wrap wrongly),
		// starting outside it so each piece runs between two crossings
		D, E := R, O
		if turnShareAbout(O, rv.F) < turnShare(R) {
			D, E = O, R
		}
		// (just before it enters, along the way it runs: an outside start
		// is placed on the periodic copy nearest the face, which has to be
		// the one it then runs into)
		start := 1.2345
		const steps = 256
		inD := func(a float64) bool { _, in := D.place(D.uvRaw(circ.Eval(a))); return in }
		for k := 0; k < steps; k++ {
			a := 1.2345 + 2*math.Pi*float64(k)/steps
			if !inD(a) && inD(a+2*math.Pi/steps) {
				start = a
				break
			}
		}
		pts := make([]geom.Vec3, m+1)
		for i := 0; i < m; i++ {
			pts[i] = circ.Eval(start + 2*math.Pi*float64(i)/float64(m))
		}
		pts[m] = pts[0]
		c.addCut(D, E, pts, true, false)
	}
	return true, nil
}

// coaxial reports whether s is a surface of revolution about the axis
// through o along d (a plane square to it counts).
func coaxial(s geom.Surface, o, d geom.Vec3, tol float64) bool {
	onAxis := func(p geom.Vec3) bool {
		q := p.Sub(o)
		return q.Sub(d.Scale(q.Dot(d))).Len() < tol
	}
	par := func(z geom.Vec3) bool { return math.Abs(math.Abs(z.Dot(d))-1) < 1e-9 }
	switch k := s.(type) {
	case geom.Plane:
		return par(k.F.Z)
	case geom.Cylinder:
		return par(k.F.Z) && onAxis(k.F.O)
	case geom.Cone:
		return par(k.F.Z) && onAxis(k.F.O)
	case geom.Torus:
		return par(k.F.Z) && onAxis(k.F.O)
	case geom.Revolved:
		return par(k.F.Z) && onAxis(k.F.O)
	case geom.Sphere:
		return onAxis(k.F.O)
	}
	return false
}

// turnShare is how much of a turn a face's periodic parameter covers (2
// for a face that isn't periodic).
// turnShareAbout is turnShare, a face that isn't periodic measured by
// the angle its points span round the axis of frame a (a narrow wall
// standing on a drum covers a sliver of the turn: trimmed first, the
// circle's piece on it never runs along the drum's other edges).
func turnShareAbout(f *bface, a geom.Frame) float64 {
	if f.pu > 0 || len(f.mpts) == 0 {
		return turnShare(f)
	}
	var as []float64
	for _, p := range f.mpts {
		l := a.ToLocal(p)
		if math.Hypot(l.X, l.Y) < 1e-9 {
			return 2 // round the axis itself
		}
		as = append(as, math.Atan2(l.Y, l.X))
	}
	sort.Float64s(as)
	gap := as[0] + 2*math.Pi - as[len(as)-1]
	for i := 1; i < len(as); i++ {
		gap = math.Max(gap, as[i]-as[i-1])
	}
	return (2*math.Pi - gap) / (2 * math.Pi)
}

func turnShare(f *bface) float64 {
	if f.pu <= 0 {
		return 2
	}
	return (f.hi.X - f.lo.X) / f.pu
}
