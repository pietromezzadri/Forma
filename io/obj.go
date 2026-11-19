package io

import (
	"bufio"
	"fmt"
	stdio "io"
	"os"
	"path/filepath"
	"strconv"

	"forma/brep"
)

// OBJItem is one named mesh of an OBJ file.
type OBJItem struct {
	Name string
	Mesh *brep.Mesh
}

// WriteOBJ writes meshes as Wavefront OBJ (millimetres), one object each,
// with vertex normals and every B-rep face as its own group.
func WriteOBJ(w stdio.Writer, items []OBJItem) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "# Forma\n")
	base := 1
	for _, it := range items {
		m := it.Mesh
		fmt.Fprintf(bw, "o %s\n", objName(it.Name))
		for _, p := range m.Pos {
			fmt.Fprintf(bw, "v %s %s %s\n", objNum(p.X), objNum(p.Y), objNum(p.Z))
		}
		for _, n := range m.Nrm {
			fmt.Fprintf(bw, "vn %s %s %s\n", objNum(n.X), objNum(n.Y), objNum(n.Z))
		}
		face := -1
		for ti, t := range m.Tris {
			if ti < len(m.TriFace) && m.TriFace[ti] != face {
				face = m.TriFace[ti]
				fmt.Fprintf(bw, "g face%d\n", face)
			}
			a, b, c := int(t[0])+base, int(t[1])+base, int(t[2])+base
			fmt.Fprintf(bw, "f %d//%d %d//%d %d//%d\n", a, a, b, b, c, c)
		}
		base += len(m.Pos)
	}
	return bw.Flush()
}

// SaveOBJ writes an OBJ file.
func SaveOBJ(path string, items []OBJItem) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := WriteOBJ(f, items); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func objNum(x float64) string { return strconv.FormatFloat(x, 'g', 9, 64) }

func objName(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c <= ' ' {
			b[i] = '_'
		}
	}
	if len(b) == 0 {
		return "body"
	}
	return string(b)
}
