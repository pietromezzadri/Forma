package io

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"math"
	"testing"

	"forma/brep"
	"forma/geom"
	"forma/ops"
)

// A box and a cylinder in two colours to 3MF and back: the package's three
// parts, two objects with their names and colours, every mesh closed (each
// edge used once each way) and as big as the solid.
func TestWrite3MF(t *testing.T) {
	box, err := ops.Box(geom.V3(0, 0, 0), geom.V3(40, 30, 20))
	if err != nil {
		t.Fatal(err)
	}
	cyl, err := ops.Cylinder(geom.Frame{O: geom.V3(60, 0, 0), X: geom.XAxis, Y: geom.YAxis, Z: geom.ZAxis}, 10, 25)
	if err != nil {
		t.Fatal(err)
	}
	mesh := func(b *brep.Body) *brep.Mesh {
		opt := brep.DefaultMeshOptions(b)
		opt.ChordTol = 0.01
		return brep.Tessellate(b, opt)
	}
	items := []ThreeMFItem{
		{Name: "Box & lid", Mesh: mesh(box), Color: [3]float64{0.75, 0.22, 0.17}},
		{Name: "Peg", Mesh: mesh(cyl), Color: [3]float64{0.1, 0.4, 0.8}},
	}
	var buf bytes.Buffer
	if err := Write3MF(&buf, "test", "Forma test", items, false); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, f := range zr.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		b.ReadFrom(r)
		r.Close()
		files[f.Name] = b.Bytes()
	}
	for _, n := range []string{"[Content_Types].xml", "_rels/.rels", "3D/3dmodel.model"} {
		if _, ok := files[n]; !ok {
			t.Fatalf("no %s in the package", n)
		}
	}
	var model struct {
		Unit  string `xml:"unit,attr"`
		Bases []struct {
			Color string `xml:"displaycolor,attr"`
		} `xml:"resources>basematerials>base"`
		Objects []struct {
			ID     int    `xml:"id,attr"`
			Name   string `xml:"name,attr"`
			PIndex int    `xml:"pindex,attr"`
			Tris   []struct {
				V1 int `xml:"v1,attr"`
				V2 int `xml:"v2,attr"`
				V3 int `xml:"v3,attr"`
			} `xml:"mesh>triangles>triangle"`
		} `xml:"resources>object"`
		Items []struct {
			Object int `xml:"objectid,attr"`
		} `xml:"build>item"`
	}
	if err := xml.Unmarshal(files["3D/3dmodel.model"], &model); err != nil {
		t.Fatal(err)
	}
	if model.Unit != "millimeter" || len(model.Objects) != 2 || len(model.Items) != 2 || len(model.Bases) != 2 {
		t.Fatalf("unit %q, %d objects, %d build items, %d colours", model.Unit, len(model.Objects), len(model.Items), len(model.Bases))
	}
	if model.Objects[0].Name != "Box & lid" || model.Bases[model.Objects[0].PIndex].Color != "#BF382BFF" || model.Bases[model.Objects[1].PIndex].Color != "#1A66CCFF" {
		t.Fatalf("names / colours: %q %v", model.Objects[0].Name, model.Bases)
	}
	// the vertices, read on their own
	var coords struct {
		Objects []struct {
			Verts []struct {
				X float64 `xml:"x,attr"`
				Y float64 `xml:"y,attr"`
				Z float64 `xml:"z,attr"`
			} `xml:"mesh>vertices>vertex"`
		} `xml:"resources>object"`
	}
	xml.Unmarshal(files["3D/3dmodel.model"], &coords)
	for k, o := range model.Objects {
		vs := coords.Objects[k].Verts
		edges := map[[2]int]int{}
		vol := 0.0
		for _, tr := range o.Tris {
			ids := [3]int{tr.V1, tr.V2, tr.V3}
			for i := 0; i < 3; i++ {
				edges[[2]int{ids[i], ids[(i+1)%3]}]++
			}
			p := func(i int) geom.Vec3 { return geom.V3(vs[i].X, vs[i].Y, vs[i].Z) }
			vol += p(ids[0]).Dot(p(ids[1]).Cross(p(ids[2]))) / 6
		}
		for e, n := range edges {
			if n != 1 || edges[[2]int{e[1], e[0]}] != 1 {
				t.Fatalf("%s: edge %v used %d times this way, %d the other: not closed", o.Name, e, n, edges[[2]int{e[1], e[0]}])
			}
		}
		want := []float64{40 * 30 * 20, math.Pi * 100 * 25}[k]
		t.Logf("%s: %d vertices, %d triangles, volume %.1f of %.1f", o.Name, len(vs), len(o.Tris), vol, want)
		if math.Abs(vol-want) > 0.002*want {
			t.Fatalf("%s: volume %.2f, want %.2f", o.Name, vol, want)
		}
	}
}
