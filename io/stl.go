// Package io reads and writes exchange formats: STL now; OBJ and STEP later.
package io

import (
	"bufio"
	"encoding/binary"
	"fmt"
	stdio "io"
	"math"
	"os"
	"strings"

	"forma/brep"
	"forma/geom"
)

// Triangle is one facet of an STL file.
type Triangle struct {
	N       geom.Vec3
	A, B, C geom.Vec3
}

// MeshTriangles turns tessellations into facets.
func MeshTriangles(meshes ...*brep.Mesh) []Triangle {
	var out []Triangle
	for _, m := range meshes {
		for _, t := range m.Tris {
			a, b, c := m.Pos[t[0]], m.Pos[t[1]], m.Pos[t[2]]
			out = append(out, Triangle{N: b.Sub(a).Cross(c.Sub(a)).Norm(), A: a, B: b, C: c})
		}
	}
	return out
}

// WriteSTL writes binary STL (units: mm).
func WriteSTL(w stdio.Writer, name string, tris []Triangle) error {
	bw := bufio.NewWriter(w)
	var head [80]byte
	copy(head[:], "Forma "+name)
	if _, err := bw.Write(head[:]); err != nil {
		return err
	}
	if err := binary.Write(bw, binary.LittleEndian, uint32(len(tris))); err != nil {
		return err
	}
	var rec [50]byte
	put := func(off int, v geom.Vec3) {
		binary.LittleEndian.PutUint32(rec[off:], math.Float32bits(float32(v.X)))
		binary.LittleEndian.PutUint32(rec[off+4:], math.Float32bits(float32(v.Y)))
		binary.LittleEndian.PutUint32(rec[off+8:], math.Float32bits(float32(v.Z)))
	}
	for _, t := range tris {
		put(0, t.N)
		put(12, t.A)
		put(24, t.B)
		put(36, t.C)
		rec[48], rec[49] = 0, 0
		if _, err := bw.Write(rec[:]); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// SaveSTL writes the meshes to a binary STL file.
func SaveSTL(path, name string, meshes ...*brep.Mesh) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := WriteSTL(f, name, MeshTriangles(meshes...)); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ReadSTL reads binary or ASCII STL.
func ReadSTL(r stdio.Reader) ([]Triangle, error) {
	data, err := stdio.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(data) >= 84 {
		n := binary.LittleEndian.Uint32(data[80:84])
		if int64(len(data)) == 84+int64(n)*50 {
			return readBinary(data, int(n)), nil
		}
	}
	if strings.HasPrefix(strings.TrimSpace(string(data[:min(len(data), 256)])), "solid") {
		return readASCII(string(data))
	}
	return nil, fmt.Errorf("not an STL file")
}

func readBinary(data []byte, n int) []Triangle {
	f := func(off int) geom.Vec3 {
		return geom.V3(
			float64(math.Float32frombits(binary.LittleEndian.Uint32(data[off:]))),
			float64(math.Float32frombits(binary.LittleEndian.Uint32(data[off+4:]))),
			float64(math.Float32frombits(binary.LittleEndian.Uint32(data[off+8:]))))
	}
	out := make([]Triangle, n)
	for i := range out {
		o := 84 + i*50
		out[i] = Triangle{N: f(o), A: f(o + 12), B: f(o + 24), C: f(o + 36)}
	}
	return out
}

func readASCII(s string) ([]Triangle, error) {
	var out []Triangle
	var cur Triangle
	var vs []geom.Vec3
	for _, line := range strings.Split(s, "\n") {
		fs := strings.Fields(line)
		if len(fs) == 0 {
			continue
		}
		switch fs[0] {
		case "facet":
			if len(fs) == 5 {
				fmt.Sscan(fs[2]+" "+fs[3]+" "+fs[4], &cur.N.X, &cur.N.Y, &cur.N.Z)
			}
			vs = vs[:0]
		case "vertex":
			if len(fs) != 4 {
				return nil, fmt.Errorf("bad vertex line %q", line)
			}
			var v geom.Vec3
			if _, err := fmt.Sscan(fs[1]+" "+fs[2]+" "+fs[3], &v.X, &v.Y, &v.Z); err != nil {
				return nil, err
			}
			vs = append(vs, v)
		case "endfacet":
			if len(vs) != 3 {
				return nil, fmt.Errorf("facet with %d vertices", len(vs))
			}
			cur.A, cur.B, cur.C = vs[0], vs[1], vs[2]
			out = append(out, cur)
		}
	}
	return out, nil
}
