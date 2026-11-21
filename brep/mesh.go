package brep

import (
	"math"

	"forma/geom"
)

// Mesh is a body's display / export tessellation: triangles tagged with the
// face they came from, and each edge as a polyline.
type Mesh struct {
	Pos     []geom.Vec3
	Nrm     []geom.Vec3
	Tris    [][3]int32
	TriFace []int // face ID of each triangle
	Edges   []EdgeLine
	Verts   []VertexPoint
}

type EdgeLine struct {
	ID  int
	Pts []geom.Vec3
}

type VertexPoint struct {
	ID int
	P  geom.Vec3
}

// MeshOptions bound the tessellation error.
type MeshOptions struct {
	ChordTol float64 // max distance from the true surface (mm)
	MaxEdge  float64 // max boundary segment length (mm)
}

// DefaultMeshOptions scales the tolerances to the body's size.
func DefaultMeshOptions(b *Body) MeshOptions {
	d := b.Bounds().Diagonal()
	if d <= 0 {
		d = 1
	}
	return MeshOptions{ChordTol: math.Max(d*2e-4, 1e-4), MaxEdge: d / 8}
}

type edgeSamples struct {
	ts  []float64
	pts []geom.Vec3
}

// Tessellate meshes every face of b. Neighbouring faces share their edge
// samples exactly, so the mesh is watertight.
func Tessellate(b *Body, opt MeshOptions) *Mesh {
	m := &Mesh{}
	samples := map[*Edge]*edgeSamples{}
	for _, e := range b.Edges() {
		ts := geom.SampleParams(e.Curve, e.T0, e.T1, opt.ChordTol, opt.MaxEdge)
		s := &edgeSamples{ts: ts}
		for i, t := range ts {
			p := e.Curve.Eval(t)
			if i == 0 {
				p = e.Verts[0].P
			} else if i == len(ts)-1 {
				p = e.Verts[1].P
			}
			s.pts = append(s.pts, p)
		}
		samples[e] = s
		m.Edges = append(m.Edges, EdgeLine{ID: e.ID, Pts: s.pts})
	}
	for _, v := range b.Vertices() {
		m.Verts = append(m.Verts, VertexPoint{ID: v.ID, P: v.P})
	}
	for _, f := range b.Faces() {
		meshFace(m, f, samples, opt)
	}
	return m
}

// a boundary point of a face in its parameter space
type bpt struct {
	uv    geom.Vec2
	p     geom.Vec3
	exact bool // p is an edge sample (keep it, so faces meet)
}

func minRadius(s geom.Surface) float64 {
	switch k := s.(type) {
	case geom.Cylinder:
		return k.R
	case geom.Cone:
		return math.Max(math.Abs(k.R), 1e-3)
	case geom.Sphere:
		return k.R
	case geom.Torus:
		return k.Rm
	case geom.Revolved:
		return k.BendRadius()
	case geom.Extruded:
		if r := k.BendRadius(); !math.IsInf(r, 1) {
			return r
		}
	}
	return 0
}

