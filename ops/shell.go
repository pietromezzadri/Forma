package ops

import (
	"fmt"
	"math"

	"forma/brep"
	"forma/geom"
)

// Shell hollows a solid to walls of thickness t, leaving the open faces
// open: the solid less a copy of itself with every face moved in by t (and
// the open faces out by t, so the copy breaks through them). The copy has
// the solid's topology, which holds while t is smaller than its features.
// The inner faces are named "in/<name>".
func Shell(b *brep.Body, open map[*brep.Face]bool, t float64) (*brep.Body, error) {
	if t <= geom.Tol {
		return nil, fmt.Errorf("the thickness must be more than 0")
	}
	inner, err := Offset(b, func(f *brep.Face) float64 {
		if open[f] {
			return t
		}
		return -t
	}, func(n string) string { return "in/" + n })
	if err != nil {
		return nil, fmt.Errorf("can't hollow it %.4g thick: %v", t, err)
	}
	r, err := brep.Boolean(b, inner, brep.Subtract)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// Offset copies a body with each face moved along its outward normal by
// dist(face), vertices and edges re-found where the moved faces meet.
func Offset(b *brep.Body, dist func(*brep.Face) float64, rename func(string) string) (*brep.Body, error) {
	surf := map[*brep.Face]geom.Surface{}
	for _, f := range b.Faces() {
		s, err := offsetSurface(f, dist(f))
		if err != nil {
			return nil, err
		}
		surf[f] = s
	}
	scale := math.Max(b.Bounds().Diagonal(), 1)
	tol := 1e-9 * scale
	bd := brep.NewBuilder(b.Name)
	vmap := map[*brep.Vertex]*brep.Vertex{}
	around := map[*brep.Vertex][]*brep.Face{}
	for _, f := range b.Faces() {
		for _, l := range f.Loops {
			for _, h := range l.HalfEdges() {
				v := h.Start()
				if !hasFace(around[v], f) {
					around[v] = append(around[v], f)
				}
			}
		}
	}
	for _, v := range b.Vertices() {
		var ss []geom.Surface
		guess := v.P
		for _, f := range around[v] {
			ss = append(ss, surf[f])
			u, w := f.Surf.Project(v.P)
			guess = guess.Add(f.OutwardNormal(u, w).Scale(dist(f) / float64(len(around[v]))))
		}
		p, ok := ontoAll(ss, guess, tol)
		if !ok {
			return nil, fmt.Errorf("the faces round (%.3f, %.3f, %.3f) don't meet once moved", v.P.X, v.P.Y, v.P.Z)
		}
		vmap[v] = bd.Vertex(p)
	}
	emap := map[*brep.Edge]*brep.Edge{}
	for _, e := range b.Edges() {
		f1, f2 := e.HE[0].Loop.Face, e.HE[1].Loop.Face
		s1, s2 := surf[f1], surf[f2]
		move := func(p geom.Vec3) (geom.Vec3, bool) {
			u, w := f1.Surf.Project(p)
			g := p.Add(f1.OutwardNormal(u, w).Scale(dist(f1)))
			if f1 == f2 {
				uu, ww := s1.Project(g)
				return s1.Eval(uu, ww), true
			}
			u2, w2 := f2.Surf.Project(p)
			g = p.Add(f1.OutwardNormal(u, w).Scale(dist(f1) / 2)).Add(f2.OutwardNormal(u2, w2).Scale(dist(f2) / 2))
			return ontoAll([]geom.Surface{s1, s2}, g, tol)
		}
		va, vb := vmap[e.Verts[0]], vmap[e.Verts[1]]
		var ne *brep.Edge
		switch c := e.Curve.(type) {
		case geom.Line:
			ne = bd.LineEdge(va, vb)
			if va == vb || va.P.Dist(vb.P) < tol {
				return nil, fmt.Errorf("an edge shrinks to nothing")
			}
		case geom.Circle:
			var q [3]geom.Vec3
			for i, th := range []float64{0, 2 * math.Pi / 3, 4 * math.Pi / 3} {
				p, ok := move(c.Eval(th))
				if !ok {
					return nil, fmt.Errorf("a circular edge has no place on the moved faces")
				}
				q[i] = p
			}
			nc, ok := circleThrough(q, c)
			if !ok {
				return nil, fmt.Errorf("a circular edge shrinks to nothing")
			}
			// its ends moved round it with the faces they meet
			t0 := nearTurn(nc.Project(va.P), e.T0)
			t1 := t0 + (e.T1 - e.T0)
			if va != vb {
				t1 = nearTurn(nc.Project(vb.P), e.T1)
				if (t1-t0)*(e.T1-e.T0) <= 0 || math.Abs(t1-t0) >= 2*math.Pi {
					return nil, fmt.Errorf("an arc turns inside out")
				}
			}
			ne = bd.Edge(va, vb, nc, t0, t1)
		default:
			pts := geom.SampleParams(e.Curve, e.T0, e.T1, scale*1e-4, scale)
			var mp []geom.Vec3
			for i, t := range pts {
				var p geom.Vec3
				switch i {
				case 0:
					p = va.P
				case len(pts) - 1:
					p = vb.P
				default:
					var ok bool
					p, ok = move(e.Curve.Eval(t))
					if !ok {
						return nil, fmt.Errorf("an edge has no place on the moved faces")
					}
				}
				mp = append(mp, p)
			}
			ct := 0.0
			if tc, ok := e.Curve.(geom.Toleranced); ok {
				ct = tc.CurveTol()
			}
			// the ends moved along it: keep what lies between them
			if len(mp) > 2 && va != vb {
				inner := geom.NewPolyline(mp[1:len(mp)-1], 0)
				ta, tb := inner.Project(va.P), inner.Project(vb.P)
				kept := []geom.Vec3{va.P}
				for i, t := range inner.Params() {
					if t > ta+1e-9 && t < tb-1e-9 {
						kept = append(kept, inner.Pts[i])
					}
				}
				if ta > tb { // the ends crossed
					kept = kept[:1]
				}
				mp = append(kept, vb.P)
			}
			pl := geom.NewPolyline(mp, ct)
			ne = bd.Edge(va, vb, pl, 0, pl.Length())
		}
		emap[e] = ne
	}
	for _, f := range b.Faces() {
		var loops [][]brep.Use
		for _, l := range f.Loops {
			var uses []brep.Use
			for _, h := range l.HalfEdges() {
				uses = append(uses, brep.Use{E: emap[h.Edge], Fwd: h.Fwd})
			}
			loops = append(loops, uses)
		}
		name := f.Name
		if rename != nil {
			name = rename(name)
		}
		bd.Face(surf[f], f.Reversed, name, loops...)
	}
	return bd.Finish()
}

func hasFace(fs []*brep.Face, f *brep.Face) bool {
	for _, g := range fs {
		if g == f {
			return true
		}
	}
	return false
}

// offsetSurface is f's surface moved by d along f's outward normal.
func offsetSurface(f *brep.Face, d float64) (geom.Surface, error) {
	e := d
	if f.Reversed {
		e = -d
	}
	switch s := f.Surf.(type) {
	case geom.Plane:
		s.F.O = s.F.O.Add(s.F.Z.Scale(e))
		return s, nil
	case geom.Cylinder:
		s.R += e
		if s.R <= geom.Tol {
			return nil, fmt.Errorf("a cylinder of radius %.4g can't be moved in by %.4g", s.R-e, math.Abs(e))
		}
		return s, nil
	case geom.Sphere:
		s.R += e
		if s.R <= geom.Tol {
			return nil, fmt.Errorf("a sphere of radius %.4g can't be moved in by %.4g", s.R-e, math.Abs(e))
		}
		return s, nil
	case geom.Torus:
		s.Rm += e
		if s.Rm <= geom.Tol {
			return nil, fmt.Errorf("a torus' tube of radius %.4g can't be moved in by %.4g", s.Rm-e, math.Abs(e))
		}
		return s, nil
	case geom.Cone:
		s.R += e * math.Sqrt(1+math.Tan(s.A)*math.Tan(s.A))
		return s, nil
	}
	return nil, fmt.Errorf("can't offset a %s", f.Surf.Kind())
}

// ontoAll moves p onto every surface (Gauss–Newton on their signed
// distances, the least move when they don't pin it down; surfaces that
// coincide count once).
func ontoAll(ss []geom.Surface, p geom.Vec3, tol float64) (geom.Vec3, bool) {
	h := 1e-6 * (1 + p.Len()*1e-3)
	for it := 0; it < 30; it++ {
		var rows []geom.Vec3
		var vals []float64
		worst := 0.0
		for _, s := range ss {
			f := geom.SignedDist(s, p)
			worst = math.Max(worst, math.Abs(f))
			g := geom.DistGrad(s, p, h)
			// drop a row that repeats an earlier one (the same surface twice)
			dup := false
			for k, r := range rows {
				if math.Abs(math.Abs(r.Dot(g))-r.Len()*g.Len()) < 1e-9*r.Len()*g.Len() &&
					math.Abs(math.Abs(vals[k])-math.Abs(f)) < 1e-9+1e-6*math.Abs(f) {
					dup = true
					break
				}
			}
			if !dup {
				rows = append(rows, g)
				vals = append(vals, f)
			}
		}
		if worst < tol {
			return p, true
		}
		step, ok := minNormStep(rows, vals)
		if !ok {
			return p, false
		}
		p = p.Add(step)
	}
	for _, s := range ss {
		if math.Abs(geom.SignedDist(s, p)) > tol*100 {
			return p, false
		}
	}
	return p, true
}

// minNormStep solves J δ = −f for the least δ (J's rows rows), in the
// least-squares sense when there are more than three.
func minNormStep(rows []geom.Vec3, f []float64) (geom.Vec3, bool) {
	m := len(rows)
	if m == 0 {
		return geom.Vec3{}, true
	}
	if m > 3 {
		// normal equations JᵀJ δ = −Jᵀ f
		var a [3][3]float64
		var b [3]float64
		for i, r := range rows {
			v := [3]float64{r.X, r.Y, r.Z}
			for j := 0; j < 3; j++ {
				for k := 0; k < 3; k++ {
					a[j][k] += v[j] * v[k]
				}
				b[j] -= v[j] * f[i]
			}
		}
		x, ok := solve(a[:], b[:])
		if !ok {
			return geom.Vec3{}, false
		}
		return geom.V3(x[0], x[1], x[2]), true
	}
	// (J Jᵀ) λ = −f, δ = Jᵀ λ
	a := make([][3]float64, m)
	b := make([]float64, m)
	for i := range rows {
		for j := range rows {
			a[i][j] = rows[i].Dot(rows[j])
		}
		b[i] = -f[i]
	}
	lam, ok := solve(a, b)
	if !ok {
		return geom.Vec3{}, false
	}
	var d geom.Vec3
	for i, r := range rows {
		d = d.Add(r.Scale(lam[i]))
	}
	return d, true
}

// solve is Gaussian elimination with partial pivoting on an n×n system
// (n ≤ 3, the rows' first n entries).
func solve(a [][3]float64, b []float64) ([]float64, bool) {
	n := len(b)
	m := make([][4]float64, n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			m[i][j] = a[i][j]
		}
		m[i][3] = b[i]
	}
	for c := 0; c < n; c++ {
		p := c
		for r := c + 1; r < n; r++ {
			if math.Abs(m[r][c]) > math.Abs(m[p][c]) {
				p = r
			}
		}
		if math.Abs(m[p][c]) < 1e-14 {
			return nil, false
		}
		m[c], m[p] = m[p], m[c]
		for r := 0; r < n; r++ {
			if r == c {
				continue
			}
			k := m[r][c] / m[c][c]
			for j := c; j < n; j++ {
				m[r][j] -= k * m[c][j]
			}
			m[r][3] -= k * m[c][3]
		}
	}
	x := make([]float64, n)
	for i := 0; i < n; i++ {
		x[i] = m[i][3] / m[i][i]
	}
	return x, true
}

// circleThrough is the circle through three points, set up so its
// parameters run as c's do (the same angle at the same place).
func circleThrough(q [3]geom.Vec3, c geom.Circle) (geom.Circle, bool) {
	a, b, d := q[0], q[1], q[2]
	ab, ad := b.Sub(a), d.Sub(a)
	n := ab.Cross(ad)
	nn := n.Dot(n)
	if nn < 1e-24 {
		return geom.Circle{}, false
	}
	// circumcentre
	o := a.Add(n.Cross(ab).Scale(ad.Dot(ad)).Add(ad.Cross(n).Scale(ab.Dot(ab))).Scale(1 / (2 * nn)))
	r := a.Dist(o)
	if n.Dot(c.F.Z) < 0 {
		n = n.Neg()
	}
	X := a.Sub(o).Norm() // q[0] was at angle 0
	Z := n.Norm()
	return geom.Circle{F: geom.Frame{O: o, X: X, Y: Z.Cross(X), Z: Z}, R: r}, true
}

// nearTurn is angle a moved by whole turns to lie nearest ref.
func nearTurn(a, ref float64) float64 {
	return a + 2*math.Pi*math.Round((ref-a)/(2*math.Pi))
}
