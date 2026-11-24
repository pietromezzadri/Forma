package ops

import (
	"fmt"

	"forma/brep"
	"forma/geom"
)

// Splitting a body by a plane: what lies in front of it (along its normal)
// and what lies behind, each a body of its own. A body the plane cuts in
// several places (a U cut across its arms) gives several pieces a side,
// which stay together as one body there.

// ErrSplitMiss: the plane doesn't go through the body.
var ErrSplitMiss = fmt.Errorf("the plane doesn't cut through the body (it misses it, or only touches a face): move it, or pick another")

// SplitBody cuts b by the plane (O on it, Z its normal) into the part in
// front of it and the part behind. The new faces on the plane are named
// name (with their "front" / "back" side).
func SplitBody(b *brep.Body, plane geom.Frame, name string) (front, back *brep.Body, err error) {
	bx := b.Bounds()
	big := 2*bx.Diagonal() + 10
	// the half-space boxes, centred on the body's middle seen along the plane
	c := bx.Center()
	o := c.Sub(plane.Z.Scale(c.Sub(plane.O).Dot(plane.Z)))
	fr := geom.Frame{O: o, X: plane.X, Y: plane.Y, Z: plane.Z}
	half := func(z0, z1 float64, side string) (*brep.Body, error) {
		return frameBox(fr, -big, big, -big, big, z0, z1, name+"/"+side)
	}
	inFront, err := half(0, big, "front") // its face on the plane is the front part's new face
	if err != nil {
		return nil, nil, err
	}
	behind, err := half(-big, 0, "back")
	if err != nil {
		return nil, nil, err
	}
	front, err = brep.Boolean(b, inFront, brep.Intersect)
	if err == brep.ErrEmpty {
		return nil, nil, ErrSplitMiss
	}
	if err != nil {
		return nil, nil, err
	}
	back, err = brep.Boolean(b, behind, brep.Intersect)
	if err == brep.ErrEmpty {
		return nil, nil, ErrSplitMiss
	}
	if err != nil {
		return nil, nil, err
	}
	front.Name, back.Name = b.Name, b.Name
	return front, back, nil
}

// frameBox is the box x0..x1, y0..y1, z0..z1 in frame fr (right-handed),
// its faces named after their side (name/+z for the face at z1, …); the
// face lying at z = 0, if any, takes the plain name.
func frameBox(fr geom.Frame, x0, x1, y0, y1, z0, z1 float64, name string) (*brep.Body, error) {
	if x1-x0 < geom.Tol || y1-y0 < geom.Tol || z1-z0 < geom.Tol {
		return nil, fmt.Errorf("box: zero size")
	}
	at := func(i, j, k int) geom.Vec3 {
		return fr.O.Add(fr.X.Scale(pick(i, x0, x1))).Add(fr.Y.Scale(pick(j, y0, y1))).Add(fr.Z.Scale(pick(k, z0, z1)))
	}
	pts := []geom.Vec3{at(0, 0, 0), at(1, 0, 0), at(1, 1, 0), at(0, 1, 0), at(0, 0, 1), at(1, 0, 1), at(1, 1, 1), at(0, 1, 1)}
	faces := [][]int{{0, 3, 2, 1}, {4, 5, 6, 7}, {0, 1, 5, 4}, {1, 2, 6, 5}, {2, 3, 7, 6}, {3, 0, 4, 7}}
	names := []string{name + "/-z", name + "/+z", name + "/-y", name + "/+x", name + "/+y", name + "/-x"}
	switch {
	case z0 == 0:
		names[0] = name
	case z1 == 0:
		names[1] = name
	}
	return Polyhedron(name, pts, faces, names)
}
