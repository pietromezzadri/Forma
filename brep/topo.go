// Package brep is Forma's boundary representation: bodies made of shells of
// faces bounded by loops of half-edges, with analytic geometry on every face
// and edge.
//
// Conventions: a face's outward normal is its surface's natural normal
// (∂u × ∂v), negated when Reversed. Its first loop is the outer one, running
// counter-clockwise seen from outside; the other loops (holes) run clockwise.
// An edge runs along its curve from Verts[0] at T0 to Verts[1] at T1; its
// half-edge HE[0] goes the same way, HE[1] the other.
package brep

import "forma/geom"

type Vertex struct {
	ID int
	P  geom.Vec3
}

type Edge struct {
	ID     int
	Curve  geom.Curve
	T0, T1 float64
	Verts  [2]*Vertex
	HE     [2]*HalfEdge
}

// HalfEdge is one side of an edge, in one loop.
type HalfEdge struct {
	Edge       *Edge
	Loop       *Loop
	Fwd        bool // runs along the edge's curve
	Next, Prev *HalfEdge
}

type Loop struct {
	ID    int
	Face  *Face
	First *HalfEdge
	// Lone is the vertex of an edgeless loop (Euler operators make these
	// while a solid is being built); First is then nil.
	Lone *Vertex
}

type Face struct {
	ID       int
	Surf     geom.Surface
	Reversed bool
	Loops    []*Loop
	Shell    *Shell
	// Name is the face's role from the operation that made it (top, side 3, …),
	// the start of persistent naming.
	Name string
}

type Shell struct {
	ID    int
	Faces []*Face
}

type Body struct {
	Name   string
	Shells []*Shell
	nextID int
}

func (b *Body) newID() int { b.nextID++; return b.nextID }

// Start is the vertex a half-edge leaves from.
func (h *HalfEdge) Start() *Vertex {
	if h.Fwd {
		return h.Edge.Verts[0]
	}
	return h.Edge.Verts[1]
}

// End is the vertex it arrives at.
func (h *HalfEdge) End() *Vertex {
	if h.Fwd {
		return h.Edge.Verts[1]
	}
	return h.Edge.Verts[0]
}

// Twin is the other half of the edge.
func (h *HalfEdge) Twin() *HalfEdge {
	if h.Edge.HE[0] == h {
		return h.Edge.HE[1]
	}
	return h.Edge.HE[0]
}

// Params are the curve parameters at the half-edge's start and end.
func (h *HalfEdge) Params() (float64, float64) {
	if h.Fwd {
		return h.Edge.T0, h.Edge.T1
	}
	return h.Edge.T1, h.Edge.T0
}

// HalfEdges lists a loop's half-edges in order.
func (l *Loop) HalfEdges() []*HalfEdge {
	var out []*HalfEdge
	if l.First == nil {
		return nil
	}
	h := l.First
	for {
		out = append(out, h)
		h = h.Next
		if h == l.First || len(out) > 1<<20 {
			break
		}
	}
	return out
}

// OutwardNormal is the face's outward normal at (u, v).
func (f *Face) OutwardNormal(u, v float64) geom.Vec3 {
	n := f.Surf.Normal(u, v)
	if f.Reversed {
		return n.Neg()
	}
	return n
}

// Faces lists all faces of the body.
func (b *Body) Faces() []*Face {
	var out []*Face
	for _, s := range b.Shells {
		out = append(out, s.Faces...)
	}
	return out
}

// Edges lists every edge once, in the order first met.
func (b *Body) Edges() []*Edge {
	seen := map[*Edge]bool{}
	var out []*Edge
	for _, f := range b.Faces() {
		for _, l := range f.Loops {
			for _, h := range l.HalfEdges() {
				if !seen[h.Edge] {
					seen[h.Edge] = true
					out = append(out, h.Edge)
				}
			}
		}
	}
	return out
}

// Vertices lists every vertex once.
func (b *Body) Vertices() []*Vertex {
	seen := map[*Vertex]bool{}
	var out []*Vertex
	add := func(v *Vertex) {
		if v != nil && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	for _, f := range b.Faces() {
		for _, l := range f.Loops {
			add(l.Lone)
			for _, h := range l.HalfEdges() {
				add(h.Start())
			}
		}
	}
	return out
}

// Bounds of the body's vertices and sampled edges.
func (b *Body) Bounds() geom.Box {
	var box geom.Box
	for _, v := range b.Vertices() {
		box.Add(v.P)
	}
	for _, e := range b.Edges() {
		for i := 0; i <= 16; i++ {
			box.Add(e.Curve.Eval(e.T0 + (e.T1-e.T0)*float64(i)/16))
		}
	}
	return box
}

// Counts returns V, E, F, rings (inner loops), shells.
func (b *Body) Counts() (v, e, f, rings, shells int) {
	v = len(b.Vertices())
	e = len(b.Edges())
	for _, fc := range b.Faces() {
		f++
		rings += len(fc.Loops) - 1
	}
	return v, e, f, rings, len(b.Shells)
}
