package brep_test

import (
	"math"
	"math/rand"
	"testing"

	"forma/brep"
	"forma/geom"
	"forma/ops"
)

// The tree's winding numbers agree with the exact sum: far off, near the
// surface either side, inside and out.
func TestWindingTree(t *testing.T) {
	a := box(t, 0, 0, 0, 40, 30, 10)
	b, err := brep.Boolean(a, cyl(t, 20, 15, -5, 8, 20), brep.Subtract)
	if err != nil {
		t.Fatal(err)
	}
	m := brep.Tessellate(b, brep.DefaultMeshOptions(b))
	fast := brep.WindingFast(m)
	rng := rand.New(rand.NewSource(1))
	worst := 0.0
	for i := 0; i < 3000; i++ {
		var q geom.Vec3
		if i%2 == 0 {
			q = geom.V3(rng.Float64()*60-10, rng.Float64()*50-10, rng.Float64()*30-10)
		} else {
			// just off a triangle, either side
			tr := m.Tris[rng.Intn(len(m.Tris))]
			p := m.Pos[tr[0]].Add(m.Pos[tr[1]]).Add(m.Pos[tr[2]]).Scale(1.0 / 3)
			n := m.Pos[tr[1]].Sub(m.Pos[tr[0]]).Cross(m.Pos[tr[2]].Sub(m.Pos[tr[0]])).Norm()
			q = p.Add(n.Scale((rng.Float64() - 0.5) * 0.02))
		}
		ex, fa := brep.WindingExact(m, q), fast(q)
		worst = math.Max(worst, math.Abs(ex-fa))
		if (ex > 0.5) != (fa > 0.5) {
			t.Fatalf("at %v: exact %.4f, tree %.4f", q, ex, fa)
		}
	}
	if worst > 0.02 {
		t.Fatalf("worst difference %.4f", worst)
	}
	t.Logf("worst difference %.2g", worst)
}

// A plate drilled through by a cylinder, the everyday boolean.
func BenchmarkBooleanPlateHole(b *testing.B) {
	t := &testing.T{}
	plate := box(t, 0, 0, 0, 80, 60, 10)
	hole := cyl(t, 40, 30, -5, 8, 20)
	for i := 0; i < b.N; i++ {
		if _, err := brep.Boolean(plate, hole, brep.Subtract); err != nil {
			b.Fatal(err)
		}
	}
}

// A cylinder joined to a sphere: curved against curved.
func BenchmarkBooleanCurved(b *testing.B) {
	t := &testing.T{}
	c := cyl(t, 0, 0, 0, 20, 40)
	s, err := ops.Sphere(geom.V3(10, 0, 40), 15)
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < b.N; i++ {
		if _, err := brep.Boolean(c, s, brep.Union); err != nil {
			b.Fatal(err)
		}
	}
}

// A sphere centred in the plane of the cylinder's seam: its own seam lies
// in that plane too, so the cut crosses both seams at the same points.
func TestBooleanSeamOnSeam(t *testing.T) {
	c := cyl(t, 0, 0, 0, 20, 40)
	vc := vol(t, c)
	for _, cs := range [][4]float64{{10, 0, 20, 15}, {10, 0, 40, 15}, {12, 0, 38, 15}, {8, 0, 41, 14}} {
		s, err := ops.Sphere(geom.V3(cs[0], cs[1], cs[2]), cs[3])
		if err != nil {
			t.Fatal(err)
		}
		vs := vol(t, s)
		var v [3]float64
		for i, op := range []brep.BoolOp{brep.Union, brep.Subtract, brep.Intersect} {
			r, err := brep.Boolean(c, s, op)
			if err != nil {
				t.Fatalf("%v %v: %v", cs, op, err)
			}
			v[i] = vol(t, r)
		}
		tol := 2e-3 * (vc + vs)
		if math.Abs(v[0]-(vc+vs-v[2])) > tol || math.Abs(v[1]-(vc-v[2])) > tol {
			t.Fatalf("%v: union %.1f, subtract %.1f, intersect %.1f of %.1f and %.1f", cs, v[0], v[1], v[2], vc, vs)
		}
	}
}
