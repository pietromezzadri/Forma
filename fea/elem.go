package fea

import (
	"math"

	"forma/geom"
)

// The 10-node (quadratic) tetrahedron: corners 0–3 at natural coordinates
// L1..L4 = 1−ξ−η−ζ, ξ, η, ζ, and a node at the middle of each edge.
var tetEdges = [6][2]int{{0, 1}, {1, 2}, {0, 2}, {0, 3}, {1, 3}, {2, 3}}

// natural coordinates of the ten nodes (ξ, η, ζ)
var nodeXi = func() [10][3]float64 {
	c := [4][3]float64{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	var out [10][3]float64
	copy(out[:4], c[:])
	for k, e := range tetEdges {
		for i := 0; i < 3; i++ {
			out[4+k][i] = (c[e[0]][i] + c[e[1]][i]) / 2
		}
	}
	return out
}()

// gauss4 is the 4-point rule for a tetrahedron (exact to degree 2), with
// weights for the reference tetrahedron's volume 1/6.
var gauss4 = func() [4][4]float64 {
	a, b := 0.5854101966249685, 0.1381966011250105
	w := 1.0 / 24
	return [4][4]float64{{a, b, b, w}, {b, a, b, w}, {b, b, a, w}, {b, b, b, w}}
}()

// shapeDeriv are dN/dξ, dN/dη, dN/dζ of the ten shape functions at a point.
func shapeDeriv(xi, eta, zeta float64) [10][3]float64 {
	L := [4]float64{1 - xi - eta - zeta, xi, eta, zeta}
	dL := [4][3]float64{{-1, -1, -1}, {1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	var d [10][3]float64
	for i := 0; i < 4; i++ {
		for k := 0; k < 3; k++ {
			d[i][k] = (4*L[i] - 1) * dL[i][k]
		}
	}
	for e, ed := range tetEdges {
		a, b := ed[0], ed[1]
		for k := 0; k < 3; k++ {
			d[4+e][k] = 4 * (L[a]*dL[b][k] + L[b]*dL[a][k])
		}
	}
	return d
}

// gradients are dN/dx of the ten shape functions at a natural point, and
// the Jacobian's determinant there.
func gradients(x *[10]geom.Vec3, xi, eta, zeta float64) ([10][3]float64, float64) {
	d := shapeDeriv(xi, eta, zeta)
	var J [3][3]float64 // J[i][j] = dx_i / dξ_j
	for n := 0; n < 10; n++ {
		p := [3]float64{x[n].X, x[n].Y, x[n].Z}
		for i := 0; i < 3; i++ {
			for j := 0; j < 3; j++ {
				J[i][j] += p[i] * d[n][j]
			}
		}
	}
	det := J[0][0]*(J[1][1]*J[2][2]-J[1][2]*J[2][1]) -
		J[0][1]*(J[1][0]*J[2][2]-J[1][2]*J[2][0]) +
		J[0][2]*(J[1][0]*J[2][1]-J[1][1]*J[2][0])
	var inv [3][3]float64
	if det != 0 {
		inv[0][0] = (J[1][1]*J[2][2] - J[1][2]*J[2][1]) / det
		inv[0][1] = (J[0][2]*J[2][1] - J[0][1]*J[2][2]) / det
		inv[0][2] = (J[0][1]*J[1][2] - J[0][2]*J[1][1]) / det
		inv[1][0] = (J[1][2]*J[2][0] - J[1][0]*J[2][2]) / det
		inv[1][1] = (J[0][0]*J[2][2] - J[0][2]*J[2][0]) / det
		inv[1][2] = (J[0][2]*J[1][0] - J[0][0]*J[1][2]) / det
		inv[2][0] = (J[1][0]*J[2][1] - J[1][1]*J[2][0]) / det
		inv[2][1] = (J[0][1]*J[2][0] - J[0][0]*J[2][1]) / det
		inv[2][2] = (J[0][0]*J[1][1] - J[0][1]*J[1][0]) / det
	}
	var g [10][3]float64
	for n := 0; n < 10; n++ {
		for i := 0; i < 3; i++ {
			// dN/dx_i = Σ_j dN/dξ_j dξ_j/dx_i
			g[n][i] = d[n][0]*inv[0][i] + d[n][1]*inv[1][i] + d[n][2]*inv[2][i]
		}
	}
	return g, det
}

// elasticity is the isotropic stiffness matrix in Voigt order
// (xx, yy, zz, xy, yz, zx; shears as engineering strains).
func elasticity(E, nu float64) [6][6]float64 {
	lam := E * nu / ((1 + nu) * (1 - 2*nu))
	mu := E / (2 * (1 + nu))
	var D [6][6]float64
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			D[i][j] = lam
		}
		D[i][i] = lam + 2*mu
		D[3+i][3+i] = mu
	}
	return D
}

// stiffness is a tetrahedron's 30 × 30 stiffness matrix (node-major,
// x y z per node); ok is false if it is turned inside out anywhere.
func stiffness(x *[10]geom.Vec3, D *[6][6]float64, ke *[30][30]float64) bool {
	*ke = [30][30]float64{}
	for _, gp := range gauss4 {
		g, det := gradients(x, gp[0], gp[1], gp[2])
		if det <= 0 {
			return false
		}
		w := gp[3] * det
		// DB for each node's three columns, then accumulate Bᵀ (DB)
		var db [10][3][6]float64
		for n := 0; n < 10; n++ {
			bx, by, bz := g[n][0], g[n][1], g[n][2]
			// B columns for u_x, u_y, u_z of node n
			cols := [3][6]float64{
				{bx, 0, 0, by, 0, bz},
				{0, by, 0, bx, bz, 0},
				{0, 0, bz, 0, by, bx},
			}
			for c := 0; c < 3; c++ {
				for r := 0; r < 6; r++ {
					s := 0.0
					for k := 0; k < 6; k++ {
						s += D[r][k] * cols[c][k]
					}
					db[n][c][r] = s
				}
			}
		}
		for m := 0; m < 10; m++ {
			bx, by, bz := g[m][0], g[m][1], g[m][2]
			rows := [3][6]float64{
				{bx, 0, 0, by, 0, bz},
				{0, by, 0, bx, bz, 0},
				{0, 0, bz, 0, by, bx},
			}
			for a := 0; a < 3; a++ {
				for n := 0; n < 10; n++ {
					for c := 0; c < 3; c++ {
						s := 0.0
						for k := 0; k < 6; k++ {
							s += rows[a][k] * db[n][c][k]
						}
						ke[3*m+a][3*n+c] += w * s
					}
				}
			}
		}
	}
	return true
}

// stressAt is the stress (Voigt) at a natural point of a tetrahedron with
// nodal displacements u.
func stressAt(x *[10]geom.Vec3, u *[10]geom.Vec3, D *[6][6]float64, xi, eta, zeta float64) [6]float64 {
	g, _ := gradients(x, xi, eta, zeta)
	var eps [6]float64
	for n := 0; n < 10; n++ {
		bx, by, bz := g[n][0], g[n][1], g[n][2]
		ux, uy, uz := u[n].X, u[n].Y, u[n].Z
		eps[0] += bx * ux
		eps[1] += by * uy
		eps[2] += bz * uz
		eps[3] += by*ux + bx*uy
		eps[4] += bz*uy + by*uz
		eps[5] += bz*ux + bx*uz
	}
	var s [6]float64
	for i := 0; i < 6; i++ {
		for k := 0; k < 6; k++ {
			s[i] += D[i][k] * eps[k]
		}
	}
	return s
}

// vonMises is the equivalent stress of a stress (Voigt).
func vonMises(s [6]float64) float64 {
	a, b, c := s[0]-s[1], s[1]-s[2], s[2]-s[0]
	return math.Sqrt(0.5*(a*a+b*b+c*c) + 3*(s[3]*s[3]+s[4]*s[4]+s[5]*s[5]))
}