func meshFace(m *Mesh, f *Face, samples map[*Edge]*edgeSamples, opt MeshOptions) {
	s := f.Surf
	su, sv := s.Metric()
	pu, pv := s.PeriodU(), s.PeriodV()

	var loops [][]bpt
	for _, l := range f.Loops {
		for _, run := range loopRuns(l, pu == 0 && pv == 0) {
			if lp := loopParams(run, s, samples, pu, pv, f.Reversed); len(lp) >= 2 {
				loops = append(loops, lp)
			}
		}
	}
	if len(loops) == 0 {
		return
	}

	// interior spacing in the metric parameter space
	h := 0.0
	if r := minRadius(s); r > 0 {
		h = r * geom.ArcStep(r, opt.ChordTol)
		if h > opt.MaxEdge {
			h = opt.MaxEdge
		}
	}

	// close gaps (poles) in parameter space, then gather points and segments
	var pts []geom.Vec2
	var info []bpt
	var segs [][2]int
	metric := func(uv geom.Vec2) geom.Vec2 { return geom.Vec2{X: uv.X * su, Y: uv.Y * sv} }
	gapStep := h
	if gapStep == 0 {
		gapStep = opt.MaxEdge
	}
	for _, lp := range loops {
		start := len(pts)
		for i, a := range lp {
			pts = append(pts, metric(a.uv))
			info = append(info, a)
			b := lp[(i+1)%len(lp)]
			// a gap between a and b with no edge: points along it, evaluated on the surface
			if !b.exact || !a.exact || a.p.Dist(b.p) > geom.Tol {
				continue
			}
			ma, mb := metric(a.uv), metric(b.uv)
			if d := ma.Dist(mb); d > gapStep {
				n := int(math.Ceil(d / gapStep))
				for k := 1; k < n; k++ {
					uv := a.uv.Lerp(b.uv, float64(k)/float64(n))
					pts = append(pts, metric(uv))
					info = append(info, bpt{uv: uv, p: s.Eval(uv.X, uv.Y)})
				}
			}
		}
		for i := start; i < len(pts); i++ {
			j := i + 1
			if j == len(pts) {
				j = start
			}
			segs = append(segs, [2]int{i, j})
		}
	}
	nBoundary := len(pts)

	if h > 0 {
		var box geom.Box
		for _, p := range pts {
			box.Add(geom.Vec3{X: p.X, Y: p.Y})
		}
		keep := 0.45 * h
		idx := newSegIndex(pts, segs, keep)
		inside := geom.NewSegBands(pts[:nBoundary], segs)
		// poles and apexes: the parameter grid crowds there, so keep interior
		// points a spacing away from them in space
		var poles []geom.Vec3
		for _, in := range info {
			if s.Singular(in.uv.X, in.uv.Y) {
				poles = append(poles, in.p)
			}
		}
		nearPole := func(p geom.Vec3) bool {
			for _, q := range poles {
				if q.Dist(p) < keep {
					return true
				}
			}
			return false
		}
		row := 0
		for y0 := box.Min.Y + h*0.5; y0 < box.Max.Y; y0 += h {
			row++
			col := 0
			for x0 := box.Min.X + h*0.5; x0 < box.Max.X; x0 += h {
				col++
				// a small fixed jitter keeps grid points off common circles,
				// which Delaunay insertion handles badly
				x := x0 + h*0.08*hashUnit(row, col, 1)
				y := y0 + h*0.08*hashUnit(row, col, 2)
				q := geom.Vec2{X: x, Y: y}
				if !inside.Inside(q) {
					continue
				}
				if idx.near(q, pts, keep) {
					continue
				}
				uv := geom.Vec2{X: x / su, Y: y / sv}
				p := s.Eval(uv.X, uv.Y)
				if nearPole(p) {
					continue
				}
				pts = append(pts, q)
				info = append(info, bpt{uv: uv, p: p})
			}
		}
	}

	tris := geom.Triangulate(pts, segs)
	base := int32(len(m.Pos))
	for _, in := range info {
		m.Pos = append(m.Pos, in.p)
		m.Nrm = append(m.Nrm, f.OutwardNormal(in.uv.X, in.uv.Y))
	}
	for _, t := range tris {
		a, b, c := info[t[0]].p, info[t[1]].p, info[t[2]].p
		if b.Sub(a).Cross(c.Sub(a)).Len() < 1e-14 {
			continue // degenerate at a pole
		}
		tri := [3]int32{base + int32(t[0]), base + int32(t[1]), base + int32(t[2])}
		if f.Reversed {
			tri[1], tri[2] = tri[2], tri[1]
		}
		m.Tris = append(m.Tris, tri)
		m.TriFace = append(m.TriFace, f.ID)
	}
}

