package geom

import (
	"math"
	"sort"
)

// BSpline is a clamped, non-rational B-spline curve: degree Deg, knots
// (len(Ctrl)+Deg+1 of them, the first and last Deg+1 equal) and control
// points. A Bézier curve is the one-span case, with knots 0…0 1…1. The
// parameter runs over [Knots[Deg], Knots[len(Ctrl)]].
type BSpline struct {
	Deg   int
	Knots []float64
	Ctrl  []Vec3
}

// Bezier is the Bézier curve with these control points (degree one less
// than their number), parametrised over [0, 1].
func Bezier(ctrl ...Vec3) *BSpline {
	n := len(ctrl) - 1
	k := make([]float64, 0, 2*n+2)
	for i := 0; i <= n; i++ {
		k = append(k, 0)
	}
	for i := 0; i <= n; i++ {
		k = append(k, 1)
	}
	return &BSpline{Deg: n, Knots: k, Ctrl: append([]Vec3(nil), ctrl...)}
}

// Range is the curve's parameter range.
func (b *BSpline) Range() (float64, float64) {
	return b.Knots[b.Deg], b.Knots[len(b.Ctrl)]
}

// span finds the knot span holding t (index i with K[i] ≤ t < K[i+1],
// clamped to the last non-empty span at the end).
func (b *BSpline) span(t float64) int {
	n := len(b.Ctrl) - 1
	p := b.Deg
	if t >= b.Knots[n+1] {
		i := n
		for i > p && b.Knots[i] >= b.Knots[n+1] {
			i--
		}
		return i
	}
	if t <= b.Knots[p] {
		i := p
		for i < n && b.Knots[i+1] <= b.Knots[p] {
			i++
		}
		return i
	}
	i := sort.SearchFloat64s(b.Knots, t)
	// SearchFloat64s gives the first knot ≥ t
	if i < len(b.Knots) && b.Knots[i] == t {
		for i+1 < len(b.Knots) && b.Knots[i+1] == t {
			i++
		}
		if i > n {
			i = n
		}
		return i
	}
	return i - 1
}

// deBoor evaluates the curve with control points cs (same knots and degree).
func deBoor(deg int, knots []float64, cs []Vec3, t float64, k int) Vec3 {
	d := make([]Vec3, deg+1)
	for j := 0; j <= deg; j++ {
		d[j] = cs[j+k-deg]
	}
	for r := 1; r <= deg; r++ {
		for j := deg; j >= r; j-- {
			i := j + k - deg
			den := knots[i+deg-r+1] - knots[i]
			a := 0.0
			if den != 0 {
				a = (t - knots[i]) / den
			}
			d[j] = d[j-1].Scale(1 - a).Add(d[j].Scale(a))
		}
	}
	return d[deg]
}

func (b *BSpline) Eval(t float64) Vec3 {
	t0, t1 := b.Range()
	t = math.Max(t0, math.Min(t1, t))
	if b.Deg == 0 {
		return b.Ctrl[0]
	}
	return deBoor(b.Deg, b.Knots, b.Ctrl, t, b.span(t))
}

// derivative is the B-spline of the curve's first derivative.
func (b *BSpline) derivative() *BSpline {
	p := b.Deg
	if p == 0 {
		return &BSpline{Deg: 0, Knots: []float64{b.Knots[0], b.Knots[len(b.Knots)-1]}, Ctrl: []Vec3{{}}}
	}
	q := make([]Vec3, len(b.Ctrl)-1)
	for i := range q {
		den := b.Knots[i+p+1] - b.Knots[i+1]
		if den != 0 {
			q[i] = b.Ctrl[i+1].Sub(b.Ctrl[i]).Scale(float64(p) / den)
		}
	}
	return &BSpline{Deg: p - 1, Knots: b.Knots[1 : len(b.Knots)-1], Ctrl: q}
}

func (b *BSpline) Deriv(t float64) Vec3 { return b.derivative().Eval(t) }

// Deriv2 is the second derivative.
func (b *BSpline) Deriv2(t float64) Vec3 { return b.derivative().derivative().Eval(t) }

// Project finds the parameter of the curve point nearest p: the nearest of
// a sampling, then Newton steps on (c(t) − p)·c'(t) = 0.
func (b *BSpline) Project(p Vec3) float64 {
	t0, t1 := b.Range()
	d1, d2 := b.derivative(), b.derivative().derivative()
	n := 16 * (len(b.Ctrl) - b.Deg)
	if n < 32 {
		n = 32
	}
	best, bd := t0, math.Inf(1)
	for i := 0; i <= n; i++ {
		t := t0 + (t1-t0)*float64(i)/float64(n)
		if d := b.Eval(t).Dist(p); d < bd {
			best, bd = t, d
		}
	}
	t := best
	for it := 0; it < 30; it++ {
		c := b.Eval(t).Sub(p)
		c1, c2 := d1.Eval(t), d2.Eval(t)
		f := c.Dot(c1)
		df := c1.Dot(c1) + c.Dot(c2)
		if df <= 0 {
			break
		}
		nt := math.Max(t0, math.Min(t1, t-f/df))
		if math.Abs(nt-t) < 1e-15*(1+math.Abs(t1-t0)) {
			t = nt
			break
		}
		t = nt
	}
	return t
}

func (*BSpline) Kind() string { return "bspline" }

