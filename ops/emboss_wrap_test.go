package ops

import (
	"fmt"
	"testing"
	"time"

	"forma/brep"
	"forma/geom"
)

// softSolid is checkSolid's checks as a message (the case goes on).
func softSolid(b *brep.Body) (float64, error) {
	if err := brep.Check(b); err != nil {
		return 0, err
	}
	opt := brep.DefaultMeshOptions(b)
	opt.ChordTol = b.Bounds().Diagonal() * 2e-5
	m := brep.Tessellate(b, opt)
	if err := m.Watertight(1e-6); err != nil {
		return m.Volume(), fmt.Errorf("mesh: %v", err)
	}
	return m.Volume(), nil
}

// "Luna" wrapped round small cylinders, cut in and inlaid: each letter is
// cut along its own normal, so neighbours' cuts close in under the surface.
func TestEmbossWrapNeighbours(t *testing.T) {
	for _, rad := range []float64{8, 10, 12} {
		for _, size := range []float64{8, 10} {
			cyl, err := Cylinder(geom.World, rad, 30)
			if err != nil {
				t.Fatal(err)
			}
			v0 := checkSolid(t, cyl)
			regs, fr := nameSketch(t, "Luna", -60, 15, size, 0)
			grown, _ := nameSketch(t, "Luna", -60, 15, size, 0.2)
			area, garea := inkArea(regs), inkArea(grown)
			for _, c := range []struct {
				name string
				sp   EmbossSpec
			}{
				{"cut", EmbossSpec{Mode: EmbossCut, Depth: 2, Wrap: true}},
				{"inlay", EmbossSpec{Mode: EmbossInlay, Depth: 2, Clearance: 0.2, Wrap: true}},
			} {
				name := fmt.Sprintf("r%g size %g %s", rad, size, c.name)
				t0 := time.Now()
				b, inl, err := EmbossFace(cyl, sideOf(cyl), regs, grown, fr, c.sp)
				if err != nil {
					t.Errorf("%s: %v", name, err)
					continue
				}
				v, err := softSolid(b)
				if err != nil {
					t.Errorf("%s: %v", name, err)
				}
				a := area
				if c.sp.Mode == EmbossInlay {
					a = garea
				}
				dv := v0 - v
				// cut in, the letters shrink toward the axis and the trims
				// take a little off underneath
				if dv < 0.7*a*2 || dv > 1.1*a*2 {
					t.Errorf("%s: removed %.1f mm³ for %.1f mm² × 2", name, dv, a)
				}
				vin := 0.0
				for _, l := range inl {
					vl, err := softSolid(l)
					if err != nil {
						t.Errorf("%s inlay: %v", name, err)
					}
					vin += vl
					if _, err := brep.Boolean(l, b, brep.Intersect); err != brep.ErrEmpty {
						t.Errorf("%s: an inlay overlaps the body (%v)", name, err)
					}
				}
				t.Logf("%s: removed %.1f mm³ (%.1f mm² letters), inlays %d, %.1f mm³ (%v)", name, dv, a, len(inl), vin, time.Since(t0))
			}
		}
	}
}
