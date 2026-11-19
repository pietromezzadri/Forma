package brep

import (
	"testing"

	"forma/geom"
)

// A box swept from a lamina with Euler operators alone.
func TestEulerBox(t *testing.T) {
	b, f, v0 := MVFS("box", geom.V3(0, 0, 0))
	l := f.Loops[0]
	_, v1, err := b.MEVLone(l, geom.V3(10, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	h := HalfEdgeFrom(f, v1) // v1 → v0
	_, v2 := b.MEV(h, geom.V3(10, 10, 0))
	h = HalfEdgeFrom(f, v2)
	_, v3 := b.MEV(h, geom.V3(0, 10, 0))
	// close the square: v3 → v0
	h3, h0 := HalfEdgeFrom(f, v3), findFrom(f, v0)
	if _, _, err := b.MEF(h3, h0); err != nil {
		t.Fatal(err)
	}
	if err := CheckTopo(b); err != nil {
		t.Fatalf("lamina: %v", err)
	}
	if v, e, fc, _, _ := b.Counts(); v != 4 || e != 4 || fc != 2 {
		t.Fatalf("lamina V%d E%d F%d", v, e, fc)
	}
	// sweep: a vertical edge up from each corner, then the top edges
	top := b.Faces()[0]
	bottom := []*Vertex{v0, v1, v2, v3}
	var up []*Vertex
	for _, v := range bottom {
		_, nv := b.MEV(HalfEdgeFrom(top, v), v.P.Add(geom.V3(0, 0, 5)))
		up = append(up, nv)
	}
	for i := range up {
		a, c := up[i], up[(i+1)%4]
		ha, hc := HalfEdgeFrom(top, a), HalfEdgeFrom(top, c)
		if ha == nil || hc == nil {
			t.Fatalf("top vertices not in the swept face")
		}
		if _, _, err := b.MEF(ha, hc); err != nil {
			t.Fatal(err)
		}
	}
	if err := CheckTopo(b); err != nil {
		t.Fatal(err)
	}
	v, e, fc, r, s := b.Counts()
	if v != 8 || e != 12 || fc != 6 || r != 0 || s != 1 {
		t.Fatalf("box V%d E%d F%d R%d S%d", v, e, fc, r, s)
	}
	if g := Genus(b); g != 0 {
		t.Fatalf("genus %d", g)
	}
}

func findFrom(f *Face, v *Vertex) *HalfEdge {
	for _, l := range f.Loops {
		for _, h := range l.HalfEdges() {
			if h.Start() == v {
				return h
			}
		}
	}
	return nil
}

// A ring made with KEMR: a square face with a square hole in it.
func TestEulerRing(t *testing.T) {
	b, f, v0 := MVFS("plate", geom.V3(0, 0, 0))
	_, v1, _ := b.MEVLone(f.Loops[0], geom.V3(10, 0, 0))
	_, v2 := b.MEV(HalfEdgeFrom(f, v1), geom.V3(10, 10, 0))
	_, v3 := b.MEV(HalfEdgeFrom(f, v2), geom.V3(0, 10, 0))
	_, _, _ = b.MEF(HalfEdgeFrom(f, v3), findFrom(f, v0))
	front := f
	// a bridge from v0 inward and the hole's square
	bridge, w0 := b.MEV(HalfEdgeFrom(front, v0), geom.V3(3, 3, 0))
	_, w1 := b.MEV(HalfEdgeFrom(front, w0), geom.V3(7, 3, 0))
	_, w2 := b.MEV(HalfEdgeFrom(front, w1), geom.V3(7, 7, 0))
	_, w3 := b.MEV(HalfEdgeFrom(front, w2), geom.V3(3, 7, 0))
	if _, _, err := b.MEF(HalfEdgeFrom(front, w3), findInLoopFrom(front, w0, w1)); err != nil {
		t.Fatal(err)
	}
	if err := CheckTopo(b); err != nil {
		t.Fatal(err)
	}
	if _, err := b.KEMR(bridge); err != nil {
		t.Fatal(err)
	}
	if err := CheckTopo(b); err != nil {
		t.Fatal(err)
	}
	v, e, fc, r, _ := b.Counts()
	if v != 8 || e != 8 || fc != 3 || r != 1 {
		t.Fatalf("V%d E%d F%d R%d", v, e, fc, r)
	}
}

// findInLoopFrom finds the half-edge of f leaving a toward b.
func findInLoopFrom(f *Face, a, b *Vertex) *HalfEdge {
	for _, l := range f.Loops {
		for _, h := range l.HalfEdges() {
			if h.Start() == a && h.End() == b {
				return h
			}
		}
	}
	return nil
}
