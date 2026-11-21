package brep

import "forma/geom"

// For the external tests: the fast and the exact winding numbers.
func WindingFast(m *Mesh) func(geom.Vec3) float64 { return newWindTree(m).at }
func WindingExact(m *Mesh, q geom.Vec3) float64   { return windingNumber(m, q) }
