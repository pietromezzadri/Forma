package io

// 3MF: the 3D Manufacturing Format slicers read (PrusaSlicer, Bambu Studio,
// Cura, OrcaSlicer). A zip holding the model as XML: one object a body, its
// mesh with shared (welded) vertices so it is watertight, millimetres, and
// a colour each from the core spec's base materials, so a design in
// several colours comes into a slicer already split into its parts.

import (
	"archive/zip"
	"bufio"
	"encoding/xml"
	"fmt"
	stdio "io"
	"math"
	"os"
	"strconv"
	"strings"

	"forma/brep"
	"forma/geom"
)

// ThreeMFItem is one object of a 3MF file.
type ThreeMFItem struct {
	Name  string
	Mesh  *brep.Mesh
	Color [3]float64 // red, green, blue from 0 to 1
}

// Write3MF writes the items as a 3MF package (units: mm). app names the
// program in the file's metadata, title the design.
func Write3MF(w stdio.Writer, title, app string, items []ThreeMFItem, together bool) error {
	if len(items) == 0 {
		return fmt.Errorf("nothing to write")
	}
	z := zip.NewWriter(w)
	put := func(name, body string) error {
		f, err := z.Create(name)
		if err != nil {
			return err
		}
		_, err = stdio.WriteString(f, body)
		return err
	}
	if err := put("[Content_Types].xml", xml.Header+`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
 <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
 <Default Extension="model" ContentType="application/vnd.ms-package.3dmanufacturing-3dmodel+xml"/>
</Types>
`); err != nil {
		return err
	}
	if err := put("_rels/.rels", xml.Header+`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
 <Relationship Target="/3D/3dmodel.model" Id="rel0" Type="http://schemas.microsoft.com/3dmanufacturing/2013/01/3dmodel"/>
</Relationships>
`); err != nil {
		return err
	}
	f, err := z.Create("3D/3dmodel.model")
	if err != nil {
		return err
	}
	if err := writeModel(f, title, app, items, together); err != nil {
		return err
	}
	return z.Close()
}

// Save3MF writes the items to a 3MF file.
func Save3MF(path, title, app string, items []ThreeMFItem, together bool) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := Write3MF(f, title, app, items, together); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func writeModel(w stdio.Writer, title, app string, items []ThreeMFItem, together bool) error {
	bw := bufio.NewWriter(w)
	esc := func(s string) string {
		var b strings.Builder
		xml.EscapeText(&b, []byte(s))
		return strings.ReplaceAll(b.String(), `"`, "&quot;")
	}
	num := func(x float64) string {
		return strconv.FormatFloat(math.Round(x*1e6)/1e6, 'f', -1, 64)
	}
	fmt.Fprint(bw, xml.Header)
	fmt.Fprintln(bw, `<model unit="millimeter" xml:lang="en-US" xmlns="http://schemas.microsoft.com/3dmanufacturing/core/2015/02">`)
	fmt.Fprintf(bw, " <metadata name=\"Application\">%s</metadata>\n", esc(app))
	fmt.Fprintf(bw, " <metadata name=\"Title\">%s</metadata>\n", esc(title))
	fmt.Fprintln(bw, " <resources>")
	// a base material for each colour
	fmt.Fprintln(bw, `  <basematerials id="1">`)
	colours := map[string]int{}
	pindex := make([]int, len(items))
	for i, it := range items {
		c := hexColour(it.Color)
		k, ok := colours[c]
		if !ok {
			k = len(colours)
			colours[c] = k
			fmt.Fprintf(bw, "   <base name=\"%s\" displaycolor=\"%sFF\"/>\n", esc(c), c)
		}
		pindex[i] = k
	}
	fmt.Fprintln(bw, "  </basematerials>")
	for i, it := range items {
		pos, tris := weld(it.Mesh)
		if len(tris) == 0 {
			return fmt.Errorf("%s has no triangles", it.Name)
		}
		fmt.Fprintf(bw, "  <object id=\"%d\" type=\"model\" name=\"%s\" pid=\"1\" pindex=\"%d\">\n   <mesh>\n    <vertices>\n", i+2, esc(it.Name), pindex[i])
		for _, p := range pos {
			fmt.Fprintf(bw, "     <vertex x=\"%s\" y=\"%s\" z=\"%s\"/>\n", num(p.X), num(p.Y), num(p.Z))
		}
		fmt.Fprintln(bw, "    </vertices>\n    <triangles>")
		for _, t := range tris {
			fmt.Fprintf(bw, "     <triangle v1=\"%d\" v2=\"%d\" v3=\"%d\"/>\n", t[0], t[1], t[2])
		}
		fmt.Fprintln(bw, "    </triangles>\n   </mesh>\n  </object>")
	}
	if together && len(items) > 1 {
		// one object made of the parts, as they stand to each other (a
		// slicer keeps them together, each part its own colour)
		fmt.Fprintf(bw, "  <object id=\"%d\" type=\"model\" name=\"%s\">\n   <components>\n", len(items)+2, esc(title))
		for i := range items {
			fmt.Fprintf(bw, "    <component objectid=\"%d\"/>\n", i+2)
		}
		fmt.Fprintln(bw, "   </components>\n  </object>")
		fmt.Fprintf(bw, " </resources>\n <build>\n  <item objectid=\"%d\"/>\n", len(items)+2)
	} else {
		fmt.Fprintln(bw, " </resources>\n <build>")
		for i := range items {
			fmt.Fprintf(bw, "  <item objectid=\"%d\"/>\n", i+2)
		}
	}
	fmt.Fprintln(bw, " </build>\n</model>")
	return bw.Flush()
}

func hexColour(c [3]float64) string {
	b := func(x float64) int { return max(0, min(255, int(x*255+0.5))) }
	return fmt.Sprintf("#%02X%02X%02X", b(c[0]), b(c[1]), b(c[2]))
}

// weld joins a tessellation's vertices that lie within a micrometre of each
// other (each face is tessellated with its own copies of the points along
// its edges), so the mesh is one closed surface, and drops the triangles
// that leaves with no area.
func weld(m *brep.Mesh) ([]geom.Vec3, [][3]int) {
	const tol = 1e-6
	const cell = 1e-4
	type key [3]int64
	at := func(p geom.Vec3) key {
		return key{int64(math.Floor(p.X / cell)), int64(math.Floor(p.Y / cell)), int64(math.Floor(p.Z / cell))}
	}
	grid := map[key][]int{}
	var pos []geom.Vec3
	index := make([]int, len(m.Pos))
	for i, p := range m.Pos {
		k := at(p)
		found := -1
	look:
		for dx := int64(-1); dx <= 1; dx++ {
			for dy := int64(-1); dy <= 1; dy++ {
				for dz := int64(-1); dz <= 1; dz++ {
					for _, j := range grid[key{k[0] + dx, k[1] + dy, k[2] + dz}] {
						if pos[j].Dist(p) <= tol {
							found = j
							break look
						}
					}
				}
			}
		}
		if found < 0 {
			found = len(pos)
			pos = append(pos, p)
			grid[k] = append(grid[k], found)
		}
		index[i] = found
	}
	var tris [][3]int
	for _, t := range m.Tris {
		a, b, c := index[t[0]], index[t[1]], index[t[2]]
		if a == b || b == c || a == c {
			continue
		}
		tris = append(tris, [3]int{a, b, c})
	}
	return pos, tris
}
