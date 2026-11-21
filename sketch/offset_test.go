package sketch

import (
	"math"
	"testing"

	"forma/geom"
)

func square(s float64) []Seg {
	p := []geom.Vec2{{}, {X: s}, {X: s, Y: s}, {Y: s}}
	var l []Seg
	for i := range p {
		l = append(l, Seg{A: p[i], B: p[(i+1)%4]})
	}
	return l
}

// A square grown by c gains a band round each side and a quarter circle at
// each corner; shrunk, it is a smaller square.
func TestOffsetSquare(t *testing.T) {
	sq := square(10)
	grown := OffsetLoop(sq, -1) // right of a counter-clockwise loop: out
	if a, want := LoopArea(grown), 100+40+math.Pi; math.Abs(a-want) > 1e-3 {
		t.Fatalf("grown square: area %.5f, want %.5f", a, want)
	}
	// a circle offset by an arc of another radius
	c := Seg{Arc: true, A: geom.V2(5, 0), B: geom.V2(5, 0), C: geom.V2(0, 0), R: 5, CCW: true}
	if o := OffsetSeg(c, 1); len(o) != 1 || math.Abs(o[0].R-4) > 1e-12 {
		t.Fatalf("circle offset %v", o)
	}
	// a Bézier quarter circle moved out stays within tolerance of a circle
	q := arcBeziers(geom.V2(0, 0), geom.V2(10, 0), geom.V2(0, 10), true, 0)
	for _, s := range OffsetSeg(q[0], -2) {
		for i := 0; i <= 16; i++ {
			p := bezAt(s.Ctrl, float64(i)/16)
			if d := math.Abs(bezAt(q[0].Ctrl, bezProject(q[0].Ctrl, p)).Dist(p) - 2); d > offsetTol {
				t.Fatalf("offset arc strays %.2g", d)
			}
		}
	}
}

// Text grown by a clearance: every letter's area grows by about its
// outline's length times the clearance, counters shrink, and the letters
// come out as one region each.
func TestOffsetText(t *testing.T) {
	FontDirs = append(FontDirs, "../assets/fonts")
	if _, err := loadFont(DefaultFont); err != nil {
		t.Skip("no font:", err)
	}
	area := func(grow float64) (float64, int, float64) {
		s := New("t", geom.World)
		a := s.AddPoint(geom.V2(0, 0)).ID
		e := s.AddText(a, "Luna", DefaultFont, 10)
		e.Grow = grow
		var sum float64
		n := 0
		for _, r := range s.Regions() {
			if r.Ink {
				sum += math.Abs(r.Area)
				n++
			}
		}
		per := 0.0
		e.Grow = 0
		for _, l := range s.TextContours(e) {
			p := SegPoints2(l, 1e-4)
			for i := range p {
				per += p[i].Dist(p[(i+1)%len(p)])
			}
		}
		return sum, n, per
	}
	a0, n0, per := area(0)
	a1, n1, _ := area(0.2)
	if n1 != n0 {
		t.Fatalf("%d letters grown, %d before", n1, n0)
	}
	want := a0 + per*0.2
	if math.Abs(a1-want) > 0.03*per*0.2 {
		t.Fatalf("grown area %.3f, want about %.3f (was %.3f, outline %.2f)", a1, want, a0, per)
	}
	a2, _, _ := area(-0.2)
	// sharp corners give back c² tan(θ/2) each, so a little more than that
	if want := a0 - per*0.2; a2 < want-0.01 || a2 > want+0.15*per*0.2 {
		t.Fatalf("shrunk area %.3f, want about %.3f", a2, want)
	}
}
