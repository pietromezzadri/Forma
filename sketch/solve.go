package sketch

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"forma/geom"
)

// residual tolerance for "solved" (mm / radians)
const solveTol = 1e-9

type block struct {
	c      *Constraint
	params []int
	row, n int
}

type soft struct {
	par    int
	target float64
	weight float64
}

func (s *Sketch) blocks() ([]block, int) {
	var bs []block
	row := 0
	for _, c := range s.Cons {
		n := len(s.residuals(c, nil))
		if n == 0 {
			continue
		}
		bs = append(bs, block{c: c, params: s.conParams(c), row: row, n: n})
		row += n
	}
	return bs, row
}

// jacobian evaluates residuals r and J (rows × free columns) by central
// differences over each constraint's own parameters.
func (s *Sketch) jacobian(bs []block, m int, col map[int]int, nfree int, softs []soft) (r []float64, J [][]float64) {
	rows := m + len(softs)
	r = make([]float64, 0, rows)
	J = make([][]float64, rows)
	for i := range J {
		J[i] = make([]float64, nfree)
	}
	for _, b := range bs {
		r = s.residuals(b.c, r)
		for _, p := range b.params {
			j, ok := col[p]
			if !ok {
				continue
			}
			x := s.Params[p]
			h := 1e-7 * math.Max(1, math.Abs(x))
			s.Params[p] = x + h
			rp := s.residuals(b.c, nil)
			s.Params[p] = x - h
			rm := s.residuals(b.c, nil)
			s.Params[p] = x
			for k := 0; k < b.n && k < len(rp) && k < len(rm); k++ {
				d := rp[k] - rm[k]
				if b.c.Kind == Angle {
					d = math.Remainder(d, 2*math.Pi)
				}
				J[b.row+k][j] = d / (2 * h)
			}
		}
	}
	for i, sf := range softs {
		r = append(r, sf.weight*(s.Params[sf.par]-sf.target))
		if j, ok := col[sf.par]; ok {
			J[m+i][j] = sf.weight
		}
	}
	return r, J
}

func sumSq(r []float64) float64 {
	t := 0.0
	for _, v := range r {
		t += v * v
	}
	return t
}

func maxAbs(r []float64) float64 {
	t := 0.0
	for _, v := range r {
		t = math.Max(t, math.Abs(v))
	}
	return t
}

// lm runs Levenberg–Marquardt over the free parameters (those not locked).
// It returns whether the hard equations reached solveTol.
func (s *Sketch) lm(locked map[int]bool, softs []soft, maxIter int) (bool, int, float64) {
	bs, m := s.blocks()
	col := map[int]int{}
	var free []int
	for i := range s.Params {
		if !locked[i] {
			col[i] = len(free)
			free = append(free, i)
		}
	}
	n := len(free)
	hard := func(r []float64) []float64 { return r[:m] }
	r, J := s.jacobian(bs, m, col, n, softs)
	if m == 0 && len(softs) == 0 {
		return true, 0, 0
	}
	lambda := 1e-3
	cost := sumSq(r)
	it := 0
	for ; it < maxIter; it++ {
		if maxAbs(hard(r)) < solveTol && len(softs) == 0 {
			break
		}
		// normal equations
		A := make([][]float64, n)
		g := make([]float64, n)
		for i := range A {
			A[i] = make([]float64, n)
		}
		for k := range r {
			row := J[k]
			for i := 0; i < n; i++ {
				if row[i] == 0 {
					continue
				}
				g[i] += row[i] * r[k]
				for j := 0; j < n; j++ {
					if row[j] != 0 {
						A[i][j] += row[i] * row[j]
					}
				}
			}
		}
		improved := false
		for tries := 0; tries < 12; tries++ {
			M := make([][]float64, n)
			for i := range M {
				M[i] = append([]float64(nil), A[i]...)
				M[i][i] += lambda*(A[i][i]+1) + 1e-12
			}
			dx := solveLinear(M, neg(g))
			if dx == nil {
				lambda *= 10
				continue
			}
			old := make([]float64, n)
			for i, p := range free {
				old[i] = s.Params[p]
				s.Params[p] += dx[i]
			}
			r2, J2 := s.jacobian(bs, m, col, n, softs)
			c2 := sumSq(r2)
			if c2 < cost || c2 < 1e-30 {
				r, J, cost = r2, J2, c2
				lambda = math.Max(lambda/4, 1e-12)
				improved = true
				break
			}
			for i, p := range free {
				s.Params[p] = old[i]
			}
			lambda *= 8
		}
		if !improved {
			break
		}
		if len(softs) > 0 && maxAbs(hard(r)) < solveTol && maxAbs(g) < 1e-14 {
			break
		}
	}
	return maxAbs(hard(r)) < solveTol*10, it, maxAbs(hard(r))
}

