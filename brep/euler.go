package brep

import (
	"fmt"

	"forma/geom"
)

// Euler operators: local edits that keep V − E + F − R = 2(S − G) true.
// New edges are straight lines and new faces have no surface; callers give
// them geometry afterwards (SetLine, Face.Surf).

// MVFS makes a body of one vertex, one face and one shell.
func MVFS(name string, p geom.Vec3) (*Body, *Face, *Vertex) {
	b := &Body{Name: name}
	s := &Shell{ID: b.newID()}
	b.Shells = []*Shell{s}
	v := &Vertex{ID: b.newID(), P: p}
	f := &Face{ID: b.newID(), Shell: s}
	f.Loops = []*Loop{{ID: b.newID(), Face: f, Lone: v}}
	s.Faces = []*Face{f}
	return b, f, v
}

func (b *Body) newEdge(v0, v1 *Vertex) *Edge {
	e := &Edge{ID: b.newID(), Verts: [2]*Vertex{v0, v1}}
	e.SetLine()
	return e
}

// SetLine makes e a straight line between its vertices.
func (e *Edge) SetLine() {
	l, n := geom.LineThrough(e.Verts[0].P, e.Verts[1].P)
	e.Curve, e.T0, e.T1 = l, 0, n
}

// MEVLone grows the first edge out of an edgeless loop's vertex.
func (b *Body) MEVLone(l *Loop, p geom.Vec3) (*Edge, *Vertex, error) {
	if l.First != nil || l.Lone == nil {
		return nil, nil, fmt.Errorf("MEVLone: loop %d has edges", l.ID)
	}
	nv := &Vertex{ID: b.newID(), P: p}
	e := b.newEdge(l.Lone, nv)
	h1 := &HalfEdge{Edge: e, Loop: l, Fwd: true}
	h2 := &HalfEdge{Edge: e, Loop: l, Fwd: false}
	h1.Next, h1.Prev, h2.Next, h2.Prev = h2, h2, h1, h1
	e.HE = [2]*HalfEdge{h1, h2}
	l.First, l.Lone = h1, nil
	return e, nv, nil
}

// MEV grows a new edge to a new vertex at p from h's start vertex, inserted
// in h's loop just before h (a spur: in along the edge, back along it).
func (b *Body) MEV(h *HalfEdge, p geom.Vec3) (*Edge, *Vertex) {
	v := h.Start()
	nv := &Vertex{ID: b.newID(), P: p}
	e := b.newEdge(v, nv)
	l := h.Loop
	h1 := &HalfEdge{Edge: e, Loop: l, Fwd: true}  // v → nv
	h2 := &HalfEdge{Edge: e, Loop: l, Fwd: false} // nv → v
	a := h.Prev
	a.Next, h1.Prev = h1, a
	h1.Next, h2.Prev = h2, h1
	h2.Next, h.Prev = h, h2
	e.HE = [2]*HalfEdge{h1, h2}
	return e, nv
}

// MEF joins h1's start to h2's start (both in one loop) with a new edge,
// splitting the loop in two: the part holding h1 becomes a new face.
func (b *Body) MEF(h1, h2 *HalfEdge) (*Edge, *Face, error) {
	if h1.Loop != h2.Loop {
		return nil, nil, fmt.Errorf("MEF: half-edges in different loops")
	}
	if h1 == h2 {
		return nil, nil, fmt.Errorf("MEF: same half-edge")
	}
	l := h1.Loop
	v1, v2 := h1.Start(), h2.Start()
	e := b.newEdge(v1, v2)
	nh1 := &HalfEdge{Edge: e, Fwd: true}  // v1 → v2
	nh2 := &HalfEdge{Edge: e, Fwd: false} // v2 → v1
	a, c := h1.Prev, h2.Prev
	a.Next, nh1.Prev = nh1, a
	nh1.Next, h2.Prev = h2, nh1
	c.Next, nh2.Prev = nh2, c
	nh2.Next, h1.Prev = h1, nh2
	e.HE = [2]*HalfEdge{nh1, nh2}

	old := l.Face
	nf := &Face{ID: b.newID(), Shell: old.Shell}
	nl := &Loop{ID: b.newID(), Face: nf, First: nh2}
	nf.Loops = []*Loop{nl}
	old.Shell.Faces = append(old.Shell.Faces, nf)
	l.First = nh1
	for _, h := range l.HalfEdges() {
		h.Loop = l
	}
	for _, h := range nl.HalfEdges() {
		h.Loop = nl
	}
	return e, nf, nil
}

// KEMR removes an edge whose two halves lie in the same loop, splitting the
// loop into an outer part and a ring (inner loop) of the same face, which it
// returns. The ring is the part that followed e's forward half.
func (b *Body) KEMR(e *Edge) (*Loop, error) {
	he, tw := e.HE[0], e.HE[1]
	if he.Loop != tw.Loop {
		return nil, fmt.Errorf("KEMR: edge %d's halves are in different loops", e.ID)
	}
	l := he.Loop
	f := l.Face
	a, c := he.Prev, he.Next
	bb, d := tw.Prev, tw.Next
	ring := &Loop{ID: b.newID(), Face: f}
	// ring: from c to bb (the part between he and tw)
	if c == tw {
		ring.Lone = he.End()
	} else {
		bb.Next, c.Prev = c, bb
		ring.First = c
	}
	// outer: a … d
	if d == he {
		l.First, l.Lone = nil, he.Start()
	} else {
		a.Next, d.Prev = d, a
		l.First = d
	}
	for _, h := range ring.HalfEdges() {
		h.Loop = ring
	}
	f.Loops = append(f.Loops, ring)
	e.HE = [2]*HalfEdge{}
	return ring, nil
}

// KFMRH kills face hole (one loop) and makes its loop a ring of face outer —
// a hole through the body, or a second boundary of a face.
func (b *Body) KFMRH(outer, hole *Face) error {
	if len(hole.Loops) != 1 {
		return fmt.Errorf("KFMRH: face %d has %d loops", hole.ID, len(hole.Loops))
	}
	l := hole.Loops[0]
	l.Face = outer
	outer.Loops = append(outer.Loops, l)
	s := hole.Shell
	for i, f := range s.Faces {
		if f == hole {
			s.Faces = append(s.Faces[:i], s.Faces[i+1:]...)
			break
		}
	}
	return nil
}

// HalfEdgeFrom finds a half-edge of face f leaving vertex v.
func HalfEdgeFrom(f *Face, v *Vertex) *HalfEdge {
	for _, l := range f.Loops {
		for _, h := range l.HalfEdges() {
			if h.Start() == v {
				return h
			}
		}
	}
	return nil
}
