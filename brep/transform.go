package brep

import (
	"forma/geom"
)

// Transform copies a body moved by x (a rigid motion or a reflection). A
// reflection turns the loops round, so they still run counter-clockwise
// seen from outside. rename, if given, renames the faces.
func Transform(b *Body, x geom.Xform, rename func(string) string) (*Body, error) {
	flip := x.Det() < 0
	bd := NewBuilder(b.Name)
	vmap := map[*Vertex]*Vertex{}
	for _, v := range b.Vertices() {
		vmap[v] = bd.Vertex(x.Pt(v.P))
	}
	emap := map[*Edge]*Edge{}
	for _, e := range b.Edges() {
		ne := &Edge{ID: bd.body.newID(), Curve: x.Curve(e.Curve), T0: e.T0, T1: e.T1,
			Verts: [2]*Vertex{vmap[e.Verts[0]], vmap[e.Verts[1]]}}
		bd.edges = append(bd.edges, ne)
		emap[e] = ne
	}
	for _, f := range b.Faces() {
		var loops [][]Use
		for _, l := range f.Loops {
			hs := l.HalfEdges()
			uses := make([]Use, len(hs))
			for i, h := range hs {
				if flip {
					uses[len(hs)-1-i] = Use{emap[h.Edge], !h.Fwd}
				} else {
					uses[i] = Use{emap[h.Edge], h.Fwd}
				}
			}
			loops = append(loops, uses)
		}
		name := f.Name
		if rename != nil {
			name = rename(name)
		}
		nf := bd.Face(x.Surface(f.Surf), f.Reversed, name, loops...)
		// a loop of a lone vertex (none survive a finished body, but keep them)
		for i, l := range f.Loops {
			if l.Lone != nil {
				nf.Loops[i].Lone = vmap[l.Lone]
			}
		}
	}
	return bd.Finish()
}

// Copy is an exact copy of a body.
func Copy(b *Body) *Body {
	c, err := Transform(b, geom.Identity, nil)
	if err != nil {
		return b
	}
	return c
}

// Gather makes one body of several (each keeping its shells): a tool of
// many disjoint pieces for a single boolean. The bodies mustn't overlap.
func Gather(name string, bodies ...*Body) (*Body, error) {
	bd := NewBuilder(name)
	for _, b := range bodies {
		vmap := map[*Vertex]*Vertex{}
		for _, v := range b.Vertices() {
			vmap[v] = bd.Vertex(v.P)
		}
		emap := map[*Edge]*Edge{}
		for _, e := range b.Edges() {
			ne := &Edge{ID: bd.body.newID(), Curve: e.Curve, T0: e.T0, T1: e.T1,
				Verts: [2]*Vertex{vmap[e.Verts[0]], vmap[e.Verts[1]]}}
			bd.edges = append(bd.edges, ne)
			emap[e] = ne
		}
		for _, f := range b.Faces() {
			var loops [][]Use
			for _, l := range f.Loops {
				hs := l.HalfEdges()
				uses := make([]Use, len(hs))
				for i, h := range hs {
					uses[i] = Use{emap[h.Edge], h.Fwd}
				}
				loops = append(loops, uses)
			}
			nf := bd.Face(f.Surf, f.Reversed, f.Name, loops...)
			for i, l := range f.Loops {
				if l.Lone != nil {
					nf.Loops[i].Lone = vmap[l.Lone]
				}
			}
		}
	}
	return bd.Finish()
}