// solveLoosest solves after a dimension changed, moving the least connected
// geometry it can: first only the parameters of the loosest tier (see
// stiffness), then each stiffer tier as well, and last everything. So a
// diameter changed on a circle with a point riding on it moves the point,
// not the centre the circle shares with another one.
func (s *Sketch) solveLoosest() bool {
	w := s.stiffness()
	var tiers []float64
	for _, x := range w {
		found := false
		for _, t := range tiers {
			found = found || t == x
		}
		if !found {
			tiers = append(tiers, x)
		}
	}
	sort.Float64s(tiers)
	saved := append([]float64(nil), s.Params...)
	for _, t := range tiers[:max(len(tiers)-1, 0)] {
		locked := map[int]bool{}
		for i, x := range w {
			if x > t {
				locked[i] = true
			}
		}
		if ok, it, res := s.lm(locked, nil, 100); ok {
			s.analyze()
			s.Status.Converged, s.Status.Iter, s.Status.Residual = ok, it, res
			return true
		}
		copy(s.Params, saved)
	}
	return s.Solve()
}

// stiffness weighs how reluctant each parameter is to move. A point is
// stiffer the more curves it holds together (×10 for each), so a lone point
// riding on a circle gives way before the centre two circles share, and a
// loose end before a corner; radii and lone points weigh 1.
func (s *Sketch) stiffness() []float64 {
	w := make([]float64, len(s.Params))
	for i := range w {
		w[i] = 1
	}
	uses := map[int]int{}
	for _, e := range s.Ents {
		for _, p := range e.Pts {
			uses[p]++
		}
	}
	for _, e := range s.Ents {
		if e.Kind != Point || uses[e.ID] == 0 {
			continue
		}
		k := math.Pow(10, math.Min(float64(uses[e.ID]), 4))
		w[e.Par], w[e.Par+1] = k, k
	}
	return w
}

func neg(v []float64) []float64 {
	out := make([]float64, len(v))
	for i := range v {
		out[i] = -v[i]
	}
	return out
}

// solveLinear solves M x = b by Gaussian elimination with partial pivoting
// (M is changed).
func solveLinear(M [][]float64, b []float64) []float64 {
	n := len(b)
	for c := 0; c < n; c++ {
		p := c
		for r := c + 1; r < n; r++ {
			if math.Abs(M[r][c]) > math.Abs(M[p][c]) {
				p = r
			}
		}
		if math.Abs(M[p][c]) < 1e-300 {
			return nil
		}
		M[c], M[p] = M[p], M[c]
		b[c], b[p] = b[p], b[c]
		for r := c + 1; r < n; r++ {
			f := M[r][c] / M[c][c]
			if f == 0 {
				continue
			}
			for k := c; k < n; k++ {
				M[r][k] -= f * M[c][k]
			}
			b[r] -= f * b[c]
		}
	}
	x := make([]float64, n)
	for r := n - 1; r >= 0; r-- {
		v := b[r]
		for k := r + 1; k < n; k++ {
			v -= M[r][k] * x[k]
		}
		x[r] = v / M[r][r]
	}
	return x
}

// Solve satisfies every constraint, moving the geometry as little as it can.
func (s *Sketch) Solve() bool {
	ok, it, res := s.lm(nil, nil, 200)
	s.analyze()
	s.Status.Converged, s.Status.Iter, s.Status.Residual = ok, it, res
	return ok
}

