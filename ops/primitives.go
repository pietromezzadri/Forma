// Package ops makes and changes bodies: primitives now; extrude, revolve,
// booleans and fillets later.
package ops

import (
	"fmt"
	"math"

	"forma/brep"
	"forma/geom"
)

// Polyhedron builds a body with planar faces from points and faces given as
// counter-clockwise (seen from outside) lists of point indices.
func Polyhedron(name string, pts []geom.Vec3, faces [][]int, faceNames []string) (*brep.Body, error) {
	b := brep.NewBuilder(name)
	vs := make([]*brep.Vertex, len(pts))
	for i, p := range pts {
		vs[i] = b.Vertex(p)
	}
	edges := map[[2]int]*brep.Edge{}
	use := func(a, c int) brep.Use {
		if e, ok := edges[[2]int{c, a}]; ok {
			return brep.R(e)
		}
		e := b.LineEdge(vs[a], vs[c])
		edges[[2]int{a, c}] = e
		return brep.F(e)
	}
	for fi, f := range faces {
		if len(f) < 3 {
			return nil, fmt.Errorf("face %d has %d points", fi, len(f))
		}
		var loop []brep.Use
		for i := range f {
			loop = append(loop, use(f[i], f[(i+1)%len(f)]))
		}
		// Newell normal
		var n geom.Vec3
		for i := range f {
			a, c := pts[f[i]], pts[f[(i+1)%len(f)]]
			n = n.Add(geom.V3((a.Y-c.Y)*(a.Z+c.Z), (a.Z-c.Z)*(a.X+c.X), (a.X-c.X)*(a.Y+c.Y)))
		}
		x := pts[f[1]].Sub(pts[f[0]])
		name := fmt.Sprintf("face %d", fi+1)
		if fi < len(faceNames) {
			name = faceNames[fi]
		}
		b.Face(geom.Plane{F: geom.FrameZ(pts[f[0]], n, x)}, false, name, loop)
	}
	return b.Finish()
}

// Box spans the two corners.
func Box(a, c geom.Vec3) (*brep.Body, error) {
	lo, hi := a.Min(c), a.Max(c)
	if hi.X-lo.X < geom.Tol || hi.Y-lo.Y < geom.Tol || hi.Z-lo.Z < geom.Tol {
		return nil, fmt.Errorf("box: zero size")
	}
	p := func(i, j, k int) geom.Vec3 {
		return geom.V3(pick(i, lo.X, hi.X), pick(j, lo.Y, hi.Y), pick(k, lo.Z, hi.Z))
	}
	pts := []geom.Vec3{p(0, 0, 0), p(1, 0, 0), p(1, 1, 0), p(0, 1, 0), p(0, 0, 1), p(1, 0, 1), p(1, 1, 1), p(0, 1, 1)}
	faces := [][]int{
		{0, 3, 2, 1}, // bottom
		{4, 5, 6, 7}, // top
		{0, 1, 5, 4}, // front (-Y)
		{1, 2, 6, 5}, // right (+X)
		{2, 3, 7, 6}, // back (+Y)
		{3, 0, 4, 7}, // left (-X)
	}
	return Polyhedron("Box", pts, faces, []string{"bottom", "top", "front", "right", "back", "left"})
}

func pick(i int, a, b float64) float64 {
	if i == 0 {
		return a
	}
	return b
}

// Cylinder of radius r and height h standing on f's XY plane along f.Z.
func Cylinder(f geom.Frame, r, h float64) (*brep.Body, error) {
	return Cone(f, r, r, h)
}

// Cone (a frustum) from radius r0 at the base to r1 at height h; r1 = 0 is
// a pointed cone.
func Cone(f geom.Frame, r0, r1, h float64) (*brep.Body, error) {
	if r0 <= geom.Tol || r1 < 0 || h <= geom.Tol {
		return nil, fmt.Errorf("cone: bad size")
	}
	name := "Cone"
	if math.Abs(r1-r0) < geom.Tol {
		name = "Cylinder"
	}
	b := brep.NewBuilder(name)
	top := f
	top.O = f.ToWorld(geom.V3(0, 0, h))
	vb := b.Vertex(f.ToWorld(geom.V3(r0, 0, 0)))
	cb := b.Edge(vb, vb, geom.Circle{F: f, R: r0}, 0, 2*math.Pi)

	var side geom.Surface
	if name == "Cylinder" {
		side = geom.Cylinder{F: f, R: r0}
	} else {
		side = geom.Cone{F: f, R: r0, A: math.Atan((r1 - r0) / h)}
	}
	b.Face(geom.Plane{F: f.Flip()}, false, "bottom", []brep.Use{brep.R(cb)})
	if r1 <= geom.Tol {
		apex := b.Vertex(top.O)
		seam := b.LineEdge(vb, apex)
		b.Face(side, false, "side", []brep.Use{brep.F(cb), brep.F(seam), brep.R(seam)})
	} else {
		vt := b.Vertex(top.ToWorld(geom.V3(r1, 0, 0)))
		ct := b.Edge(vt, vt, geom.Circle{F: top, R: r1}, 0, 2*math.Pi)
		seam := b.LineEdge(vb, vt)
		b.Face(geom.Plane{F: top}, false, "top", []brep.Use{brep.F(ct)})
		b.Face(side, false, "side", []brep.Use{brep.F(cb), brep.F(seam), brep.R(ct), brep.R(seam)})
	}
	return b.Finish()
}

// Sphere of radius r round c.
func Sphere(c geom.Vec3, r float64) (*brep.Body, error) {
	if r <= geom.Tol {
		return nil, fmt.Errorf("sphere: bad radius")
	}
	b := brep.NewBuilder("Sphere")
	f := geom.World
	f.O = c
	s := b.Vertex(c.Add(geom.V3(0, 0, -r)))
	n := b.Vertex(c.Add(geom.V3(0, 0, r)))
	mer := geom.Circle{F: geom.Frame{O: c, X: geom.XAxis, Y: geom.ZAxis, Z: geom.YAxis.Neg()}, R: r}
	seam := b.Edge(s, n, mer, -math.Pi/2, math.Pi/2)
	b.Face(geom.Sphere{F: f, R: r}, false, "sphere", []brep.Use{brep.F(seam), brep.R(seam)})
	return b.Finish()
}

// Torus round f.Z: major radius R, tube radius rm.
func Torus(f geom.Frame, R, rm float64) (*brep.Body, error) {
	if rm <= geom.Tol || R <= rm {
		return nil, fmt.Errorf("torus: need 0 < tube radius < major radius")
	}
	b := brep.NewBuilder("Torus")
	v := b.Vertex(f.ToWorld(geom.V3(R+rm, 0, 0)))
	major := b.Edge(v, v, geom.Circle{F: f, R: R + rm}, 0, 2*math.Pi)
	tf := geom.Frame{O: f.ToWorld(geom.V3(R, 0, 0)), X: f.X, Y: f.Z, Z: f.Y.Neg()}
	tube := b.Edge(v, v, geom.Circle{F: tf, R: rm}, 0, 2*math.Pi)
	b.Face(geom.Torus{F: f, R: R, Rm: rm}, false, "torus",
		[]brep.Use{brep.F(major), brep.F(tube), brep.R(major), brep.R(tube)})
	return b.Finish()
}
