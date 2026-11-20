package sketch

import (
	"math"

	"forma/geom"
)

// Bézier curves in the plane: control points c[0] … c[n] (c[0] and c[n] the
// ends), parameter t in [0, 1].

func bezAt(c []geom.Vec2, t float64) geom.Vec2 {
	w := append([]geom.Vec2(nil), c...)
	for r := 1; r < len(w); r++ {
		for j := 0; j < len(w)-r; j++ {
			w[j] = w[j].Lerp(w[j+1], t)
		}
	}
	return w[0]
}

// bezHodo is the derivative's control points (a Bézier of one degree less).
func bezHodo(c []geom.Vec2) []geom.Vec2 {
	n := len(c) - 1
	if n < 1 {
		return []geom.Vec2{{}}
	}
	d := make([]geom.Vec2, n)
	for i := range d {
		d[i] = c[i+1].Sub(c[i]).Scale(float64(n))
	}
	return d
}

func bezDeriv(c []geom.Vec2, t float64) geom.Vec2 { return bezAt(bezHodo(c), t) }

// bezSplit cuts at t (de Casteljau): the control points of [0, t] and [t, 1].
func bezSplit(c []geom.Vec2, t float64) ([]geom.Vec2, []geom.Vec2) {
	n := len(c)
	w := append([]geom.Vec2(nil), c...)
	left, right := make([]geom.Vec2, n), make([]geom.Vec2, n)
	left[0], right[n-1] = w[0], w[n-1]
	for r := 1; r < n; r++ {
		for j := 0; j < n-r; j++ {
			w[j] = w[j].Lerp(w[j+1], t)
		}
		left[r] = w[0]
		right[n-1-r] = w[n-1-r]
	}
	return left, right
}

// bezSub is the piece between t0 and t1 (t0 < t1), as its own Bézier.
func bezSub(c []geom.Vec2, t0, t1 float64) []geom.Vec2 {
	if t0 <= 0 && t1 >= 1 {
		return append([]geom.Vec2(nil), c...)
	}
	_, r := bezSplit(c, t0)
	if t0 >= 1 {
		return r
	}
	l, _ := bezSplit(r, (t1-t0)/(1-t0))
	return l
}

func bezReversed(c []geom.Vec2) []geom.Vec2 {
	out := make([]geom.Vec2, len(c))
	for i := range c {
		out[i] = c[len(c)-1-i]
	}
	return out
}

// bezCurvature is the signed curvature at t (positive turning left).
func bezCurvature(c []geom.Vec2, t float64) float64 {
	h := bezHodo(c)
	d1 := bezAt(h, t)
	d2 := geom.Vec2{}
	if len(h) > 1 {
		d2 = bezAt(bezHodo(h), t)
	}
	l := d1.Len()
	if l < 1e-12 {
		return 0
	}
	return d1.Cross(d2) / (l * l * l)
}

// bezSteps is how many even steps keep chords within tol of the curve.
func bezSteps(c []geom.Vec2, tol float64) int {
	n := len(c) - 1
	m := 0.0
	for i := 0; i+2 < len(c); i++ {
		m = math.Max(m, c[i+2].Sub(c[i+1].Scale(2)).Add(c[i]).Len())
	}
	k := int(math.Ceil(math.Sqrt(float64(n*(n-1)) * m / (8 * math.Max(tol, 1e-9)))))
	if k < 8 {
		k = 8
	}
	if k > 512 {
		k = 512
	}
	return k
}

func bezPoints(c []geom.Vec2, tol float64) []geom.Vec2 {
	k := bezSteps(c, tol)
	out := make([]geom.Vec2, k+1)
	for i := 0; i <= k; i++ {
		out[i] = bezAt(c, float64(i)/float64(k))
	}
	out[0], out[k] = c[0], c[len(c)-1]
	return out
}

// bezProject is the parameter of the curve point nearest p.
func bezProject(c []geom.Vec2, p geom.Vec2) float64 {
	n := 64
	best, bd := 0.0, math.Inf(1)
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		if d := bezAt(c, t).Dist(p); d < bd {
			best, bd = t, d
		}
	}
	h := bezHodo(c)
	h2 := []geom.Vec2{{}}
	if len(h) > 1 {
		h2 = bezHodo(h)
	}
	t := best
	for it := 0; it < 30; it++ {
		q := bezAt(c, t).Sub(p)
		d1, d2 := bezAt(h, t), bezAt(h2, t)
		f := q.Dot(d1)
		df := d1.Dot(d1) + q.Dot(d2)
		if df <= 0 {
			break
		}
		nt := math.Max(0, math.Min(1, t-f/df))
		if math.Abs(nt-t) < 1e-15 {
			t = nt
			break
		}
		t = nt
	}
	return t
}

// bezArea is ∫ (x dy − y dx) / 2 along the curve: its share of a loop's
// signed area (exact for degree ≤ 8 with eight Gauss points).
func bezArea(c []geom.Vec2) float64 {
	xs, ws := geom.GaussLegendre8()
	h := bezHodo(c)
	a := 0.0
	for i, x := range xs {
		t := (x + 1) / 2
		p, d := bezAt(c, t), bezAt(h, t)
		a += ws[i] / 2 * (p.X*d.Y - p.Y*d.X)
	}
	return a / 2
}
