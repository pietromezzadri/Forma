package doc

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"

	"forma/brep"
	"forma/geom"
	fio "forma/io"
)

// ImportInputs: the source file's name (for show; the file itself is kept in
// the feature) and the operation.
var ImportInputs = [][2]string{{"File", ""}, {"Operation", "New body"}}

// NewImport makes a feature of the solids in a STEP file's text. The text
// is kept in the design, so the .forma file stands on its own.
func NewImport(file, step string) *Feature {
	return withInputs(&Feature{Kind: KImport, Name: baseName(file), Data: step,
		Inputs: map[string]string{"File": filepath.Base(file)}}, ImportInputs)
}

// NewMesh makes a reference mesh of an STL file's triangles: shown, not
// part of the solids (no booleans).
func NewMesh(file string, stl []byte) *Feature {
	return &Feature{Kind: KMesh, Name: baseName(file), Data: base64.StdEncoding.EncodeToString(stl),
		Inputs: map[string]string{"File": filepath.Base(file)}}
}

func baseName(file string) string {
	b := filepath.Base(file)
	return strings.TrimSuffix(b, filepath.Ext(b))
}

func (d *Document) buildImport(f *Feature) error {
	if f.Inputs["Format"] == "3mf" {
		return d.build3MF(f)
	}
	solids, err := fio.ReadSTEPString(f.Data)
	if err != nil {
		return err
	}
	var skipped []string
	for _, s := range solids {
		if s.Err != nil {
			skipped = append(skipped, fmt.Sprintf("%s: %v", s.Name, s.Err))
			continue
		}
		f.Bodies = append(f.Bodies, s.Body)
	}
	if len(f.Bodies) == 0 {
		if len(skipped) == 1 {
			return fmt.Errorf("%s", skipped[0])
		}
		return fmt.Errorf("none of the %d solids could be read: %s", len(solids), strings.Join(skipped, "; "))
	}
	if len(skipped) > 0 {
		f.Warn = fmt.Sprintf("%d of %d solids left out: %s", len(skipped), len(solids), strings.Join(skipped, "; "))
	}
	for i, b := range f.Bodies {
		b.Name = f.BodyName(i)
	}
	return nil
}

func (d *Document) buildMesh(f *Feature) error {
	raw, err := base64.StdEncoding.DecodeString(f.Data)
	if err != nil {
		return fmt.Errorf("mesh data: %w", err)
	}
	tris, err := fio.ReadSTL(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	if len(tris) == 0 {
		return fmt.Errorf("the file has no triangles")
	}
	m := &brep.Mesh{}
	for _, t := range tris {
		n := t.B.Sub(t.A).Cross(t.C.Sub(t.A))
		if n.Len() == 0 {
			continue
		}
		n = n.Norm()
		k := int32(len(m.Pos))
		m.Pos = append(m.Pos, t.A, t.B, t.C)
		m.Nrm = append(m.Nrm, n, n, n)
		m.Tris = append(m.Tris, [3]int32{k, k + 1, k + 2})
		m.TriFace = append(m.TriFace, 1)
	}
	f.RefMesh = m
	return nil
}

// MeshBounds is a reference mesh's bounding box.
func MeshBounds(m *brep.Mesh) geom.Box {
	var b geom.Box
	for _, p := range m.Pos {
		b.Add(p)
	}
	return b
}
