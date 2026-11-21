package fea

import (
	"fmt"
	"math"
	"runtime"
	"sort"
	"sync"
	"time"

	"forma/brep"
	"forma/geom"
)

// Load is a force on faces of the body: a total force spread over their
// area, or a pressure (MPa) pushing on them (into the body for p > 0).
type Load struct {
	Faces    []int // B-rep face IDs
	Force    geom.Vec3
	Pressure float64
}

// Study is a static stress analysis: a body, its material, the faces held
// fixed and the loads.
type Study struct {
	Body     *brep.Body
	Material Material
	Fixed    []int // B-rep face IDs held fixed
	Loads    []Load
	Size     float64 // element size (mm); 0 picks one
	// Progress, if set, hears what is being done and how far along (0..1)
	Progress func(stage string, frac float64)
}

// Result is what a study found.
type Result struct {
	Mesh     *Mesh
	U        []geom.Vec3 // displacement of each node (mm)
	VonMises []float64   // von Mises stress at each node (MPa), averaged
	MaxVM    float64
	MaxVMAt  geom.Vec3
	MaxU     float64
	MaxUAt   geom.Vec3
	Safety   float64 // yield / max von Mises
	Iter     int     // solver iterations
	Applied  geom.Vec3
	Took     time.Duration
}

// Run meshes the body and solves the study.
func Run(s Study) (*Result, error) {
	t0 := time.Now()
	if s.Body == nil {
		return nil, fmt.Errorf("no body")
	}
	if len(s.Fixed) == 0 {
		return nil, fmt.Errorf("hold at least one face fixed")
	}
	if s.Material.E <= 0 || s.Material.Nu <= 0 || s.Material.Nu >= 0.5 {
		return nil, fmt.Errorf("the material needs a Young's modulus and a Poisson's ratio below 0.5")
	}
	s.report("Meshing", 0)
	m, err := MeshBody(s.Body, s.Size)
	if err != nil {
		return nil, err
	}
	r, err := Solve(m, s)
	if err != nil {
		return nil, err
	}
	r.Took = time.Since(t0)
	return r, nil
}

// Solve solves a study on a mesh made already.
func Solve(m *Mesh, s Study) (*Result, error) {
	s.report("Assembling", 0)
	n := len(m.Nodes)
	D := elasticity(s.Material.E, s.Material.Nu)
	K := newBlockMatrix(m)
	// element matrices in parallel, added one after another
	ke := make([][30][30]float64, len(m.Tets))
	bad := make([]bool, len(m.Tets))
	parallel(len(m.Tets), func(lo, hi int) {
		for ti := lo; ti < hi; ti++ {
			x := m.elemNodes(ti)
			bad[ti] = !stiffness(&x, &D, &ke[ti])
		}
	})
	for ti, e := range m.Tets {
		if bad[ti] {
			return nil, fmt.Errorf("element %d is inside out", ti)
		}
		K.addElement(e, &ke[ti])
	}
	ke = nil

	// held nodes: every node of a boundary triangle on a fixed face
	fixed := make([]bool, n)
	isFixed := map[int]bool{}
	for _, f := range s.Fixed {
		isFixed[f] = true
	}
	nFixed := 0
	for _, bt := range m.Bound {
		if bt.Face != 0 && isFixed[bt.Face] {
			for _, k := range bt.N {
				if !fixed[k] {
					fixed[k] = true
					nFixed++
				}
			}
		}
	}
	if nFixed == 0 {
		return nil, fmt.Errorf("no part of the mesh lies on the fixed faces")
	}

	// loads: a 6-node triangle's share of a uniform traction goes to its
	// edge mids (a third each), none to its corners
	f := make([]geom.Vec3, n)
	var applied geom.Vec3
	for li, ld := range s.Loads {
		on := map[int]bool{}
		for _, fc := range ld.Faces {
			on[fc] = true
		}
		area := 0.0
		var tris []BoundTri
		for _, bt := range m.Bound {
			if bt.Face != 0 && on[bt.Face] {
				tris = append(tris, bt)
				area += triArea(m, bt)
			}
		}
		if area == 0 {
			return nil, fmt.Errorf("load %d: no part of the mesh lies on its faces", li+1)
		}
		for _, bt := range tris {
			a := triArea(m, bt)
			force := ld.Force.Scale(a / area)
			if ld.Pressure != 0 {
				nrm := triNormal(m, bt) // out of the body
				force = force.Add(nrm.Scale(-ld.Pressure * a))
			}
			for s := 3; s < 6; s++ {
				f[bt.N[s]] = f[bt.N[s]].Add(force.Scale(1.0 / 3))
			}
			applied = applied.Add(force)
		}
	}
	u, it, err := K.solve(f, fixed, func(x float64) { s.report("Solving", x) })
	if err != nil {
		return nil, err
	}
	res := &Result{Mesh: m, U: u, Iter: it, Applied: applied}
	res.stresses(&D)
	for i, d := range u {
		if l := d.Len(); l > res.MaxU {
			res.MaxU, res.MaxUAt = l, m.Nodes[i]
		}
	}
	if res.MaxVM > 0 {
		res.Safety = s.Material.Yield / res.MaxVM
	}
	return res, nil
}

