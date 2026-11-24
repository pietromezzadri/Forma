package ops

import (
	"fmt"
	"math"

	"forma/brep"
	"forma/geom"
)

// Ball corners: where three rounded edges (between flat faces, all
// convex, one radius) meet at a vertex, the corner is part of the ball
// that rolls along all three — not the mitre their rounds make crossing.
//
// The ball's centre C lies r in from all three faces. Each edge's round
// stops at the plane through C square to the edge (its "end plane"),
// where its cross-section is centred on C. Between the vertex and the
// three end planes the corner tool takes over: the region bounded by the
// three faces and the three end planes, minus the ball. It is built
// directly — three quadrilaterals on the faces, three end faces each with a
// quarter-circle-like arc cut out, and a spherical triangle between the
// three points where the ball touches the faces — so no boolean has to
// find where the ball meets the faces (it only touches them).

// Corner is a vertex whose three edges are rounded together.
type Corner struct {
	V   geom.Vec3    // the vertex
	Dir [3]geom.Vec3 // the edges' directions away from it (unit)
	// the faces' inward normals: In[i] is the face between edges i and
	// i+1 (mod 3)
	In [3]geom.Vec3
}

// CornerFrame works out the ball's centre and how far back from the
// vertex each edge's round stops.
func (c Corner) CornerFrame(r float64) (centre geom.Vec3, setback [3]float64, err error) {
	// n_k·(C−V) = r for the three inward normals
	m := [3]geom.Vec3{c.In[0], c.In[1], c.In[2]}
	det := m[0].Dot(m[1].Cross(m[2]))
	if math.Abs(det) < 1e-9 {
		return geom.Vec3{}, setback, fmt.Errorf("the corner's faces don't meet in a point")
	}
	// Cramer: x = r·(m1×m2 + m2×m0 + m0×m1) / det
	x := m[1].Cross(m[2]).Add(m[2].Cross(m[0])).Add(m[0].Cross(m[1])).Scale(r / det)
	centre = c.V.Add(x)
	for i := range setback {
		setback[i] = x.Dot(c.Dir[i])
		if setback[i] <= geom.Tol {
			return geom.Vec3{}, setback, fmt.Errorf("the corner is too sharp for a ball corner")
		}
	}
	return centre, setback, nil
}

