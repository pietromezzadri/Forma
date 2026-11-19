package brep

import (
	"math"

	"forma/geom"
)

// MassProps are a solid's mass properties at unit density: volume (mm³),
// surface area (mm²), centroid, and the inertia tensor about the centroid
// (mm⁵; times a density in kg/mm³ for kg·mm²) with its principal moments.
type MassProps struct {
	Volume, Area float64
	Centroid     geom.Vec3
	Inertia      [3][3]float64
	Principal    [3]float64
}

// Mass works the properties out from a tessellation (each triangle a
// tetrahedron with the origin), as exact as the mesh: rel is its chord
// tolerance over the body's size (1e-4: a part in 10⁴ on curved bodies;
// exact on flat ones).
func Mass(b *Body, rel float64) MassProps {
	opt := DefaultMeshOptions(b)
	opt.ChordTol = math.Max(b.Bounds().Diagonal()*rel, 1e-6)
	return MeshMass(Tessellate(b, opt))
}

// MeshMass is Mass of a closed, outward-facing mesh.
func MeshMass(m *Mesh) MassProps {
	var mp MassProps
	var c geom.Vec3
	var xx, yy, zz, xy, yz, zx float64
	for _, t := range m.Tris {
		a, b, d := m.Pos[t[0]], m.Pos[t[1]], m.Pos[t[2]]
		mp.Area += b.Sub(a).Cross(d.Sub(a)).Len() / 2
		v := a.Dot(b.Cross(d)) / 6
		mp.Volume += v
		c = c.Add(a.Add(b).Add(d).Scale(v / 4))
		sq := func(p, q, r float64) float64 { return p*p + q*q + r*r + p*q + p*r + q*r }
		xx += v / 10 * sq(a.X, b.X, d.X)
		yy += v / 10 * sq(a.Y, b.Y, d.Y)
		zz += v / 10 * sq(a.Z, b.Z, d.Z)
		pr := func(p0, p1, p2, q0, q1, q2 float64) float64 {
			return 2*(p0*q0+p1*q1+p2*q2) + p0*q1 + p1*q0 + p0*q2 + p2*q0 + p1*q2 + p2*q1
		}
		xy += v / 20 * pr(a.X, b.X, d.X, a.Y, b.Y, d.Y)
		yz += v / 20 * pr(a.Y, b.Y, d.Y, a.Z, b.Z, d.Z)
		zx += v / 20 * pr(a.Z, b.Z, d.Z, a.X, b.X, d.X)
	}
	if mp.Volume == 0 {
		return mp
	}
	c = c.Scale(1 / mp.Volume)
	mp.Centroid = c
	V := mp.Volume
	// second moments about the centroid
	xx -= V * c.X * c.X
	yy -= V * c.Y * c.Y
	zz -= V * c.Z * c.Z
	xy -= V * c.X * c.Y
	yz -= V * c.Y * c.Z
	zx -= V * c.Z * c.X
	mp.Inertia = [3][3]float64{
		{yy + zz, -xy, -zx},
		{-xy, zz + xx, -yz},
		{-zx, -yz, xx + yy},
	}
	mp.Principal = eigenSym(mp.Inertia)
	return mp
}

// eigenSym is the eigenvalues of a symmetric 3×3 matrix, smallest first
// (Jacobi rotations).
func eigenSym(m [3][3]float64) [3]float64 {
	a := m
	for sweep := 0; sweep < 50; sweep++ {
		off := a[0][1]*a[0][1] + a[0][2]*a[0][2] + a[1][2]*a[1][2]
		if off < 1e-24*(a[0][0]*a[0][0]+a[1][1]*a[1][1]+a[2][2]*a[2][2]+1e-300) {
			break
		}
		for p := 0; p < 2; p++ {
			for q := p + 1; q < 3; q++ {
				if math.Abs(a[p][q]) < 1e-300 {
					continue
				}
				th := (a[q][q] - a[p][p]) / (2 * a[p][q])
				t := 1 / (math.Abs(th) + math.Sqrt(th*th+1))
				if th < 0 {
					t = -t
				}
				cs := 1 / math.Sqrt(t*t+1)
				sn := t * cs
				for k := 0; k < 3; k++ {
					akp, akq := a[k][p], a[k][q]
					a[k][p] = cs*akp - sn*akq
					a[k][q] = sn*akp + cs*akq
				}
				for k := 0; k < 3; k++ {
					apk, aqk := a[p][k], a[q][k]
					a[p][k] = cs*apk - sn*aqk
					a[q][k] = sn*apk + cs*aqk
				}
			}
		}
	}
	e := [3]float64{a[0][0], a[1][1], a[2][2]}
	for i := 0; i < 3; i++ {
		for j := i + 1; j < 3; j++ {
			if e[j] < e[i] {
				e[i], e[j] = e[j], e[i]
			}
		}
	}
	return e
}
