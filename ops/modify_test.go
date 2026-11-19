package ops_test

import (
	"math"
	"testing"

	"forma/brep"
	"forma/geom"
	"forma/ops"
)

func mvol(t *testing.T, b *brep.Body) float64 {
	t.Helper()
	if err := brep.Check(b); err != nil {
		t.Fatalf("invalid: %v", err)
	}
	opt := brep.DefaultMeshOptions(b)
	opt.ChordTol = b.Bounds().Diagonal() * 2e-5
	m := brep.Tessellate(b, opt)
	if err := m.Watertight(1e-5); err != nil {
		t.Fatalf("mesh: %v", err)
	}
	return m.Volume()
}

func mbox(t *testing.T, x0, y0, z0, x1, y1, z1 float64) *brep.Body {
	b, err := ops.Box(geom.V3(x0, y0, z0), geom.V3(x1, y1, z1))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mcyl(t *testing.T, x, y, z, r, h float64) *brep.Body {
	f := geom.World
	f.O = geom.V3(x, y, z)
	b, err := ops.Cylinder(f, r, h)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// edgeAt is the edge whose middle is nearest p.
func edgeAt(b *brep.Body, p geom.Vec3) *brep.Edge {
	var best *brep.Edge
	bd := math.Inf(1)
	for _, e := range b.Edges() {
		var d float64
		if e.Verts[0] == e.Verts[1] {
			// a closed edge: its nearest sample
			d = math.Inf(1)
			for i := 0; i < 64; i++ {
				q := e.Curve.Eval(e.T0 + (e.T1-e.T0)*float64(i)/64)
				d = math.Min(d, q.Dist(p))
			}
		} else {
			d = e.Curve.Eval((e.T0 + e.T1) / 2).Dist(p)
		}
		if d < bd {
			best, bd = e, d
		}
	}
	return best
}

func round(t *testing.T, b *brep.Body, at geom.Vec3, r float64, chamfer bool) *brep.Body {
	t.Helper()
	e := edgeAt(b, at)
	tool, op, err := ops.EdgeTool(e, r, chamfer, 1, [2]bool{true, true})
	if err != nil {
		t.Fatal(err)
	}
	out, err := brep.Boolean(b, tool, op)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// spandrel: the area between a corner and a quarter circle of radius r,
// and its centroid's distance from the corner along each side.
func spandrel(r float64) (area, cx float64) {
	return r * r * (1 - math.Pi/4), r * (10 - 3*math.Pi) / (3 * (4 - math.Pi))
}

func near(t *testing.T, what string, got, want, rel float64) {
	t.Helper()
	if math.Abs(got-want) > rel*math.Abs(want) {
		t.Errorf("%s: %g, want %g", what, got, want)
	}
}

func TestFilletChamferEdges(t *testing.T) {
	b := mbox(t, 0, 0, 0, 40, 30, 20)
	a, _ := spandrel(4)
	near(t, "box edge fillet", mvol(t, round(t, b, geom.V3(20, 0, 20), 4, false)), 24000-a*40, 1e-4)
	near(t, "box edge chamfer", mvol(t, round(t, b, geom.V3(20, 0, 20), 3, true)), 24000-4.5*40, 1e-6)
	// two edges meeting at a corner
	r1 := round(t, b, geom.V3(20, 0, 20), 4, false)
	r2 := round(t, r1, geom.V3(40, 15, 20), 4, false)
	if v := mvol(t, r2); v > 24000-a*40-a*30+a*4.5 || v < 24000-a*70 {
		t.Errorf("two fillets: %g", v)
	}
	// an inside corner: two boxes in an L, its inner edge filled
	l, err := brep.Boolean(mbox(t, 0, 0, 0, 40, 30, 10), mbox(t, 0, 0, 10, 10, 30, 40), brep.Union)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "inside fillet", mvol(t, round(t, l, geom.V3(10, 15, 10), 5, false)), 40*30*10+10*30*30+spandrelA(5)*30, 1e-4)
	near(t, "inside chamfer", mvol(t, round(t, l, geom.V3(10, 15, 10), 5, true)), 40*30*10+10*30*30+12.5*30, 1e-6)
}

func spandrelA(r float64) float64 { a, _ := spandrel(r); return a }

func TestFilletCircles(t *testing.T) {
	// the top rim of a cylinder: a ring of spandrel turned about the axis
	c := mcyl(t, 0, 0, 0, 10, 20)
	a, cx := spandrel(3)
	want := math.Pi*100*20 - 2*math.Pi*(10-cx)*a
	near(t, "cylinder rim", mvol(t, round(t, c, geom.V3(10, 0, 20), 3, false)), want, 2e-4)
	near(t, "cylinder chamfer", mvol(t, round(t, c, geom.V3(10, 0, 20), 2, true)),
		math.Pi*100*20-2*math.Pi*(10-2.0/3)*2, 2e-4)
	// where a boss meets a plate (the boolean's polyline edge): filled
	plate := mbox(t, -30, -30, 0, 30, 30, 10)
	boss, err := brep.Boolean(plate, mcyl(t, 0, 0, 10, 8, 15), brep.Union)
	if err != nil {
		t.Fatal(err)
	}
	v0 := mvol(t, boss)
	a, cx = spandrel(2)
	near(t, "boss root", mvol(t, round(t, boss, geom.V3(8, 0, 10), 2, false)), v0+2*math.Pi*(8+cx)*a, 3e-4)
	// the rim of a hole through the plate: cut away
	holed, err := brep.Boolean(plate, mcyl(t, 0, 0, -5, 6, 20), brep.Subtract)
	if err != nil {
		t.Fatal(err)
	}
	v0 = mvol(t, holed)
	near(t, "hole rim", mvol(t, round(t, holed, geom.V3(6, 0, 10), 2, false)), v0-2*math.Pi*(6+cx)*a, 3e-4)
}

func TestHoles(t *testing.T) {
	plate := mbox(t, -20, -20, 0, 20, 20, 10)
	down := geom.V3(0, 0, -1)
	cut := func(h ops.HoleSpec) float64 {
		tool, err := ops.HoleTool(geom.V3(0, 0, 10), down, geom.XAxis, h, 1)
		if err != nil {
			t.Fatal(err)
		}
		r, err := brep.Boolean(plate, tool, brep.Subtract)
		if err != nil {
			t.Fatal(err)
		}
		return mvol(t, r)
	}
	near(t, "through", cut(ops.HoleSpec{Diameter: 6, Depth: 20}), 16000-math.Pi*9*10, 2e-4)
	near(t, "blind", cut(ops.HoleSpec{Diameter: 6, Depth: 5}), 16000-math.Pi*9*5, 2e-4)
	tip := 3 / math.Tan(59*math.Pi/180)
	near(t, "blind with a point", cut(ops.HoleSpec{Diameter: 6, Depth: 5, Tip: true}), 16000-math.Pi*9*5-math.Pi*9*tip/3, 2e-4)
	near(t, "counterbore", cut(ops.HoleSpec{Diameter: 6, Depth: 20, BoreDiameter: 10, BoreDepth: 4}),
		16000-math.Pi*9*6-math.Pi*25*4, 2e-4)
	// countersink: a frustum from radius 5 down to 3 over 2 mm
	fr := math.Pi * 2 / 3 * (25 + 15 + 9)
	near(t, "countersink", cut(ops.HoleSpec{Diameter: 6, Depth: 20, SinkDiameter: 10}), 16000-math.Pi*9*8-fr, 2e-4)
}

func TestShell(t *testing.T) {
	b := mbox(t, 0, 0, 0, 40, 30, 20)
	var top *brep.Face
	for _, f := range b.Faces() {
		if f.Name == "top" {
			top = f
		}
	}
	s, err := ops.Shell(b, map[*brep.Face]bool{top: true}, 2)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "open box", mvol(t, s), 24000-36*26*18, 1e-6)
	// closed: a hollow (two shells)
	s, err = ops.Shell(b, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "hollow box", mvol(t, s), 24000-36*26*16, 1e-6)
	if len(s.Shells) != 2 {
		t.Errorf("hollow box: %d shells", len(s.Shells))
	}
	// a cup from a cylinder
	c := mcyl(t, 0, 0, 0, 10, 20)
	for _, f := range c.Faces() {
		if f.Name == "top" {
			top = f
		}
	}
	s, err = ops.Shell(c, map[*brep.Face]bool{top: true}, 1.5)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "cup", mvol(t, s), math.Pi*(100*20-8.5*8.5*18.5), 3e-4)
	// a plate with a boss, shelled from below
	plate := mbox(t, -30, -30, 0, 30, 30, 10)
	boss, _ := brep.Boolean(plate, mcyl(t, 0, 0, 10, 8, 15), brep.Union)
	var bottom *brep.Face
	for _, f := range boss.Faces() {
		if f.Name == "bottom" && f.Surf.(geom.Plane).F.O.Z < 1 {
			bottom = f
		}
	}
	s, err = ops.Shell(boss, map[*brep.Face]bool{bottom: true}, 1)
	if err != nil {
		t.Fatal(err)
	}
	inner := 58*58*9 + math.Pi*49*15
	near(t, "shelled boss", mvol(t, s), mvol(t, boss)-inner, 3e-4)
}

// A lug standing against a boss: once the faces move in, the boss's base
// arcs end at new angles.
func TestShellArcEnds(t *testing.T) {
	plate := mbox(t, -30, -30, 0, 30, 30, 10)
	b, err := brep.Boolean(plate, mcyl(t, 0, 0, 10, 10, 12), brep.Union)
	if err != nil {
		t.Fatal(err)
	}
	if b, err = brep.Boolean(b, mbox(t, 8, -2, 10, 14, 2, 18), brep.Union); err != nil {
		t.Fatal(err)
	}
	var bottom *brep.Face
	for _, f := range b.Faces() {
		if p, ok := f.Surf.(geom.Plane); ok && f.Name == "bottom" && p.F.O.Z < 1 {
			bottom = f
		}
	}
	s, err := ops.Shell(b, map[*brep.Face]bool{bottom: true}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if v := mvol(t, s); v <= 0 || v >= mvol(t, b) {
		t.Fatalf("shell volume %g", v)
	}
}
