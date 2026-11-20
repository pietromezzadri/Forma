package app

import (
	"fmt"
	"math"
	"sort"

	"forma/brep"
	"forma/doc"
	"forma/geom"
	"forma/render"
	"forma/sketch"
)

// Document is the app's view of a design: the doc.Document (the history)
// plus, for each body and sketch it produces, a display item with its mesh
// and GPU copy, reused while the feature's results are unchanged.
type Document struct {
	D        *doc.Document
	Bodies   []*BodyItem
	Sketches []*SketchItem
	Meshes   []*MeshItem // reference meshes
	hist     *doc.History
	bodies   map[int]*BodyItem
	sketches map[int]*SketchItem
	meshes   map[int]*MeshItem
	changed  bool // regenerate before the next frame
	Path     string
	saved    []byte // the design as last saved (for the title's ●)
}

// BodyItem is a body in the view. ID is the feature's ID × 16 + the body's
// index within the feature (stable across regenerations).
type BodyItem struct {
	ID      int
	Name    string
	F       *doc.Feature
	K       int
	Body    *brep.Body
	Mesh    *brep.Mesh
	Visible bool
	gen     int
	gpu     *bodyGPU
	mass    *brep.MassProps // worked out on request
}

// SketchItem is a sketch in the view; ID is the feature's ID × 16.
type SketchItem struct {
	ID      int
	Name    string
	F       *doc.Feature
	S       *sketch.Sketch
	Visible bool
	Regions []sketch.Region
	gpu     *sketchGPU
	dirty   bool
	m       *Document
}

func bodyKey(fid, k int) int { return doc.BodyKey(fid, k) }

// FeatureOf the item ID (body or sketch).
func featureOf(id int) int { return id / doc.KeysPerFeature }

func NewDocument() *Document {
	d := &Document{D: doc.New(), bodies: map[int]*BodyItem{}, sketches: map[int]*SketchItem{}, meshes: map[int]*MeshItem{}}
	d.hist = doc.NewHistory(d.D)
	d.saved = d.D.Marshal()
	d.sync()
	return d
}

// replace swaps in another design (undo, redo, open), dropping GPU copies.
func (d *Document) replace(nd *doc.Document) {
	d.freeAll()
	d.D = nd
	d.bodies, d.sketches, d.meshes = map[int]*BodyItem{}, map[int]*SketchItem{}, map[int]*MeshItem{}
	d.sync()
}

func (d *Document) freeAll() {
	for _, m := range d.meshes {
		m.free()
	}
	for _, b := range d.bodies {
		if b.gpu != nil {
			b.gpu.free()
		}
	}
	for _, s := range d.sketches {
		if s.gpu != nil {
			s.gpu.free()
		}
	}
}

// Regenerate rebuilds what changed and refreshes the view's items.
func (d *Document) Regenerate() {
	d.D.Regenerate()
	d.sync()
	d.changed = false
}

// Commit regenerates and records an undo step.
func (d *Document) Commit() {
	d.Regenerate()
	d.hist.Push(d.D)
}

// Dirty reports changes since the last save.
func (d *Document) Dirty() bool { return string(d.D.Marshal()) != string(d.saved) }

func (d *Document) sync() {
	var bodies []*BodyItem
	var sketches []*SketchItem
	seenB, seenS := map[int]bool{}, map[int]bool{}
	for _, f := range d.D.Features {
		if !f.Active {
			continue
		}
		if f.Kind == doc.KSketch {
			id := bodyKey(f.ID, 0)
			it := d.sketches[id]
			if it == nil || it.F != f {
				if it != nil && it.gpu != nil {
					it.gpu.free()
				}
				it = &SketchItem{ID: id, F: f, m: d}
				d.sketches[id] = it
			}
			it.S = f.Sketch
			it.Name = f.Name
			it.Visible = !f.Hidden
			it.Regions = f.Regions
			it.dirty = true
			sketches = append(sketches, it)
			seenS[id] = true
			continue
		}
	}
	// the bodies as the design stands (joins and cuts applied)
	for _, r := range d.D.Bodies {
		f := d.D.Feature(r.Owner)
		if f == nil {
			continue
		}
		id := r.Key
		it := d.bodies[id]
		if it == nil || it.gen != r.Gen || it.Body != r.Body {
			if it != nil && it.gpu != nil {
				it.gpu.free()
			}
			it = &BodyItem{ID: id, F: f, K: r.Key % doc.KeysPerFeature, Body: r.Body, gen: r.Gen}
			it.Mesh = brep.Tessellate(r.Body, brep.DefaultMeshOptions(r.Body))
			d.bodies[id] = it
		}
		it.F = f
		it.Name = r.Name
		it.Visible = !f.Hidden
		bodies = append(bodies, it)
		seenB[id] = true
	}
	for id, it := range d.bodies {
		if !seenB[id] {
			if it.gpu != nil {
				it.gpu.free()
			}
			delete(d.bodies, id)
		}
	}
	for id, it := range d.sketches {
		if !seenS[id] {
			if it.gpu != nil {
				it.gpu.free()
			}
			delete(d.sketches, id)
		}
	}
	d.Bodies, d.Sketches = bodies, sketches
	d.syncMeshes()
}