func triArea(m *Mesh, bt BoundTri) float64 {
	a, b, c := m.Nodes[bt.N[0]], m.Nodes[bt.N[1]], m.Nodes[bt.N[2]]
	return b.Sub(a).Cross(c.Sub(a)).Len() / 2
}

func triNormal(m *Mesh, bt BoundTri) geom.Vec3 {
	a, b, c := m.Nodes[bt.N[0]], m.Nodes[bt.N[1]], m.Nodes[bt.N[2]]
	return b.Sub(a).Cross(c.Sub(a)).Norm()
}

// stresses works the von Mises stress out at every node: each element's
// stress at its own nodes, averaged where elements meet.
func (r *Result) stresses(D *[6][6]float64) {
	m := r.Mesh
	sum := make([]float64, len(m.Nodes))
	cnt := make([]float64, len(m.Nodes))
	var mu sync.Mutex
	parallel(len(m.Tets), func(lo, hi int) {
		ls := map[int]float64{}
		lc := map[int]float64{}
		for ti := lo; ti < hi; ti++ {
			x := m.elemNodes(ti)
			var u [10]geom.Vec3
			for k, nd := range m.Tets[ti] {
				u[k] = r.U[nd]
			}
			for k := 0; k < 10; k++ {
				s := stressAt(&x, &u, D, nodeXi[k][0], nodeXi[k][1], nodeXi[k][2])
				ls[m.Tets[ti][k]] += vonMises(s)
				lc[m.Tets[ti][k]]++
			}
		}
		mu.Lock()
		for k, v := range ls {
			sum[k] += v
			cnt[k] += lc[k]
		}
		mu.Unlock()
	})
	r.VonMises = make([]float64, len(m.Nodes))
	for i := range sum {
		if cnt[i] > 0 {
			r.VonMises[i] = sum[i] / cnt[i]
		}
		if r.VonMises[i] > r.MaxVM {
			r.MaxVM, r.MaxVMAt = r.VonMises[i], m.Nodes[i]
		}
	}
}

// parallel runs fn over [0, n) in chunks on all the CPUs.
func parallel(n int, fn func(lo, hi int)) {
	w := runtime.NumCPU()
	if n < 64 || w < 2 {
		fn(0, n)
		return
	}
	var wg sync.WaitGroup
	step := (n + w - 1) / w
	for lo := 0; lo < n; lo += step {
		hi := min(lo+step, n)
		wg.Add(1)
		go func(lo, hi int) {
			defer wg.Done()
			fn(lo, hi)
		}(lo, hi)
	}
	wg.Wait()
}

// blockMatrix is a symmetric stiffness matrix in compressed rows of 3 × 3
// blocks, one row per node.
type blockMatrix struct {
	rowStart []int
	cols     []int
	vals     [][9]float64
}

func newBlockMatrix(m *Mesh) *blockMatrix {
	n := len(m.Nodes)
	nb := make([][]int, n)
	for _, e := range m.Tets {
		for _, a := range e {
			nb[a] = append(nb[a], e[:]...)
		}
	}
	K := &blockMatrix{rowStart: make([]int, n+1)}
	for i := range nb {
		s := nb[i]
		sort.Ints(s)
		u := s[:0]
		for k, x := range s {
			if k == 0 || x != s[k-1] {
				u = append(u, x)
			}
		}
		K.cols = append(K.cols, u...)
		K.rowStart[i+1] = len(K.cols)
	}
	K.vals = make([][9]float64, len(K.cols))
	return K
}

func (K *blockMatrix) at(i, j int) *[9]float64 {
	row := K.cols[K.rowStart[i]:K.rowStart[i+1]]
	k := sort.SearchInts(row, j)
	return &K.vals[K.rowStart[i]+k]
}

func (K *blockMatrix) addElement(e [10]int, ke *[30][30]float64) {
	for a := 0; a < 10; a++ {
		for b := 0; b < 10; b++ {
			blk := K.at(e[a], e[b])
			for r := 0; r < 3; r++ {
				for c := 0; c < 3; c++ {
					blk[3*r+c] += ke[3*a+r][3*b+c]
				}
			}
		}
	}
}

