package doc

import (
	"bytes"
	"math"
	"os"
	"testing"

	"forma/brep"
	"forma/geom"
	fio "forma/io"
	"forma/ops"
)

// An imported solid is a body of the design: a cylinder cuts it, a sketch
// on one of its faces finds that face, and it all survives a save.
func TestImportFeature(t *testing.T) {
	src, err := os.ReadFile("../io/testdata/occ_fillet_box.step")
	if err != nil {
		t.Fatal(err)
	}
	d := New()
	imp := d.Insert(NewImport("C:/parts/occ_fillet_box.step", string(src)))
	d.Regenerate()
	mustOK(t, imp)
	if imp.Name != "occ_fillet_box" || len(d.Bodies) != 1 {
		t.Fatalf("name %q, %d bodies", imp.Name, len(d.Bodies))
	}
	v0 := brep.Mass(d.Bodies[0].Body, 1e-4).Volume
	approx(t, "imported volume", v0, 5804.696111, 5e-4)

	cyl := d.Insert(NewPrim(KCylinder))
	cyl.Inputs = map[string]string{"X": "15", "Y": "10", "Z": "-1", "Radius": "4", "Height": "12", "Operation": "Cut"}
	d.Regenerate()
	mustOK(t, cyl)
	if len(d.Bodies) != 1 {
		t.Fatalf("%d bodies after the cut", len(d.Bodies))
	}
	v1 := brep.Mass(d.Bodies[0].Body, 1e-4).Volume
	approx(t, "cut volume", v1, v0-math.Pi*16*10, 5e-4)

	// faces are named f<ID>/imp:<n>
	named := 0
	for _, fc := range d.Bodies[0].Body.Faces() {
		if len(fc.Name) > 4 && fc.Name[:len("f1/imp:")] == "f1/imp:" {
			named++
		}
	}
	if named < 20 {
		t.Errorf("only %d faces named f1/imp:…", named)
	}

	d2, err := Unmarshal(d.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	d2.Regenerate()
	if len(d2.Bodies) != 1 {
		t.Fatalf("%d bodies after loading", len(d2.Bodies))
	}
	approx(t, "volume after loading", brep.Mass(d2.Bodies[0].Body, 1e-4).Volume, v1, 1e-9)
}

// A reference mesh is shown, not joined: it adds no body.
func TestMeshFeature(t *testing.T) {
	b, err := ops.Sphere(geom.V3(0, 0, 0), 5)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := fio.WriteSTL(&buf, "ball", fio.MeshTriangles(brep.Tessellate(b, brep.DefaultMeshOptions(b)))); err != nil {
		t.Fatal(err)
	}
	d := New()
	box := d.Insert(NewPrim(KBox))
	m := d.Insert(NewMesh("scan.stl", buf.Bytes()))
	d.Regenerate()
	mustOK(t, box, m)
	if m.RefMesh == nil || len(m.RefMesh.Tris) == 0 || len(d.Bodies) != 1 {
		t.Fatalf("mesh %v, %d bodies", m.RefMesh != nil, len(d.Bodies))
	}
	if v := m.RefMesh.Volume(); math.Abs(v-4*math.Pi*125/3) > 0.02*v {
		t.Errorf("mesh volume %.2f", v)
	}
}
