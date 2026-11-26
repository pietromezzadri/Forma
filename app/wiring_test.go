package app

import (
	"math"
	"os"
	"testing"

	"forma/doc"
)

// The wiring sheet's layout of a circuit converted from the old named
// wiring and of a Wokwi project: no wire runs through a box, and no two
// nets' vertical runs lie on one another.
func TestWiringSheetRoutes(t *testing.T) {
	old := &doc.Circuit{Board: "uno"}
	for _, m := range []string{"sg90", "battery-4aa", "tt-motor", "tb6612", "pot", "button", "led", "led"} {
		old.AddPart(m, "")
	}
	diagram, err := os.ReadFile("../doc/testdata/wokwi_button/diagram.json")
	if err != nil {
		t.Fatal(err)
	}
	wk, _, err := doc.ReadWokwi(diagram, "")
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]*doc.Circuit{"converted": old, "wokwi": wk} {
		if len(c.Wires) < 8 {
			t.Fatalf("%s: only %d wires", name, len(c.Wires))
		}
		boxes := sheetBoxes(c)
		terms := sheetTerms(boxes)
		routes := sheetRoutes(c, boxes, terms)
		roots := wireRoots(c)
		type run struct {
			x, lo, hi float64
			net       string
		}
		var runs, hruns []run
		for i, r := range routes {
			if len(r) < 2 {
				t.Fatalf("%s: wire %+v not routed", name, c.Wires[i])
			}
			for k := 1; k < len(r); k++ {
				a, b := r[k-1], r[k]
				// through a box: a point well inside one
				for s := 0.05; s < 1; s += 0.05 {
					p := a.Add(b.Sub(a).Scale(s))
					for _, bx := range boxes {
						if p.X > bx.at.X+1 && p.X < bx.at.X+bx.w-1 && p.Y > bx.at.Y+1 && p.Y < bx.at.Y+bx.h-1 {
							t.Fatalf("%s: wire %s–%s runs through %s at %v", name, c.Wires[i].A, c.Wires[i].B, bx.name, p)
						}
					}
				}
				if math.Abs(a.X-b.X) < 1e-9 && math.Abs(a.Y-b.Y) > 1e-9 {
					runs = append(runs, run{a.X, math.Min(a.Y, b.Y), math.Max(a.Y, b.Y), roots[i]})
				} else if math.Abs(a.Y-b.Y) < 1e-9 {
					hruns = append(hruns, run{a.Y, math.Min(a.X, b.X), math.Max(a.X, b.X), roots[i]})
				}
			}
		}
		for i, p := range runs {
			for _, q := range runs[i+1:] {
				if p.net != q.net && math.Abs(p.x-q.x) < 2 && p.lo < q.hi-1 && q.lo < p.hi-1 {
					t.Fatalf("%s: nets %s and %s share a lane at x=%.0f", name, p.net, q.net, p.x)
				}
			}
		}
		for i, p := range hruns {
			for _, q := range hruns[i+1:] {
				if p.net != q.net && math.Abs(p.x-q.x) < 2 && p.lo < q.hi-1 && q.lo < p.hi-1 {
					t.Fatalf("%s: nets %s and %s run along each other at y=%.0f", name, p.net, q.net, p.x)
				}
			}
		}
	}
}