func (d *Document) Body(id int) *BodyItem {
	for _, b := range d.Bodies {
		if b.ID == id {
			return b
		}
	}
	return nil
}

func (d *Document) Sketch(id int) *SketchItem {
	for _, s := range d.Sketches {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// AddPrim inserts a primitive feature.
func (d *Document) AddPrim(kind string, inputs map[string]string) (*doc.Feature, error) {
	f := doc.NewPrim(kind)
	for k, v := range inputs {
		f.Inputs[k] = v
	}
	d.D.Insert(f)
	d.Commit()
	if f.Err != "" {
		return f, fmt.Errorf("%s", f.Err)
	}
	return f, nil
}

// AddSketch inserts a sketch feature and returns its item.
func (d *Document) AddSketch(pl doc.PlaneRef) *SketchItem {
	f := d.D.Insert(doc.NewSketch(pl))
	d.Commit()
	return d.Sketch(bodyKey(f.ID, 0))
}

// Remove deletes the feature behind an item (or a feature ID × 16).
func (d *Document) Remove(id int) {
	d.D.Remove(featureOf(id))
	d.Commit()
}

// Bounds of everything visible.
func (d *Document) Bounds() geom.Box {
	var b geom.Box
	for _, it := range d.Bodies {
		if it.Visible && it.Body != nil {
			b.Union(it.Body.Bounds())
		}
	}
	for _, m := range d.Meshes {
		if m.Visible {
			b.Union(doc.MeshBounds(m.Mesh))
		}
	}
	for _, s := range d.Sketches {
		if !s.Visible {
			continue
		}
		sb := s.S.Bounds()
		if sb.Empty() {
			continue
		}
		for _, c := range []geom.Vec3{sb.Min, sb.Max, {X: sb.Min.X, Y: sb.Max.Y}, {X: sb.Max.X, Y: sb.Min.Y}} {
			b.Add(s.S.Plane.At2(geom.Vec2{X: c.X, Y: c.Y}))
		}
	}
	return b
}

type bodyGPU struct {
	faces *render.TriMesh
	edges *render.LineSet
	verts *render.PointSet
}

func (g *bodyGPU) free() {
	g.faces.Delete()
	g.edges.Delete()
	g.verts.Delete()
}

func (b *BodyItem) upload() *bodyGPU {
	if b.gpu != nil || b.Mesh == nil {
		return b.gpu
	}
	m := b.Mesh
	ids := make([]uint32, len(m.Pos))
	for ti, t := range m.Tris {
		for _, k := range t {
			ids[k] = uint32(m.TriFace[ti])
		}
	}
	// sort triangles by face so each face is one range
	order := make([]int, len(m.Tris))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return m.TriFace[order[i]] < m.TriFace[order[j]] })
	tris := make([][3]int32, len(order))
	for i, k := range order {
		tris[i] = m.Tris[k]
	}
	// seams (an edge with the same face on both sides) aren't drawn
	seam := map[int]bool{}
	for _, e := range b.Body.Edges() {
		if e.HE[0] != nil && e.HE[1] != nil && e.HE[0].Loop.Face == e.HE[1].Loop.Face {
			seam[e.ID] = true
		}
	}
	var lines []render.Polyline
	for _, e := range m.Edges {
		if seam[e.ID] {
			continue
		}
		lines = append(lines, render.Polyline{ID: uint32(e.ID), Pts: e.Pts})
	}
	var vpts []geom.Vec3
	var vids []uint32
	for _, v := range m.Verts {
		vpts = append(vpts, v.P)
		vids = append(vids, uint32(v.ID))
	}
	b.gpu = &bodyGPU{
		faces: render.NewTriMesh(m.Pos, m.Nrm, ids, tris),
		edges: render.NewLineSet(lines),
		verts: render.NewPointSet(vpts, vids),
	}
	return b.gpu
}

type sketchGPU struct {
	curves  *render.LineSet // non-construction curves
	constr  *render.LineSet // construction curves (dashed)
	points  *render.PointSet
	regions *render.TriMesh
	// curve colour classes
	fixed map[uint32]bool
}

