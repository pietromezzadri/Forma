package app

import (
	"math"
	"testing"

	"forma/geom"
)

// The near plane never passes anything in view, and in a close-up it is far
// enough out for depth to keep edges behind the faces in front of them.
func TestCameraNearPlane(t *testing.T) {
	plate := geom.Box{Min: geom.V3(0, 0, 0), Max: geom.V3(100, 80, 10)}
	c := &Camera{Target: geom.V3(50, 0, 5), Dist: 40, Yaw: math.Pi / 2, Pitch: 0.3, FovY: 0.8}
	const aspect = 1.6
	far := 1000.0
	n := c.near(aspect, plate, far)
	if n < c.Dist*0.01 || n > c.Dist*0.5 {
		t.Fatalf("near %g out of [%g, %g]", n, c.Dist*0.01, c.Dist*0.5)
	}
	// nothing of the box in view is nearer than it: sample the box
	eye := c.Eye()
	f, rt, up := c.Basis()
	th := math.Tan(c.FovY / 2)
	least := math.Inf(1)
	for i := 0; i <= 40; i++ {
		for j := 0; j <= 40; j++ {
			for k := 0; k <= 10; k++ {
				p := geom.V3(plate.Min.X+(plate.Max.X-plate.Min.X)*float64(i)/40,
					plate.Min.Y+(plate.Max.Y-plate.Min.Y)*float64(j)/40,
					plate.Min.Z+(plate.Max.Z-plate.Min.Z)*float64(k)/10)
				q := p.Sub(eye)
				z := q.Dot(f)
				if z <= 0 || math.Abs(q.Dot(rt)) > th*aspect*z || math.Abs(q.Dot(up)) > th*z {
					continue
				}
				least = math.Min(least, z)
			}
		}
	}
	if n > least {
		t.Errorf("near %g cuts the plate, whose nearest point in view is %g away", n, least)
	}
	z, ok := c.nearestInView(aspect, plate)
	if !ok || z > least+1e-9 || z < least-1 {
		t.Errorf("nearest in view %g (%v), sampled %g", z, ok, least)
	}

	// the eye inside the box: a hundredth of the way to the target
	big := geom.Box{Min: geom.V3(-1000, -1000, -1000), Max: geom.V3(1000, 1000, 1000)}
	c.Dist = 5
	if n := c.near(aspect, big, 6000); math.Abs(n-0.06) > 1e-9 {
		t.Errorf("inside the scene: near %g, want far × 1e-5 = 0.06", n)
	}
	if n := c.near(aspect, big, 100); math.Abs(n-0.05) > 1e-9 {
		t.Errorf("inside the scene: near %g, want Dist / 100 = 0.05", n)
	}
}
