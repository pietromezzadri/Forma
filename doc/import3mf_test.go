package doc

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"forma/brep"
	fio "forma/io"
)

// A box and a cylinder written to 3MF in two colours, brought into another
// design: two solid bodies with the file's names and colours, as big as
// they were (the box with its six faces), worked on (split), and kept
// through a save; a picked colour beats the file's.
func TestImport3MF(t *testing.T) {
	src := New()
	bx := src.Insert(NewPrim(KBox))
	bx.Inputs["Width"], bx.Inputs["Depth"], bx.Inputs["Height"] = "40", "30", "20"
	cy := src.Insert(NewPrim(KCylinder))
	cy.Inputs["Radius"], cy.Inputs["Height"], cy.Inputs["X"] = "10", "25", "80"
	src.Regenerate()
	var items []fio.ThreeMFItem
	for _, r := range src.Bodies {
		opt := brep.DefaultMeshOptions(r.Body)
		opt.ChordTol = 0.02
		col := [3]float64{0.9, 0.5, 0.1}
		if r.Owner == cy.ID {
			col = [3]float64{0.1, 0.4, 0.8}
		}
		items = append(items, fio.ThreeMFItem{Name: r.Name, Mesh: brep.Tessellate(r.Body, opt), Color: col})
	}
	var buf bytes.Buffer
	if err := fio.Write3MF(&buf, "src", "Forma test", items, true); err != nil {
		t.Fatal(err)
	}
	parts, tris, err := Check3MF(buf.Bytes())
	if err != nil || parts != 2 {
		t.Fatalf("check: %d parts, %d triangles, %v", parts, tris, err)
	}
	d := New()
	f := d.Insert(NewImport3MF(`C:\x\parts.3mf`, buf.Bytes()))
	d.Regenerate()
	if f.Err != "" || f.Warn != "" {
		t.Fatalf("import: %q %q", f.Err, f.Warn)
	}
	if len(d.Bodies) != 2 || d.Bodies[0].Name != "Box 1" || d.Bodies[1].Name != "Cylinder 1" {
		var names []string
		for _, r := range d.Bodies {
			names = append(names, r.Name)
		}
		t.Fatalf("bodies %v", names)
	}
	if n := len(d.Bodies[0].Body.Faces()); n != 6 {
		t.Fatalf("the box came in with %d faces", n)
	}
	vb, vc := volume(t, d.Bodies[0].Body), volume(t, d.Bodies[1].Body)
	if math.Abs(vb-24000) > 1e-3 || math.Abs(vc-math.Pi*100*25) > 0.01*math.Pi*100*25 {
		t.Fatalf("volumes %.2f %.2f", vb, vc)
	}
	if ColorOf(f, 0) != "#e6801a" || ColorOf(f, 1) != "#1a66cc" {
		t.Fatalf("colours %q %q", ColorOf(f, 0), ColorOf(f, 1))
	}
	// worked on: the imported cylinder split across its middle
	sp := d.Insert(NewSplit(d.Bodies[1].Key, PlaneRef{Std: "xy"}))
	sp.Inputs["Offset"] = "12.5"
	d.Regenerate()
	if sp.Err != "" || len(d.Bodies) != 3 {
		t.Fatalf("split: %q, %d bodies", sp.Err, len(d.Bodies))
	}
	// a colour picked for a body beats the file's, and both survive a save
	f.BodyColors = map[int]string{0: "#00ff00"}
	d2, err := Unmarshal(d.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	g := d2.Feature(f.ID)
	if len(d2.Bodies) != 3 || ColorOf(g, 0) != "#00ff00" || ColorOf(g, 1) != "#1a66cc" {
		t.Fatalf("reloaded: %d bodies, colours %q %q", len(d2.Bodies), ColorOf(g, 0), ColorOf(g, 1))
	}
	// not a 3MF
	if _, _, err := Check3MF([]byte("solid x")); err == nil || !strings.Contains(err.Error(), "3MF") {
		t.Fatalf("not a 3MF: %v", err)
	}
}