// Reversed is the same curve run the other way: parameter t ↦ t0 + t1 − t.
func (b *BSpline) Reversed() *BSpline {
	n := len(b.Ctrl)
	c := make([]Vec3, n)
	for i := range c {
		c[i] = b.Ctrl[n-1-i]
	}
	m := len(b.Knots)
	k := make([]float64, m)
	lo, hi := b.Knots[0], b.Knots[m-1]
	for i := range k {
		k[i] = lo + hi - b.Knots[m-1-i]
	}
	return &BSpline{Deg: b.Deg, Knots: k, Ctrl: c}
}

// Map applies f to the control points (any affine map maps the curve).
func (b *BSpline) Map(f func(Vec3) Vec3) *BSpline {
	c := make([]Vec3, len(b.Ctrl))
	for i, p := range b.Ctrl {
		c[i] = f(p)
	}
	return &BSpline{Deg: b.Deg, Knots: append([]float64(nil), b.Knots...), Ctrl: c}
}

// Length is the curve's arc length (Gauss–Legendre per span).
func (b *BSpline) Length() float64 {
	return b.LengthBetween(b.Range())
}

// LengthBetween is the arc length from t0 to t1.
func (b *BSpline) LengthBetween(t0, t1 float64) float64 {
	if t1 < t0 {
		t0, t1 = t1, t0
	}
	d := b.derivative()
	l := 0.0
	cuts := []float64{t0}
	for _, k := range b.Knots {
		if k > t0 && k < t1 && k != cuts[len(cuts)-1] {
			cuts = append(cuts, k)
		}
	}
	cuts = append(cuts, t1)
	for i := 0; i+1 < len(cuts); i++ {
		a, c := cuts[i], cuts[i+1]
		// split each span in four for curves that bend sharply
		for q := 0; q < 4; q++ {
			x0 := a + (c-a)*float64(q)/4
			x1 := a + (c-a)*float64(q+1)/4
			for j, x := range gaussX {
				t := (x0+x1)/2 + (x1-x0)/2*x
				l += gaussW[j] * (x1 - x0) / 2 * d.Eval(t).Len()
			}
		}
	}
	return l
}

// eight-point Gauss–Legendre nodes and weights on [−1, 1]
var gaussX = []float64{-0.9602898564975363, -0.7966664774136267, -0.5255324099163290, -0.1834346424956498,
	0.1834346424956498, 0.5255324099163290, 0.7966664774136267, 0.9602898564975363}
var gaussW = []float64{0.1012285362903763, 0.2223810344533745, 0.3137066458778873, 0.3626837833783620,
	0.3626837833783620, 0.3137066458778873, 0.2223810344533745, 0.1012285362903763}

// GaussLegendre8 returns the eight-point Gauss–Legendre rule on [−1, 1].
func GaussLegendre8() ([]float64, []float64) { return gaussX, gaussW }

// steps is how many even parameter steps per span keep the chords within
// tol of the curve: a degree-p span's chords are off by at most
// p(p−1)/8 · max|Δ²P| / N² (control-polygon bound).
func (b *BSpline) steps(tol float64) int {
	p := b.Deg
	if p <= 1 {
		return 1
	}
	m := 0.0
	for i := 0; i+2 < len(b.Ctrl); i++ {
		d := b.Ctrl[i+2].Sub(b.Ctrl[i+1].Scale(2)).Add(b.Ctrl[i])
		m = math.Max(m, d.Len())
	}
	n := int(math.Ceil(math.Sqrt(float64(p*(p-1)) * m / (8 * math.Max(tol, 1e-12)))))
	if n < 2 {
		n = 2
	}
	if n > 400 {
		n = 400
	}
	return n
}

// MinBendRadius is roughly the tightest radius of curvature along the curve
// (sampled), +Inf for a straight one.
func (b *BSpline) MinBendRadius() float64 {
	t0, t1 := b.Range()
	d1, d2 := b.derivative(), b.derivative().derivative()
	r := math.Inf(1)
	n := 24 * (len(b.Ctrl) - b.Deg)
	for i := 0; i <= n; i++ {
		t := t0 + (t1-t0)*float64(i)/float64(n)
		a, c := d1.Eval(t), d2.Eval(t)
		la := a.Len()
		k := a.Cross(c).Len()
		if la > 1e-12 && k > 1e-300 {
			r = math.Min(r, la*la*la/k)
		}
	}
	return r
}

// Split cuts a Bézier (one span) at t into two Béziers (de Casteljau).
func (b *BSpline) SplitBezier(t float64) (*BSpline, *BSpline) {
	n := len(b.Ctrl)
	work := append([]Vec3(nil), b.Ctrl...)
	left := make([]Vec3, n)
	right := make([]Vec3, n)
	left[0], right[n-1] = work[0], work[n-1]
	for r := 1; r < n; r++ {
		for j := 0; j < n-r; j++ {
			work[j] = work[j].Lerp(work[j+1], t)
		}
		left[r] = work[0]
		right[n-1-r] = work[n-1-r]
	}
	return Bezier(left...), Bezier(right...)
}

// SubBezier is the piece of a Bézier between t0 and t1 (t0 < t1), itself a
// Bézier over [0, 1].
func (b *BSpline) SubBezier(t0, t1 float64) *BSpline {
	_, r := b.SplitBezier(t0)
	if t0 >= 1 {
		return Bezier(r.Ctrl...)
	}
	l, _ := r.SplitBezier((t1 - t0) / (1 - t0))
	return l
}
