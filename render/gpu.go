package render

import (
	"github.com/go-gl/gl/v4.1-core/gl"

	"forma/geom"
)

// Range is a span of indices (triangles) or vertices (lines, points) that
// belongs to one ID.
type Range struct{ First, Count int32 }

// TriMesh is an indexed triangle mesh on the GPU with a normal and an ID per
// vertex.
type TriMesh struct {
	vao, vbo, ebo uint32
	count         int32
	Ranges        map[uint32]Range
}

// NewTriMesh uploads triangles. ids gives each vertex's ID; triangles of one
// ID should be contiguous for Ranges to be useful.
func NewTriMesh(pos, nrm []geom.Vec3, ids []uint32, tris [][3]int32) *TriMesh {
	m := &TriMesh{Ranges: map[uint32]Range{}}
	data := make([]float32, 0, len(pos)*7)
	for i, p := range pos {
		n := geom.ZAxis
		if i < len(nrm) {
			n = nrm[i]
		}
		data = append(data, float32(p.X), float32(p.Y), float32(p.Z), float32(n.X), float32(n.Y), float32(n.Z), 0)
	}
	idx := make([]uint32, 0, len(tris)*3)
	for ti, t := range tris {
		id := ids[t[0]]
		r, ok := m.Ranges[id]
		if !ok {
			r.First = int32(ti * 3)
		}
		r.Count = int32(ti*3+3) - r.First
		m.Ranges[id] = r
		idx = append(idx, uint32(t[0]), uint32(t[1]), uint32(t[2]))
	}
	gl.GenVertexArrays(1, &m.vao)
	gl.BindVertexArray(m.vao)
	gl.GenBuffers(1, &m.vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, m.vbo)
	// pack the uint ID into the 7th float's bits
	for i := range pos {
		data[i*7+6] = uintBits(ids[i])
	}
	if len(data) > 0 {
		gl.BufferData(gl.ARRAY_BUFFER, len(data)*4, gl.Ptr(data), gl.STATIC_DRAW)
	}
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(0, 3, gl.FLOAT, false, 28, 0)
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointerWithOffset(1, 3, gl.FLOAT, false, 28, 12)
	gl.EnableVertexAttribArray(2)
	gl.VertexAttribIPointerWithOffset(2, 1, gl.UNSIGNED_INT, 28, 24)
	gl.GenBuffers(1, &m.ebo)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, m.ebo)
	if len(idx) > 0 {
		gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(idx)*4, gl.Ptr(idx), gl.STATIC_DRAW)
	}
	gl.BindVertexArray(0)
	m.count = int32(len(idx))
	return m
}

func (m *TriMesh) Draw() {
	if m == nil || m.count == 0 {
		return
	}
	gl.BindVertexArray(m.vao)
	gl.DrawElementsWithOffset(gl.TRIANGLES, m.count, gl.UNSIGNED_INT, 0)
}

// DrawRange draws only the triangles of one ID.
func (m *TriMesh) DrawRange(id uint32) {
	if m == nil {
		return
	}
	r, ok := m.Ranges[id]
	if !ok {
		return
	}
	gl.BindVertexArray(m.vao)
	gl.DrawElementsWithOffset(gl.TRIANGLES, r.Count, gl.UNSIGNED_INT, uintptr(r.First)*4)
}

func (m *TriMesh) Delete() {
	if m == nil {
		return
	}
	gl.DeleteVertexArrays(1, &m.vao)
	gl.DeleteBuffers(1, &m.vbo)
	gl.DeleteBuffers(1, &m.ebo)
}

// Polyline is a run of points with one ID.
type Polyline struct {
	ID  uint32
	Pts []geom.Vec3
}

// LineSet is polylines drawn as GL_LINES pairs, with a running length for dashes.
type LineSet struct {
	vao, vbo uint32
	count    int32
	Ranges   map[uint32]Range
}

