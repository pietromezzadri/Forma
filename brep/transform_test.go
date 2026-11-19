package brep_test

import (
	"math"
	"testing"

	"forma/brep"
	"forma/geom"
	"forma/ops"
)

func TestMassOfBox(t *testing.T) {
	b := box(t, 0, 0, 0, 40, 30, 20)
	mp := brep.Mass(b, 2e-5)
	if math.Abs(mp.Volume-24000) > 1e-6 || math.Abs(mp.Area-2*(1200+800+600)) > 1e-6 {
		t.Fatalf("volume %g area %g", mp.Volume, mp.Area)
	}
	if mp.Centroid.Dist(geom.V3(20, 15, 10)) > 1e-9 {
		t.Fatalf("centroid %v", mp.Centroid)
	}
	// Ixx = m (b² + c²) / 12 for a box of sides a, b, c
	m := mp.Volume
	want := [3]float64{m * (30*30 + 20*20) / 12, m * (40*40 + 20*20) / 12, m * (40*40 + 30*30) / 12}
	for i := 0; i < 3; i++ {
		if math.Abs(mp.Inertia[i][i]-want[i]) > 1e-6*want[i] {
			t.Fatalf("I%d%d = %g, want %g", i, i, mp.Inertia[i][i], want[i])
		}
	}
	if math.Abs(mp.Inertia[0][1]) > 1e-3 {
		t.Fatalf("Ixy %g", mp.Inertia[0][1])
	}
	if mp.Principal[0] > mp.Principal[1] || mp.Principal[1] > mp.Principal[2] {
		t.Fatalf("principal %v", mp.Principal)
	}
}

func TestMassOfCylinder(t *testing.T) {
	b := cyl(t, 0, 0, 0, 10, 30)
	mp := brep.Mass(b, 2e-5)
	want := math.Pi * 100 * 30
	if math.Abs(mp.Volume-want) > 2e-4*want {
		t.Fatalf("volume %g want %g", mp.Volume, want)
	}
	// Izz = m r² / 2
	iz := mp.Volume * 100 / 2
	if math.Abs(mp.Inertia[2][2]-iz) > 5e-4*iz {
		t.Fatalf("Izz %g want %g", mp.Inertia[2][2], iz)
	}
}

// Moving, turning and mirroring keeps a body valid and its volume, and
// moves its centroid with it.
func TestTransform(t *testing.T) {
	plate := box(t, 0, 0, 0, 40, 30, 10)
	holed, err := brep.Boolean(plate, cyl(t, 12, 15, -5, 6, 20), brep.Subtract)
	if err != nil {
		t.Fatal(err)
	}
	tor, _ := ops.Torus(geom.Frame{O: geom.V3(5, 0, 3), X: geom.XAxis, Y: geom.YAxis, Z: geom.ZAxis}, 20, 4)
	sph, _ := ops.Sphere(geom.V3(1, 2, 3), 7)
	cone, _ := ops.Cone(geom.World, 10, 4, 12)
	xs := map[string]geom.Xform{
		"move":   geom.Translate(geom.V3(10, -4, 7)),
		"rotate": geom.Rotate(geom.V3(3, 1, 0), geom.V3(1, 2, 3), 0.7),
		"mirror": geom.Mirror(geom.V3(50, 0, 0), geom.V3(1, 0.3, 0)),
		"both":   geom.Rotate(geom.V3(0, 0, 0), geom.ZAxis, 1.1).Then(geom.Mirror(geom.V3(0, 0, 5), geom.ZAxis)),
	}
	for bn, b := range map[string]*brep.Body{"holed plate": holed, "torus": tor, "sphere": sph, "cone": cone} {
		m0 := brep.Mass(b, 1e-4)
		v0 := vol(t, b)
		for xn, x := range xs {
			c, err := brep.Transform(b, x, nil)
			if err != nil {
				t.Fatalf("%s %s: %v", bn, xn, err)
			}
			v := vol(t, c)
			if math.Abs(v-v0) > 1e-4*v0 {
				t.Errorf("%s %s: volume %g, was %g", bn, xn, v, v0)
			}
			m1 := brep.Mass(c, 1e-4)
			if d := m1.Centroid.Dist(x.Pt(m0.Centroid)); d > 1e-3 {
				t.Errorf("%s %s: centroid off by %g", bn, xn, d)
			}
			// mirrored back it unites with the original exactly where they overlap
			back, err := brep.Transform(c, x.Inverse(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if d := brep.Mass(back, 1e-4).Centroid.Dist(m0.Centroid); d > 1e-6 {
				t.Errorf("%s %s: round trip off by %g", bn, xn, d)
			}
		}
	}
}

// A mirrored copy joins its original.
func TestMirrorJoin(t *testing.T) {
	b := box(t, 0, 0, 0, 20, 10, 10)
	b2, err := brep.Boolean(b, cyl(t, 20, 5, 0, 5, 10), brep.Union)
	if err != nil {
		t.Fatal(err)
	}
	m, err := brep.Transform(b2, geom.Mirror(geom.V3(-5, 0, 0), geom.XAxis), nil)
	if err != nil {
		t.Fatal(err)
	}
	// overlapping by nothing, touching at nothing: two shells
	u, err := brep.Boolean(b2, m, brep.Union)
	if err != nil {
		t.Fatal(err)
	}
	v1 := vol(t, b2)
	if v := vol(t, u); math.Abs(v-2*v1) > 1e-3*v1 {
		t.Fatalf("union %g want %g", v, 2*v1)
	}
	// mirrored about its own face: one solid of twice the volume
	m2, _ := brep.Transform(b2, geom.Mirror(geom.V3(0, 0, 0), geom.XAxis), nil)
	u2, err := brep.Boolean(b2, m2, brep.Union)
	if err != nil {
		t.Fatal(err)
	}
	if v := vol(t, u2); math.Abs(v-2*v1) > 1e-3*v1 {
		t.Fatalf("union about the face %g want %g", v, 2*v1)
	}
	if len(u2.Shells) != 1 {
		t.Fatalf("%d shells", len(u2.Shells))
	}
}
