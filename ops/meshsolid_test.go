package ops

import (
	"math"
	"math/rand"
	"strings"
	"testing"

	"forma/brep"
	"forma/geom"
)

func meshOf(b *brep.Body, chord float64) ([]geom.Vec3, [][3]int) {
	opt := brep.DefaultMeshOptions(b)
	if chord > 0 {
		opt.ChordTol = chord
	}
	m := brep.Tessellate(b, opt)
	var tris [][3]int
	for _, t := range m.Tris {
		tris = append(tris, [3]int{int(t[0]), int(t[1]), int(t[2])})
	}
	return m.Pos, tris
}

// A box's and a cylinder's tessellations made solids again: valid, as big
// as the mesh, flat regions merged (the box's six sides one face each); a
// solid cut by the kernel; an open mesh refused.
func TestMeshSolid(t *testing.T) {
	box, _ := Box(geom.V3(0, 0, 0), geom.V3(40, 30, 20))
	pos, tris := meshOf(box, 0)
	s, err := MeshSolid("box", pos, tris)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(s.Faces()); n != 6 {
		t.Fatalf("box: %d faces, want 6", n)
	}
	if v := checkSolid(t, s); math.Abs(v-24000) > 1e-6 {
		t.Fatalf("box volume %.4f", v)
	}
	cyl, _ := Cylinder(geom.World, 10, 25)
	pos, tris = meshOf(cyl, 0.05)
	c, err := MeshSolid("cyl", pos, tris)
	if err != nil {
		t.Fatal(err)
	}
	vc := checkSolid(t, c)
	t.Logf("cylinder: %d triangles → %d faces, volume %.1f (exact %.1f)", len(tris), len(c.Faces()), vc, math.Pi*100*25)
	if math.Abs(vc-math.Pi*100*25) > 0.01*math.Pi*100*25 || len(c.Faces()) >= len(tris) {
		t.Fatalf("cylinder: volume %.1f, %d faces of %d triangles", vc, len(c.Faces()), len(tris))
	}
	// the kernel works on it: a box cut out of the faceted cylinder
	cut, _ := Box(geom.V3(0, -20, 10), geom.V3(20, 20, 30))
	r, err := brep.Boolean(c, cut, brep.Subtract)
	if err != nil {
		t.Fatal(err)
	}
	if v := checkSolid(t, r); v <= 0.5*vc || v >= vc {
		t.Fatalf("cut: %.1f of %.1f", v, vc)
	}
	// open: a triangle taken out
	if _, err := MeshSolid("open", pos, tris[1:]); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("open mesh: %v", err)
	}
}

// The faceted cylinder cut whichever corner each face's loop starts at:
// the cut's plane x = 0 runs through two corners of the top cap, and where
// the cap's loop starts (its plane's frame) moved the rounding there, so
// the cut crossed the cap's boundary at a corner twice or not at all.
func TestMeshSolidCutAnyLoopStart(t *testing.T) {
	cyl, _ := Cylinder(geom.World, 10, 25)
	pos, tris := meshOf(cyl, 0.05)
	c, err := MeshSolid("cyl", pos, tris)
	if err != nil {
		t.Fatal(err)
	}
	idx := map[*brep.Vertex]int{}
	var pts []geom.Vec3
	var faces [][]int
	for _, f := range c.Faces() {
		var loop []int
		for _, h := range f.Loops[0].HalfEdges() {
			v := h.Edge.Verts[0]
			if !h.Fwd {
				v = h.Edge.Verts[1]
			}
			i, ok := idx[v]
			if !ok {
				i = len(pts)
				idx[v] = i
				pts = append(pts, v.P)
			}
			loop = append(loop, i)
		}
		faces = append(faces, loop)
	}
	rng := rand.New(rand.NewSource(7))
	for run := 0; run < 12; run++ {
		rot := make([][]int, len(faces))
		for i, f := range faces {
			k := rng.Intn(len(f))
			rot[i] = append(append([]int{}, f[k:]...), f[:k]...)
		}
		b, err := Polyhedron("cyl", pts, rot, nil)
		if err != nil {
			t.Fatal(err)
		}
		cut, _ := Box(geom.V3(0, -20, 10), geom.V3(20, 20, 30))
		r, err := brep.Boolean(b, cut, brep.Subtract)
		if err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		checkSolid(t, r)
	}
}