// mul is y = K x.
func (K *blockMatrix) mul(x, y []geom.Vec3) {
	parallel(len(y), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			var s geom.Vec3
			for k := K.rowStart[i]; k < K.rowStart[i+1]; k++ {
				b := &K.vals[k]
				v := x[K.cols[k]]
				s.X += b[0]*v.X + b[1]*v.Y + b[2]*v.Z
				s.Y += b[3]*v.X + b[4]*v.Y + b[5]*v.Z
				s.Z += b[6]*v.X + b[7]*v.Y + b[8]*v.Z
			}
			y[i] = s
		}
	})
}

// solve finds K u = f with the fixed nodes held at 0: conjugate gradients
// preconditioned by the inverses of the diagonal blocks.
func (K *blockMatrix) solve(f []geom.Vec3, fixed []bool, progress func(float64)) ([]geom.Vec3, int, error) {
	n := len(f)
	inv := make([][9]float64, n)
	for i := 0; i < n; i++ {
		b := *K.at(i, i)
		var ok bool
		if inv[i], ok = inverse3(b); !ok {
			return nil, 0, fmt.Errorf("the stiffness is singular at node %d", i)
		}
	}
	mask := func(v []geom.Vec3) {
		for i := range v {
			if fixed[i] {
				v[i] = geom.Vec3{}
			}
		}
	}
	pre := func(r, z []geom.Vec3) {
		parallel(len(r), func(lo, hi int) {
			for i := lo; i < hi; i++ {
				b, v := &inv[i], r[i]
				z[i] = geom.Vec3{X: b[0]*v.X + b[1]*v.Y + b[2]*v.Z, Y: b[3]*v.X + b[4]*v.Y + b[5]*v.Z, Z: b[6]*v.X + b[7]*v.Y + b[8]*v.Z}
			}
		})
		mask(z)
	}
	dot := func(a, b []geom.Vec3) float64 {
		s := 0.0
		for i := range a {
			s += a[i].Dot(b[i])
		}
		return s
	}
	u := make([]geom.Vec3, n)
	r := append([]geom.Vec3(nil), f...)
	mask(r)
	norm0 := math.Sqrt(dot(r, r))
	if norm0 == 0 {
		return u, 0, nil
	}
	z := make([]geom.Vec3, n)
	pre(r, z)
	p := append([]geom.Vec3(nil), z...)
	q := make([]geom.Vec3, n)
	rz := dot(r, z)
	maxIt := 20*n + 1000
	for it := 1; it <= maxIt; it++ {
		K.mul(p, q)
		mask(q)
		pq := dot(p, q)
		if pq <= 0 {
			return nil, it, fmt.Errorf("the part can move freely: hold it so it can't slide or turn")
		}
		alpha := rz / pq
		for i := range u {
			u[i] = u[i].Add(p[i].Scale(alpha))
			r[i] = r[i].Sub(q[i].Scale(alpha))
		}
		res := math.Sqrt(dot(r, r))
		if res < 1e-9*norm0 {
			return u, it, nil
		}
		if it%20 == 0 && progress != nil {
			progress(math.Min(1, math.Log(norm0/res)/math.Log(1e9)))
		}
		pre(r, z)
		rz2 := dot(r, z)
		beta := rz2 / rz
		rz = rz2
		for i := range p {
			p[i] = z[i].Add(p[i].Scale(beta))
		}
	}
	return nil, maxIt, fmt.Errorf("the solver didn't settle (is the part held well enough?)")
}

func inverse3(m [9]float64) ([9]float64, bool) {
	det := m[0]*(m[4]*m[8]-m[5]*m[7]) - m[1]*(m[3]*m[8]-m[5]*m[6]) + m[2]*(m[3]*m[7]-m[4]*m[6])
	if math.Abs(det) < 1e-300 {
		return [9]float64{}, false
	}
	return [9]float64{
		(m[4]*m[8] - m[5]*m[7]) / det, (m[2]*m[7] - m[1]*m[8]) / det, (m[1]*m[5] - m[2]*m[4]) / det,
		(m[5]*m[6] - m[3]*m[8]) / det, (m[0]*m[8] - m[2]*m[6]) / det, (m[2]*m[3] - m[0]*m[5]) / det,
		(m[3]*m[7] - m[4]*m[6]) / det, (m[1]*m[6] - m[0]*m[7]) / det, (m[0]*m[4] - m[1]*m[3]) / det,
	}, true
}

func (s Study) report(stage string, frac float64) {
	if s.Progress != nil {
		s.Progress(stage, frac)
	}
}
