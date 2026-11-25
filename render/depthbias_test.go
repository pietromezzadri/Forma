package render

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

// In perspective the bias pulls a point the same fraction of its distance
// however near the near plane is: its biased depth is that of a point
// perspRel·b nearer.
func TestDepthBiasIsAFractionOfTheDistance(t *testing.T) {
	const b = 1e-5
	k := b * perspRel
	for _, near := range []float64{1e-3, 0.4, 10} {
		far := 3000.0
		proj := mgl32.Perspective(0.8, 1.5, float32(near), float32(far))
		w, c := depthBias(proj, b)
		for _, z := range []float64{near * 2, 40, 400, far * 0.9} {
			if z <= near {
				continue
			}
			clip := proj.Mul4x1(mgl32.Vec4{0, 0, float32(-z), 1})
			got := float64((clip[2] - w*clip[3] - c) / clip[3])
			p := proj.Mul4x1(mgl32.Vec4{0, 0, float32(-z * (1 - k)), 1})
			want := float64(p[2] / p[3])
			// the constant floor pulls a little more; never less
			if got > want+1e-6 {
				t.Errorf("near %g, z %g: depth %g, want ≤ %g (pulled %g of the distance)", near, z, got, want, k)
			}
			// and not much more than the floor's share
			if got < want-float64(w)-1e-6 {
				t.Errorf("near %g, z %g: depth %g, pulled further than %g", near, z, got, want-float64(w))
			}
		}
	}
}

func TestDepthBiasOrthoIsConstant(t *testing.T) {
	proj := mgl32.Ortho(-10, 10, -10, 10, -500, 500)
	w, c := depthBias(proj, 2e-5)
	if w != 2e-5 || c != 0 {
		t.Fatalf("ortho bias %g, %g; want 2e-5, 0", w, c)
	}
	if math.IsNaN(float64(w)) {
		t.Fatal("NaN")
	}
}
