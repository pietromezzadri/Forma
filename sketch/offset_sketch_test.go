package sketch

import (
	"math"
	"testing"

	"forma/geom"
)

// rectLines draws a w × h rectangle at the origin as four lines sharing points.
func rectLines(s *Sketch, w, h float64) []int {
	var pts []int
	for _, p := range []geom.Vec2{{}, {X: w}, {X: w, Y: h}, {Y: h}} {
		pts = append(pts, s.AddPoint(p).ID)
	}
	var ls []int
	for i := range pts {
		ls = append(ls, s.AddLine(pts[i], pts[(i+1)%4]).ID)
	}
	return ls
}

// regionAreas are the areas of the sketch's regions (outer less holes).
func regionAreas(s *Sketch) []float64 {
	var out []float64
	for _, r := range s.Regions() {
		a := math.Abs(LoopArea(r.Outer))
		for _, h := range r.Holes {
			a -= math.Abs(LoopArea(h))
		}
		out = append(out, a)
	}
	return out
}

func hasArea(as []float64, want float64) bool {
	for _, a := range as {
		if math.Abs(a-want) < 1e-6*math.Max(1, want) {
			return true
		}
	}
	return false
}

// A rectangle offset out is a bigger rectangle with sharp corners, and the
// copy follows the rectangle and the distance.
func TestSketchOffsetRect(t *testing.T) {
	s := New("s", geom.World)
	ls := rectLines(s, 40, 20)
	for i, l := range ls {
		k := Horizontal
		if i%2 == 1 {
			k = Vertical
		}
		if _, err := s.AddConstraint(k, []int{l}, 0); err != nil {
			t.Fatal(err)
		}
	}
	c, err := s.AddOffset(ls, 3)
	if err != nil {
		t.Fatal(err)
	}
	s.Solve()
	if n := len(s.offsetEnts(c.ID)); n != 4 {
		t.Fatalf("%d curves in the copy, want 4 lines", n)
	}
	// the ring between them
	if as := regionAreas(s); !hasArea(as, 46*26-40*20) || !hasArea(as, 40*20) {
		t.Fatalf("regions %v", as)
	}
	// the rectangle made wider: the copy follows
	w, err := s.AddConstraint(HDistance, []int{s.Ent(ls[0]).Pts[0], s.Ent(ls[0]).Pts[1]}, 40)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetValue(w.ID, 50, ""); err != nil {
		t.Fatal(err)
	}
	if as := regionAreas(s); !hasArea(as, 56*26-50*20) {
		t.Fatalf("after widening: regions %v", as)
	}
	// inward
	if err := s.SetValue(c.ID, -2, ""); err != nil {
		t.Fatal(err)
	}
	if as := regionAreas(s); !hasArea(as, 46*16) {
		t.Fatalf("inward: regions %v", as)
	}
	// deleting a curve of the copy deletes the offset
	s.Delete(s.offsetEnts(c.ID)[0].ID)
	if s.Con(c.ID) != nil || len(s.offsetEnts(c.ID)) != 0 {
		t.Fatal("the offset outlived its copy")
	}
	if len(s.Curves()) != 4 {
		t.Fatalf("%d curves left", len(s.Curves()))
	}
}

// A slot (two lines, two half circles) offset out keeps its shape; offset
// in by more than its radius its ends close to points the lines meet at.
func TestSketchOffsetSlot(t *testing.T) {
	s := New("s", geom.World)
	r := 5.0
	p := func(x, y float64) int { return s.AddPoint(geom.V2(x, y)).ID }
	a, b, cc, d := p(0, -r), p(30, -r), p(30, r), p(0, r)
	c1, c2 := p(30, 0), p(0, 0)
	ids := []int{
		s.AddLine(a, b).ID,
		s.AddArc(c1, b, cc).ID,
		s.AddLine(cc, d).ID,
		s.AddArc(c2, d, a).ID,
	}
	s.Solve()
	c, err := s.AddOffset(ids, 2)
	if err != nil {
		t.Fatal(err)
	}
	s.Solve()
	slot := func(r float64) float64 { return 30*2*r + math.Pi*r*r }
	if as := regionAreas(s); !hasArea(as, slot(7)-slot(5)) {
		t.Fatalf("out: regions %v, want a ring of %.3f", as, slot(7)-slot(5))
	}
	if err := s.SetValue(c.ID, -3, ""); err != nil {
		t.Fatal(err)
	}
	if as := regionAreas(s); !hasArea(as, slot(2)) {
		t.Fatalf("in by 3: regions %v, want %.3f", as, slot(2))
	}
}