// hashUnit is a repeatable pseudo-random number in [-1, 1].
func hashUnit(a, b, c int) float64 {
	h := uint32(a)*73856093 ^ uint32(b)*19349663 ^ uint32(c)*83492791
	h ^= h >> 13
	h *= 0x5bd1e995
	h ^= h >> 15
	return float64(h%20001)/10000 - 1
}

func nearSegs(p geom.Vec2, pts []geom.Vec2, segs [][2]int, d float64) bool {
	for _, s := range segs {
		a, b := pts[s[0]], pts[s[1]]
		ab := b.Sub(a)
		t := 0.0
		if l2 := ab.Dot(ab); l2 > 0 {
			t = math.Max(0, math.Min(1, p.Sub(a).Dot(ab)/l2))
		}
		if a.Add(ab.Scale(t)).Dist(p) < d {
			return true
		}
	}
	return false
}

// loopParams walks a loop's half-edges and returns its points in the face's
// parameter space, unwrapped across periodic seams so the loop is a plain
// polygon there. The last sample of each half-edge is left out (it is the
// next one's first).
// heUV is one half-edge of a loop in a face's parameter space: its samples
// (in the half-edge's direction) with their edge parameters, 3D points and
// unwrapped (u, v).
type heUV struct {
	h    *HalfEdge
	ts   []float64
	pts  []geom.Vec3
	uv   []geom.Vec2
	sing []bool
}

func loopParams(hs []*HalfEdge, s geom.Surface, samples map[*Edge]*edgeSamples, pu, pv float64, reversed bool) []bpt {
	var out []bpt
	for _, e := range unwrapLoop(hs, s, samples, pu, pv, reversed) {
		n := len(e.uv)
		last := n - 1
		if e.sing[n-1] {
			// a pole at the joint: keep this side's copy of it, so the gap to
			// the next half-edge's copy (at another u) is filled along the pole
			last = n
		}
		for i := 0; i < last; i++ {
			if k := len(out); k > 0 && out[k-1].uv.Dist(e.uv[i]) < 1e-12 {
				continue
			}
			out = append(out, bpt{uv: e.uv[i], p: e.pts[i], exact: true})
		}
	}
	if k := len(out); k > 1 && out[k-1].uv.Dist(out[0].uv) < 1e-12 {
		out = out[:k-1]
	}
	return out
}