// analyze finds the degrees of freedom and which entities can't move, from
// the rank and row space of the constraint Jacobian.
func (s *Sketch) analyze() {
	bs, m := s.blocks()
	col := map[int]int{}
	for i := range s.Params {
		col[i] = i
	}
	n := len(s.Params)
	_, J := s.jacobian(bs, m, col, n, nil)
	basis := rowBasis(J, n)
	s.Status.DOF = n - len(basis)
	pinned := make([]bool, n)
	for i := 0; i < n; i++ {
		t := 0.0
		for _, q := range basis {
			t += q[i] * q[i]
		}
		pinned[i] = t > 1-1e-6
	}
	fixed := map[int]bool{}
	for _, e := range s.Ents {
		ok := true
		for _, p := range s.entParams(e) {
			if !pinned[p] {
				ok = false
			}
		}
		fixed[e.ID] = ok
	}
	s.Status.Fixed = fixed
}

// Rank is the number of independent constraint equations now.
func (s *Sketch) Rank() int {
	bs, m := s.blocks()
	col := map[int]int{}
	for i := range s.Params {
		col[i] = i
	}
	_, J := s.jacobian(bs, m, col, len(s.Params), nil)
	return len(rowBasis(J, len(s.Params)))
}

func (s *Sketch) entParams(e *Entity) []int {
	switch e.Kind {
	case Point:
		return []int{e.Par, e.Par + 1}
	case Circle:
		c := s.Ent(e.Pts[0])
		return []int{c.Par, c.Par + 1, e.Par}
	}
	var out []int
	for _, p := range e.Pts {
		pe := s.Ent(p)
		out = append(out, pe.Par, pe.Par+1)
	}
	return out
}

// rowBasis is an orthonormal basis of J's row space (modified Gram–Schmidt
// run twice per row).
func rowBasis(J [][]float64, n int) [][]float64 {
	var basis [][]float64
	for _, row := range J {
		v := append([]float64(nil), row...)
		norm0 := math.Sqrt(dot(v, v))
		if norm0 < 1e-12 {
			continue
		}
		for pass := 0; pass < 2; pass++ {
			for _, q := range basis {
				d := dot(v, q)
				for i := range v {
					v[i] -= d * q[i]
				}
			}
		}
		nv := math.Sqrt(dot(v, v))
		if nv < 1e-7*norm0 {
			continue
		}
		for i := range v {
			v[i] /= nv
		}
		basis = append(basis, v)
	}
	return basis
}

func dot(a, b []float64) float64 {
	t := 0.0
	for i := range a {
		t += a[i] * b[i]
	}
	return t
}

// Errors from adding a constraint.
var (
	ErrConflict  = errors.New("over-constrained: it conflicts with the others")
	ErrRedundant = errors.New("over-constrained: it is already implied by the others")
)

// AddConstraint adds a constraint and solves. One the sketch already
// implies, or that can't be met with the others, is refused and the sketch
// left as it was. A dimension with value NaN takes the current measurement.
func (s *Sketch) AddConstraint(k ConKind, ids []int, value float64) (*Constraint, error) {
	if err := s.checkArgs(k, ids); err != nil {
		return nil, err
	}
	c := &Constraint{ID: s.id(), Kind: k, E: append([]int(nil), ids...), Value: value}
	switch k {
	case Fix:
		p := s.P(ids[0])
		c.Value, c.Value2 = p.X, p.Y
	case Tangent:
		if a, b := s.Ent(ids[0]), s.Ent(ids[1]); a.Kind != Line && b.Kind != Line {
			d := s.center(ids[0]).Dist(s.center(ids[1]))
			c.Flip = d < math.Max(s.Radius(a), s.Radius(b))
		}
	}
	if k.Dimensional() && math.IsNaN(value) {
		c.Value = s.Measure(c)
		if k == Distance || k == Radius || k == Diameter {
			c.Value = math.Abs(c.Value)
		}
	}
	before := s.Rank()
	saved := append([]float64(nil), s.Params...)
	s.Cons = append(s.Cons, c)
	if s.Rank() == before {
		// implied by the others? it is redundant if it already holds
		r := s.residuals(c, nil)
		s.Cons = s.Cons[:len(s.Cons)-1]
		if maxAbs(r) < 1e-6 {
			return nil, ErrRedundant
		}
		// try it: a dependent equation that doesn't hold is a conflict,
		// unless solving can meet it (rank can be lower at special positions)
		s.Cons = append(s.Cons, c)
		if !s.Solve() {
			s.Cons = s.Cons[:len(s.Cons)-1]
			copy(s.Params, saved)
			s.Solve()
			return nil, ErrConflict
		}
		return c, nil
	}
	if !s.Solve() {
		s.Cons = s.Cons[:len(s.Cons)-1]
		copy(s.Params, saved)
		s.Solve()
		return nil, ErrConflict
	}
	return c, nil
}

