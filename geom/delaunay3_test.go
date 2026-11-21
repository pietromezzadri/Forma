package geom

import (
	"math"
	"math/rand"
	"testing"
)

// Random points: the tetrahedra fill their convex hull (volumes add up to
// it), none is inverted, and none holds another point in its circumsphere.
func TestDelaunay3(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	var pts []Vec3
	// a box's corners and random points inside: the hull is the box
	for _, x := range []float64{0, 2} {
		for _, y := range []float64{0, 3} {
			for _, z := range []float64{0, 1} {
				pts = append(pts, V3(x, y, z))
			}
		}
	}
	for i := 0; i < 2000; i++ {
		pts = append(pts, V3(rng.Float64()*2, rng.Float64()*3, rng.Float64()))
	}
	tets := Delaunay3(pts)
	vol := 0.0
	for _, tt := range tets {
		a, b, c, d := pts[tt[0]], pts[tt[1]], pts[tt[2]], pts[tt[3]]
		v := orient3(a, b, c, d) / 6
		if v <= 0 {
			t.Fatalf("tetrahedron %v has volume %g", tt, v)
		}
		vol += v
	}
	if math.Abs(vol-6) > 1e-9 {
		t.Fatalf("volume %.12f, want 6 (%d tetrahedra)", vol, len(tets))
	}
	// Delaunay: spot-check circumspheres against all points
	for k := 0; k < len(tets); k += len(tets) / 50 {
		tt := tets[k]
		c, r := circumsphere(pts[tt[0]], pts[tt[1]], pts[tt[2]], pts[tt[3]])
		for i, p := range pts {
			if i == tt[0] || i == tt[1] || i == tt[2] || i == tt[3] {
				continue
			}
			if p.Dist(c) < r*(1-1e-9) {
				t.Fatalf("point %d inside the circumsphere of %v", i, tt)
			}
		}
	}
	t.Logf("%d points, %d tetrahedra", len(pts), len(tets))
}

func circumsphere(a, b, c, d Vec3) (Vec3, float64) {
	// solve 2 (x − a)·(b − a) = |b|² − |a|², etc.
	u, v, w := b.Sub(a), c.Sub(a), d.Sub(a)
	den := 2 * u.Dot(v.Cross(w))
	o := v.Cross(w).Scale(u.Dot(u)).Add(w.Cross(u).Scale(v.Dot(v))).Add(u.Cross(v).Scale(w.Dot(w))).Scale(1 / den)
	return a.Add(o), o.Len()
}