// CornerTool is the solid the ball corner takes off the vertex.
func CornerTool(c Corner, r float64, name string) (*brep.Body, error) {
	C, set, err := c.CornerFrame(r)
	if err != nil {
		return nil, err
	}
	b := brep.NewBuilder(name)
	V := b.Vertex(c.V)
	var A [3]*brep.Vertex // where each edge's round ends
	for i := 0; i < 3; i++ {
		A[i] = b.Vertex(c.V.Add(c.Dir[i].Scale(set[i])))
	}
	// T[i]: where the ball touches face i (between edges i and i+1)
	var T [3]*brep.Vertex
	for i := 0; i < 3; i++ {
		T[i] = b.Vertex(C.Sub(c.In[i].Scale(r)))
	}
	lines := map[[2]*brep.Vertex]*brep.Edge{}
	line := func(p, q *brep.Vertex) {
		lines[[2]*brep.Vertex{p, q}] = b.LineEdge(p, q)
	}
	for i := 0; i < 3; i++ {
		line(V, A[i])
	}
	// on face i: A[i]–T[i] and A[i+1]–T[i]
	for i := 0; i < 3; i++ {
		line(A[i], T[i])
		line(A[(i+1)%3], T[i])
	}
	// arcs: on edge i's end plane, between the touch points of the two faces
	// beside edge i (faces i−1 and i)
	arcs := map[[2]*brep.Vertex]*brep.Edge{}
	for i := 0; i < 3; i++ {
		p, q := T[(i+2)%3], T[i]
		x := p.P.Sub(C).Scale(1 / r)
		z := c.Dir[i]
		y := z.Cross(x)
		th := math.Atan2(q.P.Sub(C).Dot(y), q.P.Sub(C).Dot(x))
		if th < 0 {
			z, y = z.Neg(), y.Neg()
			th = -th
		}
		circ := geom.Circle{F: geom.Frame{O: C, X: x, Y: y, Z: z}, R: r}
		arcs[[2]*brep.Vertex{p, q}] = b.Edge(p, q, circ, 0, th)
	}
	// a face from a ring of vertices, turned to face out along n
	use := func(p, q *brep.Vertex) brep.Use {
		if e, ok := lines[[2]*brep.Vertex{p, q}]; ok {
			return brep.F(e)
		}
		if e, ok := lines[[2]*brep.Vertex{q, p}]; ok {
			return brep.R(e)
		}
		if e, ok := arcs[[2]*brep.Vertex{p, q}]; ok {
			return brep.F(e)
		}
		return brep.R(arcs[[2]*brep.Vertex{q, p}])
	}
	ring := func(vs []*brep.Vertex, n geom.Vec3) []brep.Use {
		// Newell normal of the ring (arcs' middles included)
		var pts []geom.Vec3
		for k := range vs {
			p, q := vs[k], vs[(k+1)%len(vs)]
			pts = append(pts, p.P)
			if e, ok := arcs[[2]*brep.Vertex{p, q}]; ok {
				pts = append(pts, e.Curve.Eval((e.T0+e.T1)/2))
			} else if e, ok := arcs[[2]*brep.Vertex{q, p}]; ok {
				pts = append(pts, e.Curve.Eval((e.T0+e.T1)/2))
			}
		}
		var nw geom.Vec3
		for k := range pts {
			a, d := pts[k], pts[(k+1)%len(pts)]
			nw = nw.Add(geom.V3((a.Y-d.Y)*(a.Z+d.Z), (a.Z-d.Z)*(a.X+d.X), (a.X-d.X)*(a.Y+d.Y)))
		}
		if nw.Dot(n) < 0 {
			rev := make([]*brep.Vertex, len(vs))
			for k := range vs {
				rev[k] = vs[len(vs)-1-k]
			}
			vs = rev
		}
		var out []brep.Use
		for k := range vs {
			out = append(out, use(vs[k], vs[(k+1)%len(vs)]))
		}
		return out
	}
	plane := func(o, n geom.Vec3) geom.Plane {
		x := n.Cross(geom.ZAxis)
		if x.Len() < 0.5 {
			x = n.Cross(geom.XAxis)
		}
		return geom.Plane{F: geom.FrameZ(o, n, x.Norm())}
	}
	// on the body's faces (out of the tool: against the inward normal)
	for i := 0; i < 3; i++ {
		n := c.In[i].Neg()
		b.Face(plane(c.V, n), false, fmt.Sprintf("%s/face:%d", name, i), ring([]*brep.Vertex{V, A[i], T[i], A[(i+1)%3]}, n))
	}
	// end faces, facing along their edge
	for i := 0; i < 3; i++ {
		n := c.Dir[i]
		b.Face(plane(C, n), false, fmt.Sprintf("%s/end:%d", name, i), ring([]*brep.Vertex{A[i], T[i], T[(i+2)%3]}, n))
	}
	// the ball's patch, facing its centre (out of the tool)
	mid := T[0].P.Add(T[1].P).Add(T[2].P).Scale(1.0 / 3).Sub(C).Norm()
	sph := geom.Sphere{F: geom.Frame{O: C, X: geom.XAxis, Y: geom.YAxis, Z: geom.ZAxis}, R: r}
	b.Face(sph, true, name+"/ball", ring([]*brep.Vertex{T[0], T[1], T[2]}, mid.Neg()))
	body, err := b.Finish()
	if err != nil {
		return nil, err
	}
	if err := brep.Check(body); err != nil {
		return nil, fmt.Errorf("corner tool: %v", err)
	}
	return body, nil
}

// BallCornerOf is the ball corner at vertex v of body b, whose edges are
// es (the rounded edges meeting there), when it can have one: exactly
// three edges at the vertex, all of them in es, straight, between flat
// faces, convex. Index i of the Corner is es[i]'s.
func BallCornerOf(b *brep.Body, v *brep.Vertex, es []*brep.Edge) (Corner, bool) {
	if len(es) != 3 {
		return Corner{}, false
	}
	deg := 0
	for _, e := range b.Edges() {
		if e.Verts[0] == v {
			deg++
		}
		if e.Verts[1] == v {
			deg++
		}
	}
	if deg != 3 {
		return Corner{}, false
	}
	var c Corner
	c.V = v.P
	for i, e := range es {
		if e.HE[0] == nil || e.HE[1] == nil || e.Verts[0] == e.Verts[1] {
			return Corner{}, false
		}
		for _, h := range e.HE {
			if _, ok := h.Loop.Face.Surf.(geom.Plane); !ok {
				return Corner{}, false
			}
		}
		a, z := e.Verts[0].P, e.Verts[1].P
		mid := e.Curve.Eval((e.T0 + e.T1) / 2)
		if lineDist(a, z, mid) > 1e-6*(1+a.Dist(z)) || !convex(e.HE[0], e.HE[1], mid) {
			return Corner{}, false
		}
		d := z.Sub(a).Norm()
		if e.Verts[1] == v {
			d = d.Neg()
		}
		c.Dir[i] = d
	}
	// face i: the one edges i and i+1 share
	for i := 0; i < 3; i++ {
		e1, e2 := es[i], es[(i+1)%3]
		var shared *brep.Face
		for _, h1 := range e1.HE {
			for _, h2 := range e2.HE {
				if h1.Loop.Face == h2.Loop.Face {
					shared = h1.Loop.Face
				}
			}
		}
		if shared == nil {
			return Corner{}, false
		}
		u, w := shared.Surf.Project(v.P)
		c.In[i] = shared.OutwardNormal(u, w).Neg()
	}
	return c, true
}