// SetValue changes a dimension and solves; a value the sketch can't take
// is refused and the old one kept.
func (s *Sketch) SetValue(id int, v float64, expr string) error {
	c := s.Con(id)
	if c == nil {
		return fmt.Errorf("no constraint %d", id)
	}
	if !c.Kind.Dimensional() {
		return fmt.Errorf("%s has no value", c.Kind)
	}
	if (c.Kind == Distance || c.Kind == Radius || c.Kind == Diameter) && v <= 0 {
		return fmt.Errorf("must be more than 0")
	}
	old, oldExpr := c.Value, c.Expr
	saved := append([]float64(nil), s.Params...)
	c.Value, c.Expr = v, expr
	if !s.solveLoosest() {
		c.Value, c.Expr = old, oldExpr
		copy(s.Params, saved)
		s.Solve()
		return ErrConflict
	}
	return nil
}

// Drag moves entity id so the grabbed point follows target, solving the
// rest round it. grab is where it was grabbed (for lines and circles).
func (s *Sketch) Drag(id int, grab, target geom.Vec2) {
	e := s.Ent(id)
	if e == nil {
		return
	}
	locked := map[int]bool{}
	saved := append([]float64(nil), s.Params...)
	var softs []soft
	switch e.Kind {
	case Point:
		s.Params[e.Par], s.Params[e.Par+1] = target.X, target.Y
		locked[e.Par], locked[e.Par+1] = true, true
	case Line, Arc:
		d := target.Sub(grab)
		pts := e.Pts
		if e.Kind == Arc {
			pts = e.Pts[:1] // drag an arc by its centre; the ends follow
		}
		for _, p := range pts {
			pe := s.Ent(p)
			s.Params[pe.Par] += d.X
			s.Params[pe.Par+1] += d.Y
			locked[pe.Par], locked[pe.Par+1] = true, true
		}
		if e.Kind == Arc {
			for _, p := range e.Pts[1:] {
				pe := s.Ent(p)
				s.Params[pe.Par] += d.X
				s.Params[pe.Par+1] += d.Y
			}
		}
	case Circle:
		r := target.Dist(s.center(id))
		s.Params[e.Par] = r
		locked[e.Par] = true
	}
	if ok, _, _ := s.lm(locked, nil, 60); ok {
		s.analyze()
		return
	}
	// can't put it there: get as near as the constraints allow
	copy(s.Params, saved)
	for p := range locked {
		softs = append(softs, soft{par: p, target: paramAfter(e, s, p, grab, target), weight: 1e-2})
	}
	s.lm(nil, softs, 60)
	s.Solve()
}

// paramAfter is the wanted value of parameter p of a dragged entity.
func paramAfter(e *Entity, s *Sketch, p int, grab, target geom.Vec2) float64 {
	switch e.Kind {
	case Point:
		if p == e.Par {
			return target.X
		}
		return target.Y
	case Circle:
		return target.Dist(s.center(e.ID))
	}
	d := target.Sub(grab)
	if (p-sParOf(s, e, p))%2 == 0 {
		return s.Params[p] + d.X
	}
	return s.Params[p] + d.Y
}

func sParOf(s *Sketch, e *Entity, p int) int {
	for _, id := range e.Pts {
		pe := s.Ent(id)
		if pe.Par == p || pe.Par+1 == p {
			return pe.Par
		}
	}
	return p
}