// unwrapLoop maps a loop's half-edges into the face's parameter space,
// unwrapped across periodic seams so the loop is a plain polygon there
// (starting at a seam use, if any).
func unwrapLoop(hs []*HalfEdge, s geom.Surface, samples map[*Edge]*edgeSamples, pu, pv float64, reversed bool) []heUV {
	if len(hs) == 0 {
		return nil
	}
	uses := map[*Edge]int{}
	for _, h := range hs {
		uses[h.Edge]++
	}
	// start at a seam use, so seams fix the unwrapping
	start := 0
	for i, h := range hs {
		if uses[h.Edge] == 2 && (pu > 0 || pv > 0) {
			start = i
			break
		}
	}
	hs = append(hs[start:], hs[:start]...)

	var res []heUV
	var prevEnd *geom.Vec2
	prevPole, prevDV := false, 0.0
	for _, h := range hs {
		es := samples[h.Edge]
		n := len(es.pts)
		pts := make([]geom.Vec3, n)
		ts := make([]float64, n)
		for i := 0; i < n; i++ {
			if h.Fwd {
				pts[i], ts[i] = es.pts[i], es.ts[i]
			} else {
				pts[i], ts[i] = es.pts[n-1-i], es.ts[n-1-i]
			}
		}
		uv := make([]geom.Vec2, n)
		sing := make([]bool, n)
		for i, p := range pts {
			u, v := s.Project(p)
			uv[i] = geom.Vec2{X: u, Y: v}
			sing[i] = s.Singular(u, v)
		}
		// unwrap along the half-edge
		for i := 1; i < n; i++ {
			uv[i] = nearestWrap(uv[i], uv[i-1], pu, pv)
		}
		// poles take the u of their neighbour on this half-edge
		for i := range uv {
			if !sing[i] {
				continue
			}
			for d := 1; d < n; d++ {
				if i-d >= 0 && !sing[i-d] {
					uv[i].X = uv[i-d].X
					break
				}
				if i+d < n && !sing[i+d] {
					uv[i].X = uv[i+d].X
					break
				}
			}
		}
		// shift the whole half-edge by periods
		du, dv := 0.0, 0.0
		if uses[h.Edge] == 2 && n >= 2 {
			a, b := uv[0], uv[n-1]
			if pu > 0 && math.Abs(b.X-a.X) < 1e-6 && math.Abs(b.Y-a.Y) > 1e-9 {
				// constant-u seam: going up it lies on the high side (the low
				// side on a reversed face, whose loops run clockwise in (u, v))
				base := snapPeriod(a.X, pu)
				want := base
				if (b.Y > a.Y) != reversed {
					want = base + pu
				}
				du = want - a.X
				if prevEnd != nil && pv > 0 {
					dv = shiftTo(a.Y, prevEnd.Y, pv)
				} else if prevEnd == nil && pv > 0 {
					dv = snapPeriod(a.Y, pv) - a.Y
				}
			} else if pv > 0 && math.Abs(b.Y-a.Y) < 1e-6 && math.Abs(b.X-a.X) > 1e-9 {
				// constant-v seam: going +u it is the low side (reversed: high)
				base := snapPeriod(a.Y, pv)
				want := base
				if (b.X < a.X) != reversed {
					want = base + pv
				}
				dv = want - a.Y
				if prevEnd != nil && pu > 0 {
					du = shiftTo(a.X, prevEnd.X, pu)
				} else if prevEnd == nil && pu > 0 {
					du = snapPeriod(a.X, pu) - a.X
				}
			} else if prevEnd != nil {
				du, dv = shiftTo(a.X, prevEnd.X, pu), shiftTo(a.Y, prevEnd.Y, pv)
			}
		} else if prevEnd != nil {
			du, dv = shiftTo(uv[0].X, prevEnd.X, pu), shiftTo(uv[0].Y, prevEnd.Y, pv)
			if prevPole && pu > 0 {
				// across a pole nearness says nothing (a half turn is ±π away):
				// a counter-clockwise loop runs left along a pole it came up to
				// and right along one it came down to (the other way round on
				// a reversed face)
				want := uv[0].X + du
				left := (prevDV > 0) != reversed
				if left {
					for want > prevEnd.X+1e-9 {
						want -= pu
					}
					for want <= prevEnd.X-pu+1e-9 {
						want += pu
					}
				} else {
					for want < prevEnd.X-1e-9 {
						want += pu
					}
					for want >= prevEnd.X+pu-1e-9 {
						want -= pu
					}
				}
				du = want - uv[0].X
			}
		} else if n >= 1 {
			// first half-edge with no seam: start in [0, period)
			du = snapPeriod(uv[0].X, pu) - uv[0].X
			dv = snapPeriod(uv[0].Y, pv) - uv[0].Y
			if pu == 0 {
				du = 0
			}
			if pv == 0 {
				dv = 0
			}
		}
		for i := range uv {
			uv[i].X += du
			uv[i].Y += dv
		}
		res = append(res, heUV{h: h, ts: ts, pts: pts, uv: uv, sing: sing})
		e := uv[n-1]
		prevEnd = &e
		prevPole = sing[n-1]
		if n >= 2 {
			prevDV = uv[n-1].Y - uv[n-2].Y
		}
	}
	return res
}

// snapPeriod puts x in [0, p), treating values within 1e-7 of p as 0.
func snapPeriod(x, p float64) float64 {
	if p == 0 {
		return x
	}
	x = math.Mod(x, p)
	if x < 0 {
		x += p
	}
	if p-x < 1e-7 {
		x = 0
	}
	return x
}