func NewLineSet(lines []Polyline) *LineSet {
	s := &LineSet{Ranges: map[uint32]Range{}}
	var data []float32
	n := int32(0)
	for _, l := range lines {
		r := Range{First: n}
		dist := 0.0
		for i := 0; i+1 < len(l.Pts); i++ {
			a, b := l.Pts[i], l.Pts[i+1]
			d2 := dist + a.Dist(b)
			data = append(data,
				float32(a.X), float32(a.Y), float32(a.Z), uintBits(l.ID), float32(dist),
				float32(b.X), float32(b.Y), float32(b.Z), uintBits(l.ID), float32(d2))
			dist = d2
			n += 2
		}
		r.Count = n - r.First
		if old, ok := s.Ranges[l.ID]; ok {
			// ids with several polylines: keep the span covering them (rare)
			if old.First < r.First {
				r.Count += r.First - old.First
				r.First = old.First
			}
		}
		s.Ranges[l.ID] = r
	}
	s.count = n
	gl.GenVertexArrays(1, &s.vao)
	gl.BindVertexArray(s.vao)
	gl.GenBuffers(1, &s.vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, s.vbo)
	if len(data) > 0 {
		gl.BufferData(gl.ARRAY_BUFFER, len(data)*4, gl.Ptr(data), gl.STATIC_DRAW)
	}
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(0, 3, gl.FLOAT, false, 20, 0)
	gl.EnableVertexAttribArray(2)
	gl.VertexAttribIPointerWithOffset(2, 1, gl.UNSIGNED_INT, 20, 12)
	gl.EnableVertexAttribArray(3)
	gl.VertexAttribPointerWithOffset(3, 1, gl.FLOAT, false, 20, 16)
	gl.BindVertexArray(0)
	return s
}

func (s *LineSet) Draw() {
	if s == nil || s.count == 0 {
		return
	}
	gl.BindVertexArray(s.vao)
	gl.DrawArrays(gl.LINES, 0, s.count)
}

func (s *LineSet) DrawRange(id uint32) {
	if s == nil {
		return
	}
	if r, ok := s.Ranges[id]; ok {
		gl.BindVertexArray(s.vao)
		gl.DrawArrays(gl.LINES, r.First, r.Count)
	}
}

func (s *LineSet) Delete() {
	if s == nil {
		return
	}
	gl.DeleteVertexArrays(1, &s.vao)
	gl.DeleteBuffers(1, &s.vbo)
}

// PointSet is points with an ID each.
type PointSet struct {
	vao, vbo uint32
	count    int32
	Index    map[uint32]int32
}

func NewPointSet(pts []geom.Vec3, ids []uint32) *PointSet {
	s := &PointSet{Index: map[uint32]int32{}}
	var data []float32
	for i, p := range pts {
		data = append(data, float32(p.X), float32(p.Y), float32(p.Z), uintBits(ids[i]))
		s.Index[ids[i]] = int32(i)
	}
	s.count = int32(len(pts))
	gl.GenVertexArrays(1, &s.vao)
	gl.BindVertexArray(s.vao)
	gl.GenBuffers(1, &s.vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, s.vbo)
	if len(data) > 0 {
		gl.BufferData(gl.ARRAY_BUFFER, len(data)*4, gl.Ptr(data), gl.STATIC_DRAW)
	}
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(0, 3, gl.FLOAT, false, 16, 0)
	gl.EnableVertexAttribArray(2)
	gl.VertexAttribIPointerWithOffset(2, 1, gl.UNSIGNED_INT, 16, 12)
	gl.BindVertexArray(0)
	return s
}

func (s *PointSet) Draw() {
	if s == nil || s.count == 0 {
		return
	}
	gl.BindVertexArray(s.vao)
	gl.DrawArrays(gl.POINTS, 0, s.count)
}

func (s *PointSet) DrawOne(id uint32) {
	if s == nil {
		return
	}
	if i, ok := s.Index[id]; ok {
		gl.BindVertexArray(s.vao)
		gl.DrawArrays(gl.POINTS, i, 1)
	}
}

func (s *PointSet) Delete() {
	if s == nil {
		return
	}
	gl.DeleteVertexArrays(1, &s.vao)
	gl.DeleteBuffers(1, &s.vbo)
}
