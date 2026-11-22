package drawing

import (
	"math"
	"testing"

	"forma/brep"
	"forma/geom"
	"forma/ops"
)

// plate is 60 × 40 × 10 with a Ø16 hole through it at (20, 20).
func plate(t *testing.T) *brep.Body {
	t.Helper()
	b, err := ops.Box(geom.V3(0, 0, 0), geom.V3(60, 40, 10))
	if err != nil {
		t.Fatal(err)
	}
	c, err := ops.Cylinder(geom.FrameZ(geom.V3(20, 20, -5), geom.ZAxis, geom.XAxis), 8, 20)
	if err != nil {
		t.Fatal(err)
	}
	out, err := brep.Boolean(b, c, brep.Subtract)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

var (
	front = FrameLooking(geom.YAxis, geom.ZAxis)
	top   = FrameLooking(geom.ZAxis.Neg(), geom.YAxis)
)

func lenOf(cs []Curve, keep func(Curve) bool) float64 {
	l := 0.0
	for _, c := range cs {
		if keep(c) {
			l += polyLen2(c.Pts)
		}
	}
	return l
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestFrames(t *testing.T) {
	if front.R.Dist(geom.XAxis) > 1e-12 || front.U.Dist(geom.ZAxis) > 1e-12 {
		t.Fatalf("front %+v", front)
	}
	if top.R.Dist(geom.XAxis) > 1e-12 || top.U.Dist(geom.YAxis) > 1e-12 {
		t.Fatalf("top %+v", top)
	}
}

func TestPlateFront(t *testing.T) {
	pr := Project([]Body{{Key: 1, Body: plate(t)}}, front, Options{Hidden: true})
	vis := lenOf(pr.Curves, func(c Curve) bool { return !c.Hidden })
	hid := lenOf(pr.Curves, func(c Curve) bool { return c.Hidden })
	// the outline 2 × (60 + 10); the hole's two sides hidden, 2 × 10
	if !near(vis, 140, 0.5) || !near(hid, 20, 0.5) {
		t.Fatalf("front: visible %.2f (want 140), hidden %.2f (want 20)", vis, hid)
	}
	if pr.Box[0].Dist(geom.V2(0, 0)) > 1e-6 || pr.Box[1].Dist(geom.V2(60, 10)) > 1e-6 {
		t.Fatalf("box %v", pr.Box)
	}
}

func TestPlateTop(t *testing.T) {
	pr := Project([]Body{{Key: 1, Body: plate(t)}}, top, Options{Hidden: true})
	vis := lenOf(pr.Curves, func(c Curve) bool { return !c.Hidden })
	hid := lenOf(pr.Curves, func(c Curve) bool { return c.Hidden })
	// the outline 200 and the hole's rim 16π; the lower rim lies under it
	want := 200 + 16*math.Pi
	if !near(vis, want, 0.6) || hid > 0.5 {
		t.Fatalf("top: visible %.2f (want %.2f), hidden %.2f (want 0)", vis, want, hid)
	}
	arcs := 0
	for _, c := range pr.Curves {
		if c.Arc != nil && !c.Hidden {
			arcs++
			if c.Arc.C.Dist(geom.V2(20, 20)) > 1e-9 || !near(c.Arc.R, 8, 1e-9) {
				t.Fatalf("arc %+v", c.Arc)
			}
		}
	}
	if arcs == 0 {
		t.Fatal("the hole's rim isn't known as a circle")
	}
}

func TestCylinderSilhouette(t *testing.T) {
	c, err := ops.Cylinder(geom.FrameZ(geom.V3(0, 0, 0), geom.ZAxis, geom.XAxis), 10, 30)
	if err != nil {
		t.Fatal(err)
	}
	pr := Project([]Body{{Key: 1, Body: c}}, front, Options{Hidden: true})
	sil := lenOf(pr.Curves, func(c Curve) bool { return c.Edge == nil && !c.Hidden })
	vis := lenOf(pr.Curves, func(c Curve) bool { return !c.Hidden })
	hid := lenOf(pr.Curves, func(c Curve) bool { return c.Hidden })
	// side on: two silhouettes 30 long, the ends as lines 20 long
	if !near(sil, 60, 0.5) || !near(vis, 100, 0.8) || hid > 0.5 {
		t.Fatalf("cylinder: silhouettes %.2f (60), visible %.2f (100), hidden %.2f (0)", sil, vis, hid)
	}
	for _, cv := range pr.Curves {
		if cv.Edge == nil {
			for _, p := range cv.Pts {
				if !near(math.Abs(p.X), 10, 0.02) {
					t.Fatalf("silhouette off the side: %v", p)
				}
			}
		}
	}
	// end on: just the rim
	pt := Project([]Body{{Key: 1, Body: c}}, top, Options{Hidden: true})
	if l := lenOf(pt.Curves, func(c Curve) bool { return !c.Hidden }); !near(l, 20*math.Pi, 0.3) {
		t.Fatalf("end on: %.2f, want %.2f", l, 20*math.Pi)
	}
}

func TestSectionHatched(t *testing.T) {
	// cut the plate through the hole's middle, looking along +Y
	pr := Section([]Body{{Key: 1, Body: plate(t)}}, geom.V3(0, 20, 0), geom.YAxis, front, Options{})
	if pr.Warn != "" {
		t.Fatal(pr.Warn)
	}
	if len(pr.Regions) != 2 {
		t.Fatalf("%d cut regions, want 2 (either side of the hole)", len(pr.Regions))
	}
	area := 0.0
	for _, r := range pr.Regions {
		for _, l := range r.Loops {
			area += math.Abs(polyArea(l))
		}
	}
	// 60 × 10 less the hole's 16 × 10
	if !near(area, 440, 0.5) {
		t.Fatalf("cut area %.2f, want 440", area)
	}
	var lines float64
	for _, r := range pr.Regions {
		for _, s := range Hatch(r.Loops, math.Pi/4, 2) {
			lines += s[0].Dist(s[1])
		}
	}
	// hatch length ≈ area / spacing
	if !near(lines, 220, 12) {
		t.Fatalf("hatch length %.1f, want about 220", lines)
	}
}

func polyArea(l []geom.Vec2) float64 {
	a := 0.0
	for i := range l {
		a += l[i].Cross(l[(i+1)%len(l)])
	}
	return a / 2
}

// A boss's outline seen side on is in plain sight: the silhouette, found
// on the facets a hair inside the true outline, mustn't hide behind its
// own face's nearer facets.
func TestBossOutlineVisible(t *testing.T) {
	b := bracket(t)
	pr := Project([]Body{{Key: 1, Body: b}}, front, Options{Hidden: true})
	hid := lenOf(pr.Curves, func(c Curve) bool { return c.Hidden })
	sil := lenOf(pr.Curves, func(c Curve) bool { return c.Edge == nil && !c.Hidden })
	if !near(hid, 20, 0.5) || !near(sil, 30, 0.5) {
		t.Fatalf("hidden %.2f (want the hole's 20), boss outline %.2f (want 2 × 15)", hid, sil)
	}
}
