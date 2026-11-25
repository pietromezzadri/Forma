package render

import "github.com/go-gl/mathgl/mgl32"

// Lines and points lying on faces are pulled toward the camera so they win
// the depth test against them. A bias b is turned into two clip-space z
// offsets for the shaders (z − w·b − near):
//
// In perspective a constant offset in normalised depth moves a point by
// b·z²/(2n) in the world (z its distance, n the near plane): next to
// nothing far off, millimetres in a close-up, where edges then showed
// through the faces in front of them (a hole's bottom through a plate's
// side). So the pull is a fraction of the distance instead, perspRel·b of
// it: the depth a point a fraction k nearer would have, at its own w, is z
// less 2fn/(f−n)·k/(1−k) in clip space — the same for every point, the
// projection's own term (−m[3][2]) scaled. A little of the constant offset
// (perspFloor·b) stays for the far parts of a deep scene, where a float's
// steps in depth are coarser than that fraction.
//
// In an orthographic view depth is linear and its steps are the same
// everywhere, so the constant offset is the right one, as before.
const (
	perspRel   = 40  // b 1e-5 (the edges) pulls them 0.04 % of their distance
	perspFloor = 0.1 // of b as a constant offset in normalised depth
)

// depthBias gives the shaders' uDepthBias (times w) and uDepthNear
// (constant) for a bias and a projection.
func depthBias(proj mgl32.Mat4, b float32) (w, near float32) {
	if proj[15] != 0 { // orthographic: no perspective divide
		return b, 0
	}
	k := float64(b) * perspRel
	if k >= 0.5 {
		k = 0.5
	}
	m := -float64(proj[14]) // 2fn/(f−n)
	return b * perspFloor, float32(m * k / (1 - k))
}
