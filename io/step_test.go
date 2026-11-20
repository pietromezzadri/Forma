package io_test

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"forma/brep"
	"forma/geom"
	fio "forma/io"
	"forma/ops"
)

func boolean(t *testing.T, a, b *brep.Body, op brep.BoolOp) *brep.Body {
	t.Helper()
	r, err := brep.Boolean(a, b, op)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func at(x, y, z float64) geom.Frame {
	f := geom.World
	f.O = geom.V3(x, y, z)
	return f
}

// stepParts are bodies covering every surface and edge kind the writer
// knows: primitives, boolean edges (fitted circles and true polylines), a
// countersunk hole (a cone), fillets (cylinders and a torus), a shell and a
// void.
func stepParts(t *testing.T) map[string]*brep.Body {
	t.Helper()
	mk := func(b *brep.Body, err error) *brep.Body {
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	box := func(x0, y0, z0, x1, y1, z1 float64) *brep.Body {
		return mk(ops.Box(geom.V3(x0, y0, z0), geom.V3(x1, y1, z1)))
	}
	out := map[string]*brep.Body{}
	out["box"] = box(0, 0, 0, 30, 20, 10)
	out["cylinder"] = mk(ops.Cylinder(at(1, 2, 3), 5, 12))
	out["cone"] = mk(ops.Cone(at(0, 0, 0), 6, 2, 9))
	out["apex"] = mk(ops.Cone(at(0, 0, 0), 6, 0, 9))
	out["sphere"] = mk(ops.Sphere(geom.V3(1, 1, 1), 7))
	out["torus"] = mk(ops.Torus(at(0, 0, 0), 10, 3))

	plate := box(-20, -15, 0, 20, 15, 8)
	hole := boolean(t, plate, mk(ops.Cylinder(at(5, 0, -1), 4, 10)), brep.Subtract)
	out["hole"] = hole

	ball := boolean(t, box(-10, -10, -10, 10, 10, 0), mk(ops.Sphere(geom.V3(0, 0, 0), 8)), brep.Union)
	out["ball"] = ball

	rx := geom.Frame{O: geom.V3(-15, 0, 0), X: geom.YAxis, Y: geom.ZAxis, Z: geom.XAxis}
	// a slanted cut through a cylinder: ellipse edges
	tilt, err := brep.Transform(box(-50, -50, -100, 50, 50, 0), geom.Rotate(geom.Vec3{}, geom.XAxis, math.Pi/5), nil)
	if err != nil {
		t.Fatal(err)
	}
	out["slant"] = boolean(t, mk(ops.Cylinder(at(0, 0, -20), 4, 40)), tilt, brep.Subtract)

	out["steinmetz"] = boolean(t, mk(ops.Cylinder(at(0, 0, -15), 5, 30)), mk(ops.Cylinder(rx, 4, 30)), brep.Union)

	sink, err := ops.HoleTool(geom.V3(-8, 5, 8), geom.ZAxis.Neg(), geom.XAxis,
		ops.HoleSpec{Diameter: 4, Depth: 6, Tip: true, SinkDiameter: 8}, 1)
	if err != nil {
		t.Fatal(err)
	}
	out["countersink"] = boolean(t, hole, sink, brep.Subtract)

	// round a top edge (a cylinder) and the hole's rim (a torus)
	r := hole
	for k, p := range []geom.Vec3{geom.V3(0, 15, 8), geom.V3(9, 0, 8)} {
		e := nearestEdge(r, p)
		tool, op, err := ops.EdgeTool(e, 1.5, false, k, [2]bool{})
		if err != nil {
			t.Fatalf("fillet %d: %v", k, err)
		}
		r = boolean(t, r, tool, op)
	}
	out["fillets"] = r

	cup, err := ops.Shell(box(0, 0, 0, 20, 20, 12), map[*brep.Face]bool{}, 1.5)
	if err != nil {
		t.Fatal(err)
	}
	out["void"] = cup
	return out
}

func nearestEdge(b *brep.Body, p geom.Vec3) *brep.Edge {
	var best *brep.Edge
	bd := math.Inf(1)
	for _, e := range b.Edges() {
		for i := 0; i <= 32; i++ {
			if d := e.Curve.Eval(e.T0 + (e.T1-e.T0)*float64(i)/32).Dist(p); d < bd {
				best, bd = e, d
			}
		}
	}
	return best
}

func volume(b *brep.Body) float64 { return brep.Mass(b, 2e-5).Volume }

func TestSTEPRoundTrip(t *testing.T) {
	for name, b := range stepParts(t) {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := fio.WriteSTEP(&buf, []*brep.Body{b}, fio.StepOptions{Name: name}); err != nil {
				t.Fatal(err)
			}
			solids, err := fio.ReadSTEPString(buf.String())
			if err != nil {
				t.Fatal(err)
			}
			if len(solids) != 1 {
				t.Fatalf("%d solids back", len(solids))
			}
			if solids[0].Err != nil {
				t.Fatalf("read: %v", solids[0].Err)
			}
			got := solids[0].Body
			v0, e0, f0, r0, s0 := b.Counts()
			v1, e1, f1, r1, s1 := got.Counts()
			if [5]int{v0, e0, f0, r0, s0} != [5]int{v1, e1, f1, r1, s1} {
				t.Errorf("counts V E F rings shells %v, back %v", [5]int{v0, e0, f0, r0, s0}, [5]int{v1, e1, f1, r1, s1})
			}
			want, have := volume(b), volume(got)
			if math.Abs(have-want) > 1e-5*math.Abs(want) {
				t.Errorf("volume %.6f, back %.6f", want, have)
			}
		})
	}
}

