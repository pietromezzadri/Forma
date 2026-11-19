package io

import (
	"bytes"
	"math"
	"testing"

	"forma/brep"
	"forma/geom"
	"forma/ops"
)

func TestSTLRoundTrip(t *testing.T) {
	b, err := ops.Cylinder(geom.World, 10, 20)
	if err != nil {
		t.Fatal(err)
	}
	m := brep.Tessellate(b, brep.DefaultMeshOptions(b))
	tris := MeshTriangles(m)
	var buf bytes.Buffer
	if err := WriteSTL(&buf, "cyl", tris); err != nil {
		t.Fatal(err)
	}
	if want := 84 + 50*len(tris); buf.Len() != want {
		t.Fatalf("file is %d bytes, want %d", buf.Len(), want)
	}
	back, err := ReadSTL(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != len(tris) {
		t.Fatalf("read %d facets, wrote %d", len(back), len(tris))
	}
	vol := 0.0
	for i, tr := range back {
		if tr.A.Dist(tris[i].A) > 1e-5 || tr.C.Dist(tris[i].C) > 1e-5 {
			t.Fatalf("facet %d changed", i)
		}
		vol += tr.A.Dot(tr.B.Cross(tr.C)) / 6
	}
	if rel := math.Abs(vol-m.Volume()) / m.Volume(); rel > 1e-5 {
		t.Fatalf("volume changed by %.2g", rel)
	}
}

func TestSTLASCII(t *testing.T) {
	src := `solid t
facet normal 0 0 1
 outer loop
  vertex 0 0 0
  vertex 1 0 0
  vertex 0 1 0
 endloop
endfacet
endsolid t`
	tr, err := ReadSTL(bytes.NewBufferString(src))
	if err != nil || len(tr) != 1 || tr[0].B.X != 1 {
		t.Fatalf("got %v %v", tr, err)
	}
}
