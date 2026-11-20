package brep

import (
	"fmt"

	"forma/geom"
)

// Scale copies a body scaled by s (> 0) about c: every length times s,
// radii included, angles kept. Edge parameters that are lengths (lines,
// polylines) scale with it.
func Scale(b *Body, c geom.Vec3, s float64) (*Body, error) {
	if s <= 0 {
		return nil, fmt.Errorf("scale must be more than 0")
	}
	if s == 1 {
		return Copy(b), nil
	}
	pt := func(p geom.Vec3) geom.Vec3 { return c.Add(p.Sub(c).Scale(s)) }
	fr := func(f geom.Frame) geom.Frame { f.O = pt(f.O); return f }
	bd := NewBuilder(b.Name)
	vmap := map[*Vertex]*Vertex{}
	for _, v := range b.Vertices() {
		vmap[v] = bd.Vertex(pt(v.P))
	}
	emap := map[*Edge]*Edge{}
	for _, e := range b.Edges() {
		cv, t0, t1 := e.Curve, e.T0, e.T1
		switch k := e.Curve.(type) {
		case geom.Line:
			cv, t0, t1 = geom.Line{P: pt(k.P), D: k.D}, t0*s, t1*s
		case geom.Circle:
			cv = geom.Circle{F: fr(k.F), R: k.R * s}
		case geom.Ellipse:
			cv = geom.Ellipse{F: fr(k.F), A: k.A * s, B: k.B * s, P: k.P}
		case *geom.BSpline:
			cv = k.Map(pt)
		case *geom.Polyline:
			pts := make([]geom.Vec3, len(k.Pts))
			for i, p := range k.Pts {
				pts[i] = pt(p)
			}
			cv, t0, t1 = geom.NewPolyline(pts, k.Tol*s), t0*s, t1*s
		default:
			return nil, fmt.Errorf("can't scale a %s edge", e.Curve.Kind())
		}
		ne := &Edge{ID: bd.body.newID(), Curve: cv, T0: t0, T1: t1,
			Verts: [2]*Vertex{vmap[e.Verts[0]], vmap[e.Verts[1]]}}
		bd.edges = append(bd.edges, ne)
		emap[e] = ne
	}
	for _, f := range b.Faces() {
		var sf geom.Surface
		switch k := f.Surf.(type) {
		case geom.Plane:
			sf = geom.Plane{F: fr(k.F)}
		case geom.Cylinder:
			sf = geom.Cylinder{F: fr(k.F), R: k.R * s}
		case geom.Cone:
			sf = geom.Cone{F: fr(k.F), R: k.R * s, A: k.A}
		case geom.Sphere:
			sf = geom.Sphere{F: fr(k.F), R: k.R * s}
		case geom.Torus:
			sf = geom.Torus{F: fr(k.F), R: k.R * s, Rm: k.Rm * s}
		case geom.Revolved:
			sf = geom.Revolved{F: fr(k.F), C: k.C.Map(func(p geom.Vec3) geom.Vec3 { return p.Scale(s) })}
		case geom.Extruded:
			sf = geom.Extruded{C: k.C.Map(pt), D: k.D}
		default:
			return nil, fmt.Errorf("can't scale a %s face", f.Surf.Kind())
		}
		var loops [][]Use
		for _, l := range f.Loops {
			hs := l.HalfEdges()
			uses := make([]Use, len(hs))
			for i, h := range hs {
				uses[i] = Use{emap[h.Edge], h.Fwd}
			}
			loops = append(loops, uses)
		}
		nf := bd.Face(sf, f.Reversed, f.Name, loops...)
		for i, l := range f.Loops {
			if l.Lone != nil {
				nf.Loops[i].Lone = vmap[l.Lone]
			}
		}
	}
	return bd.Finish()
}