// TestSTEPWithFreeCAD has OpenCASCADE (FreeCAD's Part module) read the files
// and check them: FORMA_FREECAD names FreeCAD's python.exe.
func TestSTEPWithFreeCAD(t *testing.T) {
	py := os.Getenv("FORMA_FREECAD")
	if py == "" {
		t.Skip("FORMA_FREECAD not set (FreeCAD's bin/python.exe)")
	}
	dir := t.TempDir()
	parts := stepParts(t)
	want := map[string]float64{}
	var files []string
	for name, b := range parts {
		p := filepath.Join(dir, name+".step")
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := fio.WriteSTEP(f, []*brep.Body{b}, fio.StepOptions{Name: name}); err != nil {
			t.Fatal(err)
		}
		f.Close()
		files = append(files, p)
		want[name] = volume(b)
	}
	script, _ := filepath.Abs("../tools/stepcheck/check.py")
	out, err := exec.Command(py, append([]string{script}, files...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	seen := 0
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || f[0] != "solid" {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(f[1]), ".step")
		valid := f[2] == "True"
		vol, _ := strconv.ParseFloat(f[3], 64)
		seen++
		if !valid {
			t.Errorf("%s: OpenCASCADE finds the solid invalid: %s", name, line)
		}
		if w := want[name]; math.Abs(vol-w) > 5e-4*math.Abs(w) {
			t.Errorf("%s: OpenCASCADE's volume %.5f, ours %.5f", name, vol, w)
		}
	}
	if seen != len(parts) {
		t.Errorf("%d solids checked of %d\n%s", seen, len(parts), out)
	}
	fmt.Print(string(out))
}

// TestSTEPImportOCC reads files OpenCASCADE wrote (tools/stepcheck/make.py):
// fillets with spherical corners, an oblique hole (ellipses), crossing
// cylinders (B-spline edges), a sphere and torus, a chamfered cone.
func TestSTEPImportOCC(t *testing.T) {
	list, err := os.ReadFile("testdata/volumes.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(list)), "\n") {
		f := strings.Fields(line)
		name := f[0]
		want, _ := strconv.ParseFloat(f[1], 64)
		faces, _ := strconv.Atoi(f[2])
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("testdata", name+".step"))
			if err != nil {
				t.Fatal(err)
			}
			solids, err := fio.ReadSTEPString(string(src))
			if err != nil {
				t.Fatal(err)
			}
			if len(solids) != 1 || solids[0].Err != nil {
				t.Fatalf("%d solids, %v", len(solids), solids[0].Err)
			}
			b := solids[0].Body
			if n := len(b.Faces()); n != faces {
				t.Errorf("%d faces, OpenCASCADE has %d", n, faces)
			}
			if v := volume(b); math.Abs(v-want) > 5e-4*want {
				t.Errorf("volume %.5f, OpenCASCADE's %.5f", v, want)
			}
			if name == "occ_oblique_hole" {
				n := 0
				for _, e := range b.Edges() {
					if _, ok := e.Curve.(geom.Ellipse); ok {
						n++
					}
				}
				if n == 0 {
					t.Error("the hole's rims didn't come in as ellipses")
				}
			}
		})
	}
}