// An open L offset to its left; a circle offset is a concentric circle.
func TestSketchOffsetChainAndCircle(t *testing.T) {
	s := New("s", geom.World)
	a, b, c := s.AddPoint(geom.V2(0, 0)).ID, s.AddPoint(geom.V2(20, 0)).ID, s.AddPoint(geom.V2(20, 10)).ID
	l1, l2 := s.AddLine(a, b).ID, s.AddLine(b, c).ID
	o, err := s.AddOffset([]int{l2, l1}, 2) // picked in any order
	if err != nil {
		t.Fatal(err)
	}
	s.Solve()
	es := s.offsetEnts(o.ID)
	if len(es) != 2 {
		t.Fatalf("%d curves", len(es))
	}
	// run from the chain's first curve: which end that is depends on the
	// order found; either way the corner is mitred at (18, 2) or (22, −2)
	corner := s.P(es[0].Pts[1])
	if corner.Dist(geom.V2(18, 2)) > 1e-9 && corner.Dist(geom.V2(22, -2)) > 1e-9 {
		t.Fatalf("corner at %v", corner)
	}
	ce := s.AddPoint(geom.V2(50, 0))
	ci := s.AddCircle(ce.ID, 6)
	oc, err := s.AddOffset([]int{ci.ID}, 1.5)
	if err != nil {
		t.Fatal(err)
	}
	s.Solve()
	cs := s.offsetEnts(oc.ID)
	if len(cs) != 1 || math.Abs(s.Radius(cs[0])-7.5) > 1e-9 {
		t.Fatalf("circle copy %v", cs)
	}
	// the source circle resized: the copy follows
	rc, err := s.AddConstraint(Radius, []int{ci.ID}, 6)
	if err != nil {
		t.Fatal(err)
	}
	s.SetValue(rc.ID, 9, "")
	if math.Abs(s.Radius(cs[0])-10.5) > 1e-9 {
		t.Fatalf("copy radius %.3f after the circle went to 9", s.Radius(cs[0]))
	}
	// deleting the source lets the copy go free
	s.Delete(ci.ID)
	s.Solve()
	if s.Con(oc.ID) != nil || cs[0].Off != 0 {
		t.Fatal("the copy is still driven")
	}
}

// A spline chain's offset stays its distance from it.
func TestSketchOffsetSpline(t *testing.T) {
	s := New("s", geom.World)
	p := func(x, y float64) int { return s.AddPoint(geom.V2(x, y)).ID }
	sp := s.AddSpline(p(0, 0), p(10, 15), p(25, -10), p(40, 5))
	c, err := s.AddOffset([]int{sp.ID}, 2)
	if err != nil {
		t.Fatal(err)
	}
	s.Solve()
	ctrl := s.Ctrl(sp)
	for _, e := range s.offsetEnts(c.ID) {
		for i := 0; i <= 10; i++ {
			q := bezAt(s.Ctrl(e), float64(i)/10)
			if d := bezAt(ctrl, bezProject(ctrl, q)).Dist(q); math.Abs(d-2) > 5e-3 {
				t.Fatalf("copy %.4f from the spline", d)
			}
		}
	}
}

// A circle inside a rectangle inside a bigger one is a hole of the inner
// rectangle, whichever outline the region finder meets first.
func TestRegionsNestedHoles(t *testing.T) {
	s := New("s", geom.World)
	for _, h := range []float64{19, 15} {
		w := h + 15
		var ids []int
		for _, p := range []geom.Vec2{{X: -w, Y: -h}, {X: w, Y: -h}, {X: w, Y: h}, {X: -w, Y: h}} {
			ids = append(ids, s.AddPoint(p).ID)
		}
		for i := range ids {
			s.AddLine(ids[i], ids[(i+1)%4])
		}
	}
	s.AddCircle(s.AddPoint(geom.V2(0, 0)).ID, 5)
	as := regionAreas(s)
	ring, inner, disc := 68.0*38-60*30, 60.0*30-math.Pi*25, math.Pi*25
	if len(as) != 3 || !hasArea(as, ring) || !hasArea(as, inner) || !hasArea(as, disc) {
		t.Fatalf("regions %v, want %.2f, %.2f and %.2f", as, ring, inner, disc)
	}
}
