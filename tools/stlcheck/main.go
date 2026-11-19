// stlcheck reads STL files and reports facets, volume and whether the
// surface is closed and consistently oriented (each edge shared by exactly
// two facets running it opposite ways).
//
//	go run ./tools/stlcheck file.stl…
package main

import (
	"fmt"
	"os"

	"forma/brep"
	fio "forma/io"
)

func main() {
	bad := false
	for _, path := range os.Args[1:] {
		f, err := os.Open(path)
		if err != nil {
			fmt.Println(err)
			bad = true
			continue
		}
		tris, err := fio.ReadSTL(f)
		f.Close()
		if err != nil {
			fmt.Printf("%s: %v\n", path, err)
			bad = true
			continue
		}
		m := &brep.Mesh{}
		for _, t := range tris {
			n := int32(len(m.Pos))
			m.Pos = append(m.Pos, t.A, t.B, t.C)
			m.Tris = append(m.Tris, [3]int32{n, n + 1, n + 2})
		}
		status := "closed and oriented"
		if err := m.Watertight(1e-4); err != nil {
			status = "NOT closed: " + err.Error()
			bad = true
		}
		fmt.Printf("%s: %d facets, volume %.3f mm³, area %.3f mm², %s\n", path, len(tris), m.Volume(), m.Area(), status)
	}
	if bad {
		os.Exit(1)
	}
}