func (g *sketchGPU) free() {
	g.curves.Delete()
	g.constr.Delete()
	g.points.Delete()
	g.regions.Delete()
}

// markDirty asks for the sketch's GPU copy and regions to be rebuilt.
func (s *SketchItem) markDirty() {
	s.dirty = true
	if s.m != nil {
		s.m.changed = true
	}
}

func (s *SketchItem) upload(chordTol float64) *sketchGPU {
	if !s.dirty && s.gpu != nil {
		return s.gpu
	}
	if s.gpu != nil {
		s.gpu.free()
	}
	s.dirty = false
	sk := s.S
	if s.F != nil && s.F.Regions != nil {
		s.Regions = s.F.Regions
	} else {
		s.Regions = sk.Regions()
	}
	g := &sketchGPU{fixed: map[uint32]bool{}}
	var curves, constr []render.Polyline
	var pts []geom.Vec3
	var pids []uint32
	for _, e := range sk.Ents {
		if e.Kind == sketch.Point {
			pts = append(pts, sk.Plane.At2(sk.P(e.ID)))
			pids = append(pids, uint32(e.ID))
			continue
		}
		if e.Kind == sketch.Text {
			// each letter's outline on its own
			for _, l := range sk.TextContours(e) {
				var p3 []geom.Vec3
				for _, p := range sketch.SegPoints2(l, chordTol) {
					p3 = append(p3, sk.Plane.At2(p))
				}
				if len(p3) > 1 {
					p3 = append(p3, p3[0])
				}
				pl := render.Polyline{ID: uint32(e.ID), Pts: p3}
				if e.Construction {
					constr = append(constr, pl)
				} else {
					curves = append(curves, pl)
				}
			}
			continue
		}
		var p3 []geom.Vec3
		for _, p := range sk.Polyline(e, chordTol) {
			p3 = append(p3, sk.Plane.At2(p))
		}
		pl := render.Polyline{ID: uint32(e.ID), Pts: p3}
		if e.Kind == sketch.Spline {
			// its control legs, drawn like construction lines
			var legs []geom.Vec3
			for _, p := range e.Pts {
				legs = append(legs, sk.Plane.At2(sk.P(p)))
			}
			constr = append(constr, render.Polyline{ID: uint32(e.ID), Pts: legs})
		}
		if e.Construction {
			constr = append(constr, pl)
		} else {
			curves = append(curves, pl)
		}
	}
	g.curves = render.NewLineSet(curves)
	g.constr = render.NewLineSet(constr)
	g.points = render.NewPointSet(pts, pids)

	// regions as flat triangles, ID = region index + 1
	var rp []geom.Vec3
	var rn []geom.Vec3
	var rid []uint32
	var rt [][3]int32
	for ri, r := range s.Regions {
		var pts2 []geom.Vec2
		var segs [][2]int
		addLoop := func(poly []geom.Vec2) {
			start := len(pts2)
			pts2 = append(pts2, poly...)
			for i := range poly {
				j := start + (i+1)%len(poly)
				segs = append(segs, [2]int{start + i, j})
			}
		}
		addLoop(dedupe(r.Poly))
		for _, h := range r.HolePolys {
			addLoop(dedupe(h))
		}
		base := int32(len(rp))
		for _, p := range pts2 {
			rp = append(rp, sk.Plane.At2(p))
			rn = append(rn, sk.Plane.Z)
			rid = append(rid, uint32(ri+1))
		}
		for _, t := range geom.Triangulate(pts2, segs) {
			rt = append(rt, [3]int32{base + int32(t[0]), base + int32(t[1]), base + int32(t[2])})
		}
	}
	g.regions = render.NewTriMesh(rp, rn, rid, rt)
	s.gpu = g
	return g
}

func dedupe(p []geom.Vec2) []geom.Vec2 {
	var out []geom.Vec2
	for _, q := range p {
		if len(out) > 0 && out[len(out)-1].Dist(q) < 1e-9 {
			continue
		}
		out = append(out, q)
	}
	if len(out) > 1 && out[0].Dist(out[len(out)-1]) < 1e-9 {
		out = out[:len(out)-1]
	}
	return out
}

// niceStep rounds a length to 1, 2 or 5 × 10ⁿ.
func niceStep(x float64) float64 {
	if x <= 0 {
		return 1
	}
	e := math.Pow(10, math.Floor(math.Log10(x)))
	switch m := x / e; {
	case m < 1.5:
		return e
	case m < 3.5:
		return 2 * e
	case m < 7.5:
		return 5 * e
	}
	return 10 * e
}