// shiftTo is the multiple of the period moving x nearest to target.
func shiftTo(x, target, p float64) float64 {
	if p == 0 {
		return 0
	}
	return math.Round((target-x)/p) * p
}

func nearestWrap(q, prev geom.Vec2, pu, pv float64) geom.Vec2 {
	q.X += shiftTo(q.X, prev.X, pu)
	q.Y += shiftTo(q.Y, prev.Y, pv)
	return q
}

// Volume of a closed mesh (divergence theorem); positive when outward.
func (m *Mesh) Volume() float64 {
	v := 0.0
	for _, t := range m.Tris {
		a, b, c := m.Pos[t[0]], m.Pos[t[1]], m.Pos[t[2]]
		v += a.Dot(b.Cross(c))
	}
	return v / 6
}

// Area of the mesh.
func (m *Mesh) Area() float64 {
	s := 0.0
	for _, t := range m.Tris {
		a, b, c := m.Pos[t[0]], m.Pos[t[1]], m.Pos[t[2]]
		s += b.Sub(a).Cross(c.Sub(a)).Len()
	}
	return s / 2
}

// loopRuns splits a loop for meshing. On a surface with no period an edge
// the loop runs along twice (a seam, as in an annulus made by a revolve) is
// no boundary at all: both uses are dropped and what is left falls apart into
// closed runs (the annulus's two circles).
func loopRuns(l *Loop, dropSeams bool) [][]*HalfEdge {
	hs := l.HalfEdges()
	if !dropSeams {
		return [][]*HalfEdge{hs}
	}
	uses := map[*Edge]int{}
	for _, h := range hs {
		uses[h.Edge]++
	}
	start := -1
	for i, h := range hs {
		if uses[h.Edge] == 2 {
			start = i
			break
		}
	}
	if start < 0 {
		return [][]*HalfEdge{hs}
	}
	var runs [][]*HalfEdge
	var cur []*HalfEdge
	for k := 1; k <= len(hs); k++ {
		h := hs[(start+k)%len(hs)]
		if uses[h.Edge] == 2 {
			if len(cur) > 0 {
				runs = append(runs, cur)
			}
			cur = nil
			continue
		}
		cur = append(cur, h)
	}
	if len(cur) > 0 {
		runs = append(runs, cur)
	}
	return runs
}

// segIndex buckets segments on a grid for "is a point near any of them".
type segIndex struct {
	cell  float64
	segs  [][2]int
	cells map[[2]int64][]int
}

func newSegIndex(pts []geom.Vec2, segs [][2]int, d float64) *segIndex {
	s := &segIndex{cell: math.Max(d*2, 1e-12), segs: segs, cells: map[[2]int64][]int{}}
	for i, sg := range segs {
		a, b := pts[sg[0]], pts[sg[1]]
		x0, x1 := math.Min(a.X, b.X)-d, math.Max(a.X, b.X)+d
		y0, y1 := math.Min(a.Y, b.Y)-d, math.Max(a.Y, b.Y)+d
		for x := int64(math.Floor(x0 / s.cell)); x <= int64(math.Floor(x1/s.cell)); x++ {
			for y := int64(math.Floor(y0 / s.cell)); y <= int64(math.Floor(y1/s.cell)); y++ {
				k := [2]int64{x, y}
				s.cells[k] = append(s.cells[k], i)
			}
		}
	}
	return s
}

func (s *segIndex) near(p geom.Vec2, pts []geom.Vec2, d float64) bool {
	k := [2]int64{int64(math.Floor(p.X / s.cell)), int64(math.Floor(p.Y / s.cell))}
	for _, i := range s.cells[k] {
		a, b := pts[s.segs[i][0]], pts[s.segs[i][1]]
		ab := b.Sub(a)
		t := 0.0
		if l2 := ab.Dot(ab); l2 > 0 {
			t = math.Max(0, math.Min(1, p.Sub(a).Dot(ab)/l2))
		}
		if a.Add(ab.Scale(t)).Dist(p) < d {
			return true
		}
	}
	return false
}
