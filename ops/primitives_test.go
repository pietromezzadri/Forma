package ops

import (
	"math"
	"testing"

	"forma/brep"
	"forma/geom"
)

type primCase struct {
	name         string
	make         func() (*brep.Body, error)
	v, e, f      int
	genus        int
	volume, area float64
}

func primCases() []primCase {
	slant := math.Hypot(20-8, 30)
	tilted := geom.FrameZ(geom.V3(5, -3, 2), geom.V3(1, 1, 2), geom.XAxis)
	return []primCase{
		{"box", func() (*brep.Body, error) { return Box(geom.V3(0, 0, 0), geom.V3(10, 20, 30)) },
			8, 12, 6, 0, 6000, 2 * (200 + 300 + 600)},
		{"cylinder", func() (*brep.Body, error) { return Cylinder(geom.World, 10, 25) },
			2, 3, 3, 0, math.Pi * 100 * 25, 2*math.Pi*100 + 2*math.Pi*10*25},
		{"tilted cylinder", func() (*brep.Body, error) { return Cylinder(tilted, 4, 12) },
			2, 3, 3, 0, math.Pi * 16 * 12, 2*math.Pi*16 + 2*math.Pi*4*12},
		{"frustum", func() (*brep.Body, error) { return Cone(geom.World, 20, 8, 30) },
			2, 3, 3, 0, math.Pi * 30 * (400 + 160 + 64) / 3, math.Pi*(400+64) + math.Pi*(28)*slant},
		{"cone", func() (*brep.Body, error) { return Cone(geom.World, 10, 0, 20) },
			2, 2, 2, 0, math.Pi * 100 * 20 / 3, math.Pi*100 + math.Pi*10*math.Hypot(10, 20)},
		{"sphere", func() (*brep.Body, error) { return Sphere(geom.V3(1, 2, 3), 15) },
			2, 1, 1, 0, 4.0 / 3 * math.Pi * 15 * 15 * 15, 4 * math.Pi * 225},
		{"torus", func() (*brep.Body, error) { return Torus(geom.World, 30, 8) },
			1, 2, 1, 1, 2 * math.Pi * math.Pi * 30 * 64, 4 * math.Pi * math.Pi * 30 * 8},
	}
}

func TestPrimitives(t *testing.T) {
	for _, c := range primCases() {
		t.Run(c.name, func(t *testing.T) {
			b, err := c.make()
			if err != nil {
				t.Fatal(err)
			}
			if err := brep.Check(b); err != nil {
				t.Fatal(err)
			}
			v, e, f, _, _ := b.Counts()
			if v != c.v || e != c.e || f != c.f {
				t.Errorf("counts V%d E%d F%d, want V%d E%d F%d", v, e, f, c.v, c.e, c.f)
			}
			if g := brep.Genus(b); g != c.genus {
				t.Errorf("genus %d, want %d", g, c.genus)
			}
			opt := brep.DefaultMeshOptions(b)
			opt.ChordTol = b.Bounds().Diagonal() * 2e-5
			m := brep.Tessellate(b, opt)
			if err := m.Watertight(1e-6); err != nil {
				t.Errorf("mesh not watertight: %v", err)
			}
			vol, area := m.Volume(), m.Area()
			if rel := math.Abs(vol-c.volume) / c.volume; rel > 2e-3 {
				t.Errorf("volume %.4f, want %.4f (%.3f%% off)", vol, c.volume, rel*100)
			}
			if rel := math.Abs(area-c.area) / c.area; rel > 2e-3 {
				t.Errorf("area %.4f, want %.4f (%.3f%% off)", area, c.area, rel*100)
			}
			t.Logf("%d triangles, volume %.2f (%.4f%%), area %.2f", len(m.Tris), vol, 100*(vol-c.volume)/c.volume, area)
		})
	}
}

func TestBadPrimitives(t *testing.T) {
	if _, err := Box(geom.V3(0, 0, 0), geom.V3(1, 0, 1)); err == nil {
		t.Error("flat box accepted")
	}
	if _, err := Torus(geom.World, 5, 6); err == nil {
		t.Error("self-intersecting torus accepted")
	}
}

// The default (display) tolerance must give closed meshes too.
func TestPrimitivesDefaultMesh(t *testing.T) {
	for _, c := range primCases() {
		b, err := c.make()
		if err != nil {
			t.Fatal(err)
		}
		m := brep.Tessellate(b, brep.DefaultMeshOptions(b))
		if err := m.Watertight(1e-6); err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if rel := math.Abs(m.Volume()-c.volume) / c.volume; rel > 0.01 {
			t.Errorf("%s: volume %.1f%% off", c.name, rel*100)
		}
		t.Logf("%s: %d triangles", c.name, len(m.Tris))
	}
}
