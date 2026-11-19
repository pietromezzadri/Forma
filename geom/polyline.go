package geom

import "math"

// Polyline is a curve through points, parametrised by length along it. The
// boolean uses it for intersection curves; Tol says how far it may stray from
// the true curve between its points (its points lie on it).
type Polyline struct {
	Pts []Vec3
	cum []float64
	Tol float64
}

// NewPolyline builds one (closed when the last point equals the first).
func NewPolyline(pts []Vec3, tol float64) *Polyline {
	p := &Polyline{Pts: pts, Tol: tol}
	p.cum = make([]float64, len(pts))
	for i := 1; i < len(pts); i++ {
		p.cum[i] = p.cum[i-1] + pts[i].Dist(pts[i-1])
	}
	return p
}

// Length of the polyline.
func (p *Polyline) Length() float64 { return p.cum[len(p.cum)-1] }

func (p *Polyline) seg(t float64) (int, float64) {
	n := len(p.Pts)
	if t <= 0 {
		return 0, 0
	}
	if t >= p.cum[n-1] {
		return n - 2, 1
	}
	lo, hi := 0, n-1
	for hi-lo > 1 {
		m := (lo + hi) / 2
		if p.cum[m] <= t {
			lo = m
		} else {
			hi = m
		}
	}
	l := p.cum[lo+1] - p.cum[lo]
	if l == 0 {
		return lo, 0
	}
	return lo, (t - p.cum[lo]) / l
}

func (p *Polyline) Eval(t float64) Vec3 {
	i, s := p.seg(t)
	return p.Pts[i].Lerp(p.Pts[i+1], s)
}

func (p *Polyline) Deriv(t float64) Vec3 {
	i, _ := p.seg(t)
	return p.Pts[i+1].Sub(p.Pts[i]).Norm()
}

func (p *Polyline) Project(q Vec3) float64 {
	best, bt := math.Inf(1), 0.0
	for i := 0; i+1 < len(p.Pts); i++ {
		a, b := p.Pts[i], p.Pts[i+1]
		ab := b.Sub(a)
		l2 := ab.Dot(ab)
		s := 0.0
		if l2 > 0 {
			s = math.Max(0, math.Min(1, q.Sub(a).Dot(ab)/l2))
		}
		if d := a.Add(ab.Scale(s)).Dist(q); d < best {
			best, bt = d, p.cum[i]+s*math.Sqrt(l2)
		}
	}
	return bt
}

func (*Polyline) Kind() string { return "polyline" }

// Params are the parameters of the polyline's points.
func (p *Polyline) Params() []float64 { return p.cum }

// Toleranced curves lie within CurveTol of their true curve.
type Toleranced interface{ CurveTol() float64 }

func (p *Polyline) CurveTol() float64 { return p.Tol }
