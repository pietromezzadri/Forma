package sketch

import (
	"testing"

	"forma/geom"
)

// Inter draws a D as a bowl and a stem whose bottom and top edges lie on
// each other: the letter must still be one inked region (it was none,
// the shared edges counted twice).
func TestOverlappingContoursMakeALetter(t *testing.T) {
	FontDirs = append(FontDirs, "../assets/fonts")
	for txt, want := range map[string]int{"D": 1, "ROUND": 5, "DOD": 3} {
		s := New("t", geom.World)
		a := s.AddPoint(geom.V2(0, 0))
		e := s.AddText(a.ID, txt, DefaultFont, 5)
		s.Solve()
		n := 0
		for _, r := range s.Regions() {
			if r.Text == e.ID && r.Ink {
				n++
			}
		}
		if n != want {
			t.Errorf("%q: %d letters, want %d", txt, n, want)
		}
	}
}

// Two lines drawn over each other along a square's side still close it.
func TestOverlappingLinesCloseARegion(t *testing.T) {
	s := New("t", geom.World)
	p := func(x, y float64) int { return s.AddPoint(geom.V2(x, y)).ID }
	a, b, c, d := p(0, 0), p(10, 0), p(10, 10), p(0, 10)
	s.AddLine(a, b)
	s.AddLine(b, c)
	s.AddLine(c, d)
	s.AddLine(d, a)
	// the bottom drawn again, longer, over the first
	s.AddLine(p(-2, 0), p(6, 0))
	s.Solve()
	n := 0
	for _, r := range s.Regions() {
		if r.Area > 99 && r.Area < 101 {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d regions of the square's area, want 1", n)
	}
}
