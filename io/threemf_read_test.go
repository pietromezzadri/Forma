package io

import (
	"archive/zip"
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forma/brep"
	"forma/geom"
	"forma/ops"
)

// meshVolume is a part's enclosed volume (positive when it faces out).
func meshVolume(p MeshPart) float64 {
	v := 0.0
	at := func(i uint32) geom.Vec3 {
		return geom.V3(float64(p.Pos[3*i]), float64(p.Pos[3*i+1]), float64(p.Pos[3*i+2]))
	}
	for i := 0; i+2 < len(p.Tris); i += 3 {
		v += at(p.Tris[i]).Dot(at(p.Tris[i+1]).Cross(at(p.Tris[i+2]))) / 6
	}
	return v
}

// A box and a cylinder written as two objects and as one object of two
// components, read back: names, volumes, the pieces of the joined one,
// each stood on the bed and written as an STL of its own.
func TestSeparate3MF(t *testing.T) {
	box, _ := ops.Box(geom.V3(0, 0, 0), geom.V3(40, 30, 20))
	cyl, _ := ops.Cylinder(geom.Frame{O: geom.V3(80, 0, 5), X: geom.XAxis, Y: geom.YAxis, Z: geom.ZAxis}, 10, 25)
	m := func(b *brep.Body) *brep.Mesh {
		opt := brep.DefaultMeshOptions(b)
		opt.ChordTol = 0.01
		return brep.Tessellate(b, opt)
	}
	items := []ThreeMFItem{{Name: "Base/plate", Mesh: m(box)}, {Name: "Peg", Mesh: m(cyl)}}
	vBox, vCyl := 40.0*30*20, math.Pi*100*25
	dir := t.TempDir()
	for _, together := range []bool{false, true} {
		path := filepath.Join(dir, map[bool]string{false: "apart.3mf", true: "together.3mf"}[together])
		if err := Save3MF(path, "test", "Forma test", items, together); err != nil {
			t.Fatal(err)
		}
		parts, err := Read3MFParts(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(parts) != 2 || parts[0].Name != "Base/plate" || parts[1].Name != "Peg" {
			t.Fatalf("together %v: %d parts %v", together, len(parts), names(parts))
		}
		if v := meshVolume(parts[0]); math.Abs(v-vBox) > 1e-3*vBox {
			t.Fatalf("box volume %.2f", v)
		}
		if !together {
			continue
		}
		// the two meshes as one: its loose pieces are the box and the peg
		joined := MeshPart{Name: "both", Pos: append(append([]float32(nil), parts[0].Pos...), parts[1].Pos...)}
		joined.Tris = append([]uint32(nil), parts[0].Tris...)
		for _, v := range parts[1].Tris {
			joined.Tris = append(joined.Tris, v+uint32(len(parts[0].Pos)/3))
		}
		pieces := LoosePieces(joined)
		if len(pieces) != 2 {
			t.Fatalf("together: %d loose pieces, want 2", len(pieces))
		}
		// (the cylinder has more triangles, so it comes first)
		v0, v1 := meshVolume(pieces[0]), meshVolume(pieces[1])
		if v0 < v1 {
			v0, v1 = v1, v0
		}
		if math.Abs(v0-vBox) > 2e-3*vBox || math.Abs(v1-vCyl) > 2e-3*vCyl {
			t.Fatalf("pieces' volumes %.1f %.1f, want %.1f %.1f", v0, v1, vBox, vCyl)
		}
		out := filepath.Join(dir, "parts")
		res, err := SeparateParts(path, out, true, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 2 {
			t.Fatalf("%d files written", len(res))
		}
		for _, r := range res {
			f, _ := os.Open(r.Path)
			tris, err := ReadSTL(f)
			f.Close()
			if err != nil || len(tris) != r.Triangles {
				t.Fatalf("%s: %d triangles read (%v), %d written", r.Path, len(tris), err, r.Triangles)
			}
			lo := math.Inf(1)
			for _, tr := range tris {
				lo = math.Min(lo, math.Min(tr.A.Z, math.Min(tr.B.Z, tr.C.Z)))
			}
			if math.Abs(lo) > 1e-4 {
				t.Fatalf("%s isn't on the bed (lowest z %g)", filepath.Base(r.Path), lo)
			}
		}
	}
	// a build item turned a quarter and moved, the names from PrusaSlicer's
	// config, and an object with no name of its own
	model := `<?xml version="1.0" encoding="UTF-8"?>
<model unit="millimeter" xmlns="http://schemas.microsoft.com/3dmanufacturing/core/2015/02">
 <resources>
  <object id="1" type="model"><mesh><vertices>
   <vertex x="0" y="0" z="0"/><vertex x="10" y="0" z="0"/><vertex x="0" y="10" z="0"/><vertex x="0" y="0" z="10"/>
  </vertices><triangles>
   <triangle v1="0" v2="2" v3="1"/><triangle v1="0" v2="1" v3="3"/><triangle v1="0" v2="3" v3="2"/><triangle v1="1" v2="2" v3="3"/>
  </triangles></mesh></object>
 </resources>
 <build><item objectid="1" transform="0 1 0 -1 0 0 0 0 1 100 50 7"/></build>
</model>`
	cfg := `<?xml version="1.0" encoding="UTF-8"?><config><object id="1" instances_count="1"><metadata type="object" key="name" value="Hůl &amp; spona"/></object></config>`
	path := filepath.Join(dir, "turned.3mf")
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for name, body := range map[string]string{"3D/3dmodel.model": model, "Metadata/Slic3r_PE_model.config": cfg} {
		w, _ := z.Create(name)
		w.Write([]byte(body))
	}
	z.Close()
	os.WriteFile(path, buf.Bytes(), 0o644)
	parts, err := Read3MFParts(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 1 || parts[0].Name != "Hůl & spona" {
		t.Fatalf("turned: %v", names(parts))
	}
	// (10, 0, 0) turned a quarter about Z is (0, 10, 0), then moved
	p := parts[0].Pos
	if math.Abs(float64(p[3])-100) > 1e-5 || math.Abs(float64(p[4])-60) > 1e-5 || math.Abs(float64(p[5])-7) > 1e-5 {
		t.Fatalf("turned: vertex 1 at %v, want (100, 60, 7)", p[3:6])
	}
	if v := meshVolume(parts[0]); math.Abs(v-1000.0/6) > 1e-3 {
		t.Fatalf("turned: volume %.4f, want %.4f", v, 1000.0/6)
	}
	if got := fileName(` a<b>:"c"/d\e|f?g*. `); strings.ContainsAny(got, `<>:"/\|?*`) || strings.HasSuffix(got, ".") {
		t.Fatalf("file name %q", got)
	}
}

func names(ps []MeshPart) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}
