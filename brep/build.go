package brep

import (
	"fmt"

	"forma/geom"
)

// Builder assembles a body from vertices, edges and faces given as loops of
// edge uses. Every edge must end up used exactly twice, once each way.
type Builder struct {
	body  *Body
	shell *Shell
	edges []*Edge
	err   error
}

// Use is one traversal of an edge in a loop.
type Use struct {
	E   *Edge
	Fwd bool
}

func F(e *Edge) Use { return Use{e, true} }
func R(e *Edge) Use { return Use{e, false} }

func NewBuilder(name string) *Builder {
	b := &Body{Name: name}
	s := &Shell{ID: b.newID()}
	b.Shells = []*Shell{s}
	return &Builder{body: b, shell: s}
}

func (b *Builder) Vertex(p geom.Vec3) *Vertex {
	return &Vertex{ID: b.body.newID(), P: p}
}

// Edge makes an edge along c from v0 (at t0) to v1 (at t1).
func (b *Builder) Edge(v0, v1 *Vertex, c geom.Curve, t0, t1 float64) *Edge {
	e := &Edge{ID: b.body.newID(), Curve: c, T0: t0, T1: t1, Verts: [2]*Vertex{v0, v1}}
	if b.err == nil {
		if d := c.Eval(t0).Dist(v0.P); d > 1e-5 {
			b.err = fmt.Errorf("edge %d: start %.3g mm off its vertex", e.ID, d)
		} else if d := c.Eval(t1).Dist(v1.P); d > 1e-5 {
			b.err = fmt.Errorf("edge %d: end %.3g mm off its vertex", e.ID, d)
		}
	}
	b.edges = append(b.edges, e)
	return e
}

// LineEdge is a straight edge between two vertices.
func (b *Builder) LineEdge(v0, v1 *Vertex) *Edge {
	l, n := geom.LineThrough(v0.P, v1.P)
	return b.Edge(v0, v1, l, 0, n)
}

// Face adds a face on surf; loops[0] is the outer loop.
func (b *Builder) Face(surf geom.Surface, reversed bool, name string, loops ...[]Use) *Face {
	f := &Face{ID: b.body.newID(), Surf: surf, Reversed: reversed, Shell: b.shell, Name: name}
	for _, uses := range loops {
		l := &Loop{ID: b.body.newID(), Face: f}
		var prev *HalfEdge
		for i, u := range uses {
			h := &HalfEdge{Edge: u.E, Loop: l, Fwd: u.Fwd}
			slot := 1
			if u.Fwd {
				slot = 0
			}
			if u.E.HE[slot] != nil && b.err == nil {
				b.err = fmt.Errorf("face %q: edge %d used twice the same way", name, u.E.ID)
			}
			u.E.HE[slot] = h
			if i == 0 {
				l.First = h
			} else {
				prev.Next, h.Prev = h, prev
			}
			prev = h
		}
		if prev != nil {
			prev.Next, l.First.Prev = l.First, prev
		}
		f.Loops = append(f.Loops, l)
	}
	b.shell.Faces = append(b.shell.Faces, f)
	return f
}

// Finish checks the body and returns it.
func (b *Builder) Finish() (*Body, error) {
	if b.err != nil {
		return nil, b.err
	}
	for _, e := range b.edges {
		if e.HE[0] == nil || e.HE[1] == nil {
			return nil, fmt.Errorf("edge %d is not used both ways", e.ID)
		}
	}
	b.splitShells()
	if err := Check(b.body); err != nil {
		return nil, err
	}
	return b.body, nil
}

// splitShells puts each connected set of faces in a shell of its own.
func (b *Builder) splitShells() {
	faces := b.shell.Faces
	parent := make([]int, len(faces))
	idx := map[*Face]int{}
	for i, f := range faces {
		parent[i] = i
		idx[f] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for _, e := range b.edges {
		if e.HE[0] == nil || e.HE[1] == nil {
			continue
		}
		a, c := find(idx[e.HE[0].Loop.Face]), find(idx[e.HE[1].Loop.Face])
		parent[a] = c
	}
	groups := map[int]*Shell{}
	var shells []*Shell
	for i, f := range faces {
		r := find(i)
		s := groups[r]
		if s == nil {
			s = &Shell{ID: b.body.newID()}
			if len(shells) == 0 {
				s = b.shell
				s.Faces = nil
			}
			groups[r] = s
			shells = append(shells, s)
		}
		s.Faces = append(s.Faces, f)
		f.Shell = s
	}
	b.body.Shells = shells
}
