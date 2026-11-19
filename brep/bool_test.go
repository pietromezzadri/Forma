package brep_test

import (
	"math"
	"testing"

	"forma/brep"
	"forma/geom"
	"forma/ops"
)

func vol(t *testing.T, b *brep.Body) float64 {
	t.Helper()
	if err := brep.Check(b); err != nil {
		t.Fatalf("invalid: %v", err)
	}
	opt := brep.DefaultMeshOptions(b)
	opt.ChordTol = b.Bounds().Diagonal() * 5e-5
	m := brep.Tessellate(b, opt)
	if err := m.Watertight(1e-5); err != nil {
		t.Fatalf("mesh: %v", err)
	}
	return m.Volume()
}

func box(t *testing.T, x0, y0, z0, x1, y1, z1 float64) *brep.Body {
	b, err := ops.Box(geom.V3(x0, y0, z0), geom.V3(x1, y1, z1))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func cyl(t *testing.T, x, y, z, r, h float64) *brep.Body {
	f := geom.World
	f.O = geom.V3(x, y, z)
	b, err := ops.Cylinder(f, r, h)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func boolCase(t *testing.T, name string, a, b *brep.Body, op brep.BoolOp, want float64) *brep.Body {
	t.Helper()
	r, err := brep.Boolean(a, b, op)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	v := vol(t, r)
	if math.Abs(v-want) > math.Max(1e-3*want, 1e-6) {
		t.Fatalf("%s: volume %.4f, want %.4f (%.3f%%)", name, v, want, 100*(v-want)/want)
	}
	return r
}

func TestBooleanBoxes(t *testing.T) {
	a := box(t, 0, 0, 0, 20, 20, 20)
	b := box(t, 10, 5, 5, 30, 15, 15)
	boolCase(t, "union", a, b, brep.Union, 9000)
	boolCase(t, "subtract", a, b, brep.Subtract, 7000)
	boolCase(t, "intersect", a, b, brep.Intersect, 1000)
	// a box through the middle: a tunnel
	c := box(t, -5, 5, 5, 25, 15, 15)
	r := boolCase(t, "tunnel", a, c, brep.Subtract, 8000-2000)
	if g := brep.Genus(r); g != 1 {
		t.Fatalf("tunnel genus %d", g)
	}
}

func TestBooleanHole(t *testing.T) {
	plate := box(t, 0, 0, 0, 40, 30, 10)
	pin := cyl(t, 20, 15, -5, 5, 20)
	r := boolCase(t, "hole", plate, pin, brep.Subtract, 12000-math.Pi*25*10)
	if g := brep.Genus(r); g != 1 {
		t.Fatalf("genus %d", g)
	}
	boolCase(t, "pin through", plate, pin, brep.Union, 12000+math.Pi*25*10)
}

// Faces lying on each other: a boss standing on the plate, a pocket from its top.
func TestBooleanCoplanar(t *testing.T) {
	plate := box(t, 0, 0, 0, 40, 30, 10)
	boss := cyl(t, 20, 15, 10, 5, 8)
	r := boolCase(t, "boss", plate, boss, brep.Union, 12000+math.Pi*25*8)
	if s := len(r.Shells); s != 1 {
		t.Fatalf("boss: %d shells", s)
	}
	pocket := box(t, 10, 10, 5, 30, 20, 10)
	boolCase(t, "pocket", plate, pocket, brep.Subtract, 12000-1000)
	block := box(t, 10, 10, 10, 30, 20, 25)
	boolCase(t, "block on top", plate, block, brep.Union, 12000+3000)
	hole := cyl(t, 20, 15, 3, 4, 7)
	boolCase(t, "blind hole", plate, hole, brep.Subtract, 12000-math.Pi*16*7)
}

func TestBooleanCurved(t *testing.T) {
	s, err := ops.Sphere(geom.V3(10, 10, 10), 8)
	if err != nil {
		t.Fatal(err)
	}
	b := box(t, 10, -5, -5, 30, 25, 25) // the half x > 10
	half := 4.0 / 3 * math.Pi * 512 / 2
	boolCase(t, "half sphere", s, b, brep.Intersect, half)
	boolCase(t, "other half", s, b, brep.Subtract, half)

	a := cyl(t, 0, 0, -20, 5, 40)
	f := geom.FrameZ(geom.V3(-20, 0, 0), geom.XAxis, geom.YAxis)
	c, err := ops.Cylinder(f, 5, 40)
	if err != nil {
		t.Fatal(err)
	}
	// two equal cylinders crossing at right angles meet in a Steinmetz solid: 16 r³ / 3
	boolCase(t, "steinmetz", a, c, brep.Intersect, 16*125.0/3)
	boolCase(t, "cross", a, c, brep.Union, 2*math.Pi*25*40-16*125.0/3)
}

// Volume identities over boxes and cylinders in assorted places.
func TestBooleanFuzz(t *testing.T) {
	rnd := uint64(12345)
	next := func() float64 {
		rnd ^= rnd << 13
		rnd ^= rnd >> 7
		rnd ^= rnd << 17
		return float64(rnd%10000) / 10000
	}
	shape := func() *brep.Body {
		if next() < 0.5 {
			x, y, z := next()*20, next()*20, next()*20
			return box(t, x, y, z, x+5+next()*15, y+5+next()*15, z+5+next()*15)
		}
		return cyl(t, 5+next()*15, 5+next()*15, next()*10, 3+next()*6, 5+next()*15)
	}
	fails := 0
	for i := 0; i < 24; i++ {
		a, b := shape(), shape()
		va, vb := vol(t, a), vol(t, b)
		u, e1 := brep.Boolean(a, b, brep.Union)
		x, e2 := brep.Boolean(a, b, brep.Intersect)
		d, e3 := brep.Boolean(a, b, brep.Subtract)
		vx, vd := 0.0, 0.0
		if e2 == brep.ErrEmpty {
			e2, x = nil, nil
		}
		if e3 == brep.ErrEmpty {
			e3, d = nil, nil
		}
		if e1 != nil || e2 != nil || e3 != nil {
			t.Logf("case %d: %v / %v / %v (A %v, B %v)", i, e1, e2, e3, a.Bounds(), b.Bounds())
			fails++
			continue
		}
		vu, err := volE(u)
		if err == nil && x != nil {
			vx, err = volE(x)
		}
		if err == nil && d != nil {
			vd, err = volE(d)
		}
		if err != nil {
			t.Logf("case %d: result: %v", i, err)
			fails++
			continue
		}
		tol := 2e-3 * (va + vb)
		if math.Abs(vu+vx-va-vb) > tol || math.Abs(vd-(va-vx)) > tol {
			t.Errorf("case %d: A %.2f B %.2f ∪ %.2f ∩ %.2f − %.2f", i, va, vb, vu, vx, vd)
		}
	}
	if fails > 0 {
		t.Errorf("%d of 24 cases failed", fails)
	}
}

func volE(b *brep.Body) (float64, error) {
	if err := brep.Check(b); err != nil {
		return 0, err
	}
	opt := brep.DefaultMeshOptions(b)
	opt.ChordTol = b.Bounds().Diagonal() * 5e-5
	m := brep.Tessellate(b, opt)
	if err := m.Watertight(1e-5); err != nil {
		return 0, err
	}
	return m.Volume(), nil
}

// Boxes on a coarse grid: faces flush, edges running along each other,
// boxes standing side by side, on top of each other or inside each other.
func TestBooleanFlushFuzz(t *testing.T) {
	rnd := uint64(777)
	next := func(n int) float64 {
		rnd ^= rnd << 13
		rnd ^= rnd >> 7
		rnd ^= rnd << 17
		return float64(rnd % uint64(n))
	}
	type bx struct{ lo, hi [3]float64 }
	shape := func() bx {
		var b bx
		for k := 0; k < 3; k++ {
			b.lo[k] = next(4) * 5
			b.hi[k] = b.lo[k] + 5 + next(3)*5
		}
		return b
	}
	fails, ran := 0, 0
	for i := 0; ran < 120 && i < 2000; i++ {
		p, q := shape(), shape()
		// the overlap's extent per axis; contact only along an edge or at
		// a point would make a non-manifold union, which isn't a solid
		vx, thin := 1.0, 0
		for k := 0; k < 3; k++ {
			d := math.Min(p.hi[k], q.hi[k]) - math.Max(p.lo[k], q.lo[k])
			if d < 0 {
				vx = -1
				break
			}
			if d == 0 {
				thin++
			}
			vx *= d
		}
		if vx < 0 || thin >= 2 {
			continue
		}
		ran++
		a := box(t, p.lo[0], p.lo[1], p.lo[2], p.hi[0], p.hi[1], p.hi[2])
		b := box(t, q.lo[0], q.lo[1], q.lo[2], q.hi[0], q.hi[1], q.hi[2])
		va, vb := vol(t, a), vol(t, b)
		check := func(op brep.BoolOp, want float64) {
			r, err := brep.Boolean(a, b, op)
			if err == brep.ErrEmpty && want < 1e-9 {
				return
			}
			if err != nil {
				t.Logf("case %d %v %v: %v", i, p, q, op)
				t.Logf("   %v", err)
				fails++
				return
			}
			v, err := volE(r)
			if err != nil {
				t.Logf("case %d %v %v: %v result: %v", i, p, q, op, err)
				fails++
				return
			}
			if math.Abs(v-want) > 1e-6*(va+vb) {
				t.Errorf("case %d %v %v: %v volume %g want %g", i, p, q, op, v, want)
			}
		}
		check(brep.Union, va+vb-vx)
		check(brep.Intersect, vx)
		check(brep.Subtract, va-vx)
	}
	if fails > 0 {
		t.Errorf("%d of %d operations failed", fails, ran*3)
	}
}

// Cylinders flush with boxes and with each other: caps on faces, coaxial
// cylinders sharing their caps' planes, a cylinder through a box's face.
func TestBooleanFlushCylinders(t *testing.T) {
	cases := []struct {
		name string
		a, b *brep.Body
		op   brep.BoolOp
		want float64
	}{
		{"boss on plate", box(t, 0, 0, 0, 40, 40, 10), cyl(t, 20, 20, 10, 8, 10), brep.Union, 16000 + math.Pi*64*10},
		{"through hole flush", box(t, 0, 0, 0, 40, 40, 10), cyl(t, 20, 20, 0, 8, 10), brep.Subtract, 16000 - math.Pi*64*10},
		{"half in, flush", box(t, 0, 0, 0, 40, 40, 10), cyl(t, 40, 20, 0, 8, 10), brep.Union, 16000 + math.Pi*64*5},
		{"coaxial stack", cyl(t, 0, 0, 0, 10, 10), cyl(t, 0, 0, 10, 6, 10), brep.Union, math.Pi * (100*10 + 36*10)},
		{"coaxial same radius", cyl(t, 0, 0, 0, 10, 10), cyl(t, 0, 0, 10, 10, 10), brep.Union, math.Pi * 100 * 20},
		{"coaxial bore", cyl(t, 0, 0, 0, 10, 10), cyl(t, 0, 0, 0, 6, 10), brep.Subtract, math.Pi * (100 - 36) * 10},
	}
	for _, k := range cases {
		r, err := brep.Boolean(k.a, k.b, k.op)
		if err != nil {
			t.Errorf("%s: %v", k.name, err)
			continue
		}
		v, err := volE(r)
		if err != nil {
			t.Errorf("%s: result %v", k.name, err)
			continue
		}
		if math.Abs(v-k.want) > 1e-3*k.want {
			t.Errorf("%s: volume %g want %g", k.name, v, k.want)
		}
	}
}