// An imported body takes part in booleans like any other.
func TestSTEPImportedBoolean(t *testing.T) {
	src, err := os.ReadFile("testdata/occ_fillet_box.step")
	if err != nil {
		t.Fatal(err)
	}
	solids, err := fio.ReadSTEPString(string(src))
	if err != nil || solids[0].Err != nil {
		t.Fatal(err, solids[0].Err)
	}
	b := solids[0].Body
	cyl, err := ops.Cylinder(at(15, 10, -1), 4, 12)
	if err != nil {
		t.Fatal(err)
	}
	r := boolean(t, b, cyl, brep.Subtract)
	want := volume(b) - math.Pi*16*10
	if v := volume(r); math.Abs(v-want) > 5e-4*want {
		t.Errorf("volume %.4f, want %.4f", v, want)
	}
}

// A file in inches comes in scaled to millimetres.
func TestSTEPUnits(t *testing.T) {
	b, err := ops.Box(geom.V3(0, 0, 0), geom.V3(1, 2, 3))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := fio.WriteSTEP(&buf, []*brep.Body{b}, fio.StepOptions{Name: "inch"}); err != nil {
		t.Fatal(err)
	}
	src := strings.Replace(buf.String(), "( LENGTH_UNIT() NAMED_UNIT(*) SI_UNIT(.MILLI.,.METRE.) )",
		"( CONVERSION_BASED_UNIT('INCH',#9001) LENGTH_UNIT() NAMED_UNIT(#9002) );\n"+
			"#9001=LENGTH_MEASURE_WITH_UNIT(LENGTH_MEASURE(25.4),#9003);\n"+
			"#9002=DIMENSIONAL_EXPONENTS(1.,0.,0.,0.,0.,0.,0.);\n"+
			"#9003=( LENGTH_UNIT() NAMED_UNIT(*) SI_UNIT(.MILLI.,.METRE.) )", 1)
	solids, err := fio.ReadSTEPString(src)
	if err != nil || solids[0].Err != nil {
		t.Fatal(err, solids[0].Err)
	}
	if v := volume(solids[0].Body); math.Abs(v-6*25.4*25.4*25.4) > 1e-6*v {
		t.Errorf("volume %.3f mm³, want %.3f", v, 6*25.4*25.4*25.4)
	}
}

func TestOBJ(t *testing.T) {
	b, err := ops.Cylinder(at(0, 0, 0), 5, 10)
	if err != nil {
		t.Fatal(err)
	}
	m := brep.Tessellate(b, brep.DefaultMeshOptions(b))
	var buf bytes.Buffer
	if err := fio.WriteOBJ(&buf, []fio.OBJItem{{Name: "a cylinder", Mesh: m}, {Name: "again", Mesh: m}}); err != nil {
		t.Fatal(err)
	}
	var v, vn, f, o int
	maxIdx := 0
	for _, l := range strings.Split(buf.String(), "\n") {
		fs := strings.Fields(l)
		if len(fs) == 0 {
			continue
		}
		switch fs[0] {
		case "v":
			v++
		case "vn":
			vn++
		case "o":
			o++
		case "f":
			f++
			for _, c := range fs[1:] {
				k, _ := strconv.Atoi(strings.Split(c, "//")[0])
				maxIdx = max(maxIdx, k)
			}
		}
	}
	if o != 2 || v != 2*len(m.Pos) || vn != v || f != 2*len(m.Tris) || maxIdx != v {
		t.Errorf("o %d v %d vn %d f %d max index %d (mesh %d points %d tris)", o, v, vn, f, maxIdx, len(m.Pos), len(m.Tris))
	}
}

func centre(b *brep.Body) geom.Vec3 { return brep.Mass(b, 1e-4).Centroid }

// The assembly OpenCASCADE wrote (tools/stepcheck/make_asm.py): a plate and
// one pin placed twice, the second turned a quarter about X.
func TestSTEPImportAssembly(t *testing.T) {
	src, err := os.ReadFile("testdata/occ_assembly.step")
	if err != nil {
		t.Fatal(err)
	}
	solids, err := fio.ReadSTEPString(string(src))
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		v float64
		c geom.Vec3
	}{
		{11214.601836602551, geom.V3(20, 15, 5)},
		{2356.194490192345, geom.V3(20, 15, 10)},
		{2356.194490192345, geom.V3(60, -15, 0)},
	}
	if len(solids) != len(want) {
		t.Fatalf("%d solids, want %d", len(solids), len(want))
	}
	for i, s := range solids {
		if s.Err != nil {
			t.Fatalf("%s: %v", s.Name, s.Err)
		}
		v, c := volume(s.Body), centre(s.Body)
		if math.Abs(v-want[i].v) > 5e-4*want[i].v || c.Dist(want[i].c) > 1e-2 {
			t.Errorf("%s: volume %.3f centre %v, OpenCASCADE has %.3f at %v", s.Name, v, c, want[i].v, want[i].c)
		}
	}
	if solids[1].Name != "PinBody (1)" {
		t.Errorf("names: %q %q %q", solids[0].Name, solids[1].Name, solids[2].Name)
	}
}

