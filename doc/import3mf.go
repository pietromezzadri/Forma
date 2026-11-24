package doc

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"

	"forma/geom"
	fio "forma/io"
	"forma/ops"
)

// A 3MF brought into a design: each part the file puts on the plate a
// solid body (its flat regions merged into faces, ops.MeshSolid), named
// and coloured as the file has it. The file is kept in the feature like a
// STEP import's text, so a design stays whole; a file too big for that is
// refused, with the way to print it (separating it into part files).

// Import3MFMax is the most triangles a 3MF may bring into a design.
const Import3MFMax = 300000

// NewImport3MF makes an import feature of a 3MF file's parts.
func NewImport3MF(file string, data []byte) *Feature {
	return withInputs(&Feature{Kind: KImport, Name: baseName(file), Data: base64.StdEncoding.EncodeToString(data),
		Inputs: map[string]string{"File": filepath.Base(file), "Format": "3mf"}}, ImportInputs)
}

// Check3MF reads a 3MF's parts and says whether it can come into a design
// (nil) or why not, before it is put in.
func Check3MF(data []byte) (parts int, triangles int, err error) {
	// a model of more than about 200 bytes a triangle over the limit is
	// refused before it is read (a gigabyte takes a while)
	if n, err := fio.Model3MFSize(data); err != nil {
		return 0, 0, err
	} else if n > 200*Import3MFMax {
		return 0, 0, fmt.Errorf("its model is %d MB: too much to bring into a design (at most about %d triangles). To print it, use File → Separate a 3MF into part files", n>>20, Import3MFMax)
	}
	ps, err := fio.Read3MFPartsFrom(data)
	if err != nil {
		return 0, 0, err
	}
	for _, p := range ps {
		triangles += p.Triangles()
	}
	if triangles > Import3MFMax {
		return len(ps), triangles, fmt.Errorf("%d parts of %d triangles in all: too much to bring into a design (at most %d). To print it, use File → Separate a 3MF into part files", len(ps), triangles, Import3MFMax)
	}
	return len(ps), triangles, nil
}

func (d *Document) build3MF(f *Feature) error {
	data, err := base64.StdEncoding.DecodeString(f.Data)
	if err != nil {
		return fmt.Errorf("3MF data: %w", err)
	}
	parts, err := fio.Read3MFPartsFrom(data)
	if err != nil {
		return err
	}
	f.PartNames, f.PartColors = nil, nil
	var skipped []string
	for k, p := range parts {
		pos := make([]geom.Vec3, len(p.Pos)/3)
		for i := range pos {
			pos[i] = geom.V3(float64(p.Pos[3*i]), float64(p.Pos[3*i+1]), float64(p.Pos[3*i+2]))
		}
		tris := make([][3]int, len(p.Tris)/3)
		for i := range tris {
			tris[i] = [3]int{int(p.Tris[3*i]), int(p.Tris[3*i+1]), int(p.Tris[3*i+2])}
		}
		b, err := ops.MeshSolid(fmt.Sprintf("p%d", k+1), pos, tris)
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("%s: %v", p.Name, err))
			continue
		}
		col := ""
		if p.HasColor {
			col = FormatColor(p.Color)
		}
		f.Bodies = append(f.Bodies, b)
		f.PartNames = append(f.PartNames, p.Name)
		f.PartColors = append(f.PartColors, col)
	}
	if len(f.Bodies) == 0 {
		if len(skipped) == 1 {
			return fmt.Errorf("%s", skipped[0])
		}
		return fmt.Errorf("none of the %d parts could be made solids: %s", len(parts), strings.Join(skipped, "; "))
	}
	if len(skipped) > 0 {
		f.Warn = fmt.Sprintf("%d of %d parts left out: %s", len(skipped), len(parts), strings.Join(skipped, "; "))
	}
	for i, b := range f.Bodies {
		b.Name = f.BodyName(i)
	}
	return nil
}
