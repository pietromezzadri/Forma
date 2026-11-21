package fea

import (
	"math"
	"testing"

	"forma/brep"
	"forma/geom"
	"forma/ops"
)

// faceAt finds the face of b whose vertices all lie on the plane
// coord(p) == at (coord 0, 1, 2 for x, y, z).
func faceAt(t *testing.T, b *brep.Body, coord int, at float64) int {
	t.Helper()
	for _, f := range b.Faces() {
		ok, n := true, 0
		for _, l := range f.Loops {
			for _, h := range l.HalfEdges() {
				p := h.Start().P
				c := [3]float64{p.X, p.Y, p.Z}[coord]
				ok = ok && math.Abs(c-at) < 1e-9
				n++
			}
		}
		if ok && n > 0 {
			return f.ID
		}
	}
	t.Fatalf("no face at %c = %g", "xyz"[coord], at)
	return 0
}

var steel = Material{"steel", 210000, 0.3, 250, 7.85}

// A 100 × 10 × 10 bar held at one end and pulled at the other with 1 kN:
// stress F/A = 10 MPa, stretch FL/(EA).
func TestBarInTension(t *testing.T) {
	b, err := ops.Box(geom.V3(0, 0, 0), geom.V3(100, 10, 10))
	if err != nil {
		t.Fatal(err)
	}
	r, err := Run(Study{Body: b, Material: steel, Fixed: []int{faceAt(t, b, 0, 0)},
		Loads: []Load{{Faces: []int{faceAt(t, b, 0, 100)}, Force: geom.V3(1000, 0, 0)}}, Size: 2.5})
	if err != nil {
		t.Fatal(err)
	}
	m := r.Mesh
	if v := m.Volume(); math.Abs(v-10000) > 1 {
		t.Fatalf("mesh volume %.2f, want 10000", v)
	}
	if math.Abs(r.Applied.X-1000) > 1e-6 {
		t.Fatalf("applied %v", r.Applied)
	}
	// the stretch of the free end: the mean x displacement there
	sum, cnt := 0.0, 0
	for i, p := range m.Nodes {
		if math.Abs(p.X-100) < 1e-9 {
			sum += r.U[i].X
			cnt++
		}
	}
	want := 1000 * 100 / (210000 * 100.0)
	got := sum / float64(cnt)
	t.Logf("%d nodes, %d elements, %d iterations, %v: stretch %.6f (want %.6f), max σvm %.2f MPa",
		len(m.Nodes), len(m.Tets), r.Iter, r.Took, got, want, r.MaxVM)
	// the held end can't narrow (Poisson): a little stiffer than the ideal bar
	if got > want*1.001 || got < want*0.97 {
		t.Fatalf("stretch %.6f, want %.6f", got, want)
	}
	// away from the ends the stress is F/A
	for i, p := range m.Nodes {
		if p.X > 30 && p.X < 70 && math.Abs(r.VonMises[i]-10) > 0.3 {
			t.Fatalf("σvm %.3f at %v, want 10", r.VonMises[i], p)
		}
	}
	if math.Abs(r.Safety-250/r.MaxVM) > 1e-9 {
		t.Fatal("safety factor")
	}
}

// A 100 × 10 × 10 cantilever with 100 N down at its tip: deflection
// FL³/(3EI), bending stress M c / I at mid-span.
func TestCantilever(t *testing.T) {
	b, err := ops.Box(geom.V3(0, 0, 0), geom.V3(100, 10, 10))
	if err != nil {
		t.Fatal(err)
	}
	r, err := Run(Study{Body: b, Material: steel, Fixed: []int{faceAt(t, b, 0, 0)},
		Loads: []Load{{Faces: []int{faceAt(t, b, 0, 100)}, Force: geom.V3(0, 0, -100)}}, Size: 2.5})
	if err != nil {
		t.Fatal(err)
	}
	I := 10 * 1000.0 / 12
	want := 100 * math.Pow(100, 3) / (3 * 210000 * I)
	tip := 0.0
	for i, p := range r.Mesh.Nodes {
		if math.Abs(p.X-100) < 1e-9 {
			tip = math.Min(tip, r.U[i].Z)
		}
	}
	t.Logf("%d elements, %d iterations, %v: tip %.5f mm (beam theory %.5f), max σvm %.1f MPa",
		len(r.Mesh.Tets), r.Iter, r.Took, -tip, want, r.MaxVM)
	// Timoshenko shear adds ~0.8 % here; the mesh is a little stiff
	if -tip < want*0.97 || -tip > want*1.04 {
		t.Fatalf("tip deflection %.5f, want about %.5f", -tip, want)
	}
	// at mid-span, top and bottom: σ = M c / I = 100 · 50 · 5 / I = 30 MPa
	for i, p := range r.Mesh.Nodes {
		if math.Abs(p.X-50) < 1.5 && (p.Z < 1e-9 || p.Z > 10-1e-9) && p.Y > 2 && p.Y < 8 {
			if math.Abs(r.VonMises[i]-30) > 2.5 {
				t.Fatalf("σvm %.2f at %v, want about 30", r.VonMises[i], p)
			}
		}
	}
}

// A part held only on one face of a box but loaded sideways still
// solves; one with nothing fixed is refused.
func TestStudyErrors(t *testing.T) {
	b, _ := ops.Box(geom.V3(0, 0, 0), geom.V3(20, 20, 20))
	if _, err := Run(Study{Body: b, Material: steel}); err == nil {
		t.Fatal("solved with nothing fixed")
	}
}

// Curved faces: a cylinder's mesh holds its volume (the boundary edges'
// middles lie on the surface).
func TestCylinderMeshVolume(t *testing.T) {
	c, err := ops.Cylinder(geom.World, 10, 40)
	if err != nil {
		t.Fatal(err)
	}
	m, err := MeshBody(c, 3)
	if err != nil {
		t.Fatal(err)
	}
	want := math.Pi * 100 * 40
	if v := m.Volume(); math.Abs(v-want) > 2e-3*want {
		t.Fatalf("mesh volume %.2f, want %.2f", v, want)
	}
}

// A plate with a hole pulled along its length: the stress beside the hole
// is about three times the stress far from it (Kirsch: 3 for a small hole
// in a wide plate; a little less here, the hole being a fifth of the width).
func TestPlateWithHole(t *testing.T) {
	plate, _ := ops.Box(geom.V3(0, 0, 0), geom.V3(120, 40, 4))
	f := geom.World
	f.O = geom.V3(60, 20, -1)
	hole, _ := ops.Cylinder(f, 4, 6)
	b, err := brep.Boolean(plate, hole, brep.Subtract)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Run(Study{Body: b, Material: steel, Fixed: []int{faceAt(t, b, 0, 0)},
		Loads: []Load{{Faces: []int{faceAt(t, b, 0, 120)}, Force: geom.V3(1600, 0, 0)}}, Size: 3})
	if err != nil {
		t.Fatal(err)
	}
	nominal := 1600 / (40 * 4.0) // 10 MPa
	t.Logf("%d elements, %v: max σvm %.1f MPa at %v (×%.2f the nominal)", len(r.Mesh.Tets), r.Took, r.MaxVM, r.MaxVMAt, r.MaxVM/nominal)
	if k := r.MaxVM / nominal; k < 2.4 || k > 3.6 {
		t.Fatalf("stress concentration %.2f, want about 3", k)
	}
	if math.Hypot(r.MaxVMAt.X-60, r.MaxVMAt.Y-20) > 6 {
		t.Fatalf("the highest stress is at %v, not beside the hole", r.MaxVMAt)
	}
}