// An assembly written and read back: every instance where it was placed.
func TestSTEPAssemblyRoundTrip(t *testing.T) {
	asm := stepAssembly(t)
	var buf bytes.Buffer
	if err := fio.WriteSTEPAssembly(&buf, asm, fio.StepOptions{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "NEXT_ASSEMBLY_USAGE_OCCURRENCE") {
		t.Fatal("no assembly structure written")
	}
	solids, err := fio.ReadSTEPString(buf.String())
	if err != nil {
		t.Fatal(err)
	}
	want := assemblyCentres(t, asm)
	if len(solids) != len(want) {
		t.Fatalf("%d solids back, want %d", len(solids), len(want))
	}
	for i, s := range solids {
		if s.Err != nil {
			t.Fatal(s.Err)
		}
		if c := centre(s.Body); c.Dist(want[i]) > 1e-3 {
			t.Errorf("solid %d (%s) at %v, placed at %v", i, s.Name, c, want[i])
		}
	}
	if py := os.Getenv("FORMA_FREECAD"); py != "" {
		p := filepath.Join(t.TempDir(), "asm.step")
		os.WriteFile(p, buf.Bytes(), 0o644)
		script, _ := filepath.Abs("../tools/stepcheck/check.py")
		out, err := exec.Command(py, script, p).CombinedOutput()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		n := strings.Count(string(out), "solid ")
		if n != len(want) || strings.Contains(string(out), " False ") {
			t.Errorf("OpenCASCADE read:\n%s", out)
		}
		for _, line := range strings.Split(string(out), "\n") {
			f := strings.Fields(line)
			if len(f) < 10 || f[0] != "solid" {
				continue
			}
			var c geom.Vec3
			c.X, _ = strconv.ParseFloat(f[7], 64)
			c.Y, _ = strconv.ParseFloat(f[8], 64)
			c.Z, _ = strconv.ParseFloat(f[9], 64)
			near := math.Inf(1)
			for _, w := range want {
				near = math.Min(near, w.Dist(c))
			}
			if near > 1e-3 {
				t.Errorf("OpenCASCADE puts a solid at %v, where none was placed", c)
			}
		}
		fmt.Print(string(out))
	}
}

// stepAssembly: a plate of the design's own, a pin part placed three times.
func stepAssembly(t *testing.T) fio.StepAssembly {
	plate, err := ops.Box(geom.V3(0, 0, 0), geom.V3(40, 30, 10))
	if err != nil {
		t.Fatal(err)
	}
	pin, err := ops.Cylinder(at(0, 0, 0), 5, 30)
	if err != nil {
		t.Fatal(err)
	}
	return fio.StepAssembly{Name: "asm", Bodies: []*brep.Body{plate},
		Parts: []fio.StepPart{{Name: "pin", Bodies: []*brep.Body{pin}}},
		Instances: []fio.StepInstance{
			{Part: 0, Name: "pin 1", Place: geom.Translate(geom.V3(10, 10, 10))},
			{Part: 0, Name: "pin 2", Place: geom.Rotate(geom.Vec3{}, geom.XAxis, math.Pi/2).Then(geom.Translate(geom.V3(60, 0, 0)))},
			{Part: 0, Name: "pin 3", Place: geom.Rotate(geom.V3(0, 0, 15), geom.YAxis, 0.7).Then(geom.Translate(geom.V3(-30, 5, 2)))},
		}}
}

func assemblyCentres(t *testing.T, a fio.StepAssembly) []geom.Vec3 {
	var out []geom.Vec3
	for _, b := range a.Bodies {
		out = append(out, centre(b))
	}
	for _, in := range a.Instances {
		for _, b := range a.Parts[in.Part].Bodies {
			out = append(out, in.Place.Pt(centre(b)))
		}
	}
	return out
}
