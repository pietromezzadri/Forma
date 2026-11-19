package sketch

import (
	"math"
	"testing"

	"forma/geom"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// A rectangle made fully constrained step by step: DOF falls to 0.
func TestRectangleDOF(t *testing.T) {
	s := New("s", geom.World)
	p := []*Entity{
		s.AddPoint(geom.V2(0, 0)), s.AddPoint(geom.V2(9, 0.5)),
		s.AddPoint(geom.V2(9.5, 6)), s.AddPoint(geom.V2(0.3, 5)),
	}
	var l []*Entity
	for i := range p {
		l = append(l, s.AddLine(p[i].ID, p[(i+1)%4].ID))
	}
	s.Solve()
	if s.Status.DOF != 8 {
		t.Fatalf("free quad DOF %d, want 8", s.Status.DOF)
	}
	must := func(k ConKind, ids []int, v float64) {
		t.Helper()
		if _, err := s.AddConstraint(k, ids, v); err != nil {
			t.Fatalf("%s: %v", k, err)
		}
	}
	must(Horizontal, []int{l[0].ID}, 0)
	must(Horizontal, []int{l[2].ID}, 0)
	must(Vertical, []int{l[1].ID}, 0)
	must(Vertical, []int{l[3].ID}, 0)
	if s.Status.DOF != 4 {
		t.Fatalf("rectangle DOF %d, want 4", s.Status.DOF)
	}
	must(Fix, []int{p[0].ID}, 0)
	must(Distance, []int{p[0].ID, p[1].ID}, 40)
	must(Distance, []int{p[1].ID, p[2].ID}, 25)
	if s.Status.DOF != 0 {
		t.Fatalf("DOF %d, want 0", s.Status.DOF)
	}
	for _, e := range s.Ents {
		if !s.Status.Fixed[e.ID] {
			t.Errorf("%s not fixed", s.Describe(e.ID))
		}
	}
	c := s.P(p[2].ID).Sub(s.P(p[0].ID))
	if !near(c.X, 40) || !near(c.Y, 25) {
		t.Fatalf("corner at %v", c)
	}
	// a fifth constraint on a fully constrained sketch is refused
	if _, err := s.AddConstraint(Horizontal, []int{l[0].ID}, 0); err != ErrRedundant {
		t.Fatalf("duplicate horizontal: %v", err)
	}
	if _, err := s.AddConstraint(Distance, []int{p[0].ID, p[2].ID}, 10); err != ErrConflict {
		t.Fatalf("impossible diagonal: %v", err)
	}
	// the sketch is unchanged after the refusals
	if c := s.P(p[2].ID).Sub(s.P(p[0].ID)); !near(c.X, 40) || !near(c.Y, 25) {
		t.Fatalf("refused constraint moved the corner to %v", c)
	}
	// changing a dimension moves it
	for _, cn := range s.Cons {
		if cn.Kind == Distance && cn.Value == 40 {
			if err := s.SetValue(cn.ID, 60, "60"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if c := s.P(p[2].ID).Sub(s.P(p[0].ID)); !near(c.X, 60) || !near(c.Y, 25) {
		t.Fatalf("after edit corner at %v", c)
	}
}

// A triangle from three lengths.
func TestTriangleFromLengths(t *testing.T) {
	s := New("s", geom.World)
	a := s.AddPoint(geom.V2(0, 0))
	b := s.AddPoint(geom.V2(10, 1))
	c := s.AddPoint(geom.V2(4, 7))
	s.AddLine(a.ID, b.ID)
	s.AddLine(b.ID, c.ID)
	s.AddLine(c.ID, a.ID)
	for _, d := range []struct {
		p, q int
		v    float64
	}{{a.ID, b.ID, 30}, {b.ID, c.ID, 40}, {c.ID, a.ID, 50}} {
		if _, err := s.AddConstraint(Distance, []int{d.p, d.q}, d.v); err != nil {
			t.Fatal(err)
		}
	}
	if !near(s.P(a.ID).Dist(s.P(b.ID)), 30) || !near(s.P(b.ID).Dist(s.P(c.ID)), 40) || !near(s.P(c.ID).Dist(s.P(a.ID)), 50) {
		t.Fatal("lengths not met")
	}
	// 3-4-5: right angle at b
	u, v := s.P(a.ID).Sub(s.P(b.ID)), s.P(c.ID).Sub(s.P(b.ID))
	if math.Abs(u.Dot(v)) > 1e-5 {
		t.Fatalf("angle at b not right: dot %g", u.Dot(v))
	}
	if s.Status.DOF != 3 { // a rigid triangle can still move and turn
		t.Fatalf("DOF %d, want 3", s.Status.DOF)
	}
}

// A slot: two lines tangent to two arcs, radii equal.
func TestTangentArcs(t *testing.T) {
	s := New("s", geom.World)
	c1 := s.AddPoint(geom.V2(0, 0))
	c2 := s.AddPoint(geom.V2(30, 0))
	// arcs: left one from top to bottom, right one from bottom to top
	a1s, a1e := s.AddPoint(geom.V2(0, 5)), s.AddPoint(geom.V2(0, -5))
	a2s, a2e := s.AddPoint(geom.V2(30, -5)), s.AddPoint(geom.V2(30, 5))
	arc1 := s.AddArc(c1.ID, a1s.ID, a1e.ID)
	arc2 := s.AddArc(c2.ID, a2s.ID, a2e.ID)
	top := s.AddLine(a2e.ID, a1s.ID)
	bot := s.AddLine(a1e.ID, a2s.ID)
	for _, k := range []struct {
		k   ConKind
		ids []int
		v   float64
	}{
		{Tangent, []int{top.ID, arc1.ID}, 0},
		{Tangent, []int{top.ID, arc2.ID}, 0},
		{Tangent, []int{bot.ID, arc1.ID}, 0},
		{Tangent, []int{bot.ID, arc2.ID}, 0},
		{Equal, []int{arc1.ID, arc2.ID}, 0},
		{Radius, []int{arc1.ID}, 8},
		{Fix, []int{c1.ID}, 0},
		{Distance, []int{c1.ID, c2.ID}, 50},
		{Horizontal, []int{c1.ID, c2.ID}, 0},
	} {
		if _, err := s.AddConstraint(k.k, k.ids, k.v); err != nil {
			t.Fatalf("%s: %v", k.k, err)
		}
	}
	if !near(s.Radius(arc2), 8) {
		t.Fatalf("arc 2 radius %.6f", s.Radius(arc2))
	}
	if cy := s.P(c1.ID).Y; !near(s.P(top.Pts[0]).Y-cy, 8) || !near(s.P(bot.Pts[1]).Y-cy, -8) {
		t.Fatalf("lines at %v / %v", s.P(top.Pts[0]), s.P(bot.Pts[1]))
	}
	rs := s.Regions()
	if len(rs) != 1 {
		t.Fatalf("%d regions, want 1", len(rs))
	}
	want := 50*16 + math.Pi*64
	if math.Abs(rs[0].Area-want)/want > 1e-3 {
		t.Fatalf("slot area %.3f, want %.3f", rs[0].Area, want)
	}
	if s.Status.DOF != 0 {
		t.Fatalf("slot DOF %d, want 0", s.Status.DOF)
	}
}

func rect(s *Sketch, x0, y0, x1, y1 float64) {
	a, b := s.AddPoint(geom.V2(x0, y0)), s.AddPoint(geom.V2(x1, y0))
	c, d := s.AddPoint(geom.V2(x1, y1)), s.AddPoint(geom.V2(x0, y1))
	s.AddLine(a.ID, b.ID)
	s.AddLine(b.ID, c.ID)
	s.AddLine(c.ID, d.ID)
	s.AddLine(d.ID, a.ID)
}

func TestRegionsNestedAndShared(t *testing.T) {
	s := New("s", geom.World)
	rect(s, 0, 0, 100, 60)
	rect(s, 10, 10, 30, 30) // hole
	s.AddCircle(s.AddPoint(geom.V2(70, 30)).ID, 10)
	rs := s.Regions()
	if len(rs) != 3 {
		t.Fatalf("%d regions, want 3 (plate, square, disc)", len(rs))
	}
	plate := 6000 - 400 - math.Pi*100
	if math.Abs(rs[0].Area-plate) > 0.5 || len(rs[0].Holes) != 2 {
		t.Fatalf("plate area %.2f with %d holes, want %.2f with 2", rs[0].Area, len(rs[0].Holes), plate)
	}

	// two squares sharing an edge, made by a line across a rectangle
	s2 := New("s2", geom.World)
	rect(s2, 0, 0, 40, 20)
	s2.AddLineXY(geom.V2(20, 0), geom.V2(20, 20))
	rs = s2.Regions()
	if len(rs) != 2 || !near(rs[0].Area, 400) || !near(rs[1].Area, 400) {
		t.Fatalf("split rectangle: %d regions %v", len(rs), areas(rs))
	}

	// crossing lines and a dangling one: still the two triangles' worth
	s3 := New("s3", geom.World)
	rect(s3, 0, 0, 10, 10)
	s3.AddLineXY(geom.V2(0, 0), geom.V2(10, 10))
	s3.AddLineXY(geom.V2(5, 5), geom.V2(20, 5)) // pokes out of the square
	rs = s3.Regions()
	if total := sumAreas(rs); !near(total, 100) || len(rs) != 3 {
		t.Fatalf("crossed square: %d regions %v", len(rs), areas(rs))
	}
}

func areas(rs []Region) []float64 {
	var out []float64
	for _, r := range rs {
		out = append(out, r.Area)
	}
	return out
}

func sumAreas(rs []Region) float64 {
	t := 0.0
	for _, r := range rs {
		t += r.Area
	}
	return t
}

func TestDragKeepsConstraints(t *testing.T) {
	s := New("s", geom.World)
	a := s.AddPoint(geom.V2(0, 0))
	b := s.AddPoint(geom.V2(10, 0))
	l := s.AddLine(a.ID, b.ID)
	s.AddConstraint(Horizontal, []int{l.ID}, 0)
	s.AddConstraint(Distance, []int{a.ID, b.ID}, 10)
	s.Drag(b.ID, s.P(b.ID), geom.V2(15, 7))
	pa, pb := s.P(a.ID), s.P(b.ID)
	if !near(pa.Y, pb.Y) || !near(pa.Dist(pb), 10) {
		t.Fatalf("after drag a %v b %v", pa, pb)
	}
}

func TestDeleteCascades(t *testing.T) {
	s := New("s", geom.World)
	l := s.AddLineXY(geom.V2(0, 0), geom.V2(5, 0))
	s.AddConstraint(Horizontal, []int{l.ID}, 0)
	s.Delete(l.ID)
	if len(s.Ents) != 0 || len(s.Cons) != 0 {
		t.Fatalf("left %d entities, %d constraints", len(s.Ents), len(s.Cons))
	}
}
