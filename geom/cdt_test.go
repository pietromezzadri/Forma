package geom

import (
	"math"
	"testing"
)

func area2(pts []Vec2, tris [][3]int) float64 {
	s := 0.0
	for _, t := range tris {
		s += orient(pts[t[0]], pts[t[1]], pts[t[2]])
	}
	return s / 2
}

func TestTriangulateSquareWithHole(t *testing.T) {
	pts := []Vec2{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {3, 3}, {3, 7}, {7, 7}, {7, 3}}
	segs := [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 0}, {4, 5}, {5, 6}, {6, 7}, {7, 4}}
	tris := Triangulate(pts, segs)
	if a := area2(pts, tris); math.Abs(a-84) > 1e-9 {
		t.Fatalf("area %.6f, want 84", a)
	}
	for _, tr := range tris {
		if orient(pts[tr[0]], pts[tr[1]], pts[tr[2]]) <= 0 {
			t.Fatal("clockwise triangle")
		}
	}
}

// A comb whose teeth force constraint recovery.
func TestTriangulateComb(t *testing.T) {
	var pts []Vec2
	n := 12
	for i := 0; i <= n; i++ {
		x := float64(i)
		y := 1.0
		if i%2 == 1 {
			y = 8
		}
		pts = append(pts, Vec2{x, y})
	}
	pts = append(pts, Vec2{float64(n), 0}, Vec2{0, 0})
	var segs [][2]int
	for i := range pts {
		segs = append(segs, [2]int{i, (i + 1) % len(pts)})
	}
	tris := Triangulate(pts, segs)
	// shoelace area of the polygon
	want := 0.0
	for i := range pts {
		want += pts[i].Cross(pts[(i+1)%len(pts)])
	}
	want = math.Abs(want) / 2
	if a := area2(pts, tris); math.Abs(a-want) > 1e-9 {
		t.Fatalf("area %.6f, want %.6f", a, want)
	}
}
