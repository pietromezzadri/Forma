package render

// Meshes coloured per vertex (a stress or displacement plot), lit like the
// bodies.

import (
	"github.com/go-gl/gl/v4.1-core/gl"

	"forma/geom"
)

const colorVert = `#version 410 core
layout(location = 0) in vec3 aPos;
layout(location = 1) in vec3 aNrm;
layout(location = 3) in vec3 aCol;
uniform mat4 uView;
uniform mat4 uProj;
uniform vec4 uClip;
out vec3 vNrm;
out vec3 vViewPos;
out vec3 vCol;
void main() {
	gl_ClipDistance[0] = dot(uClip.xyz, aPos) + uClip.w;
	vec4 vp = uView * vec4(aPos, 1.0);
	vViewPos = vp.xyz;
	vNrm = mat3(uView) * aNrm;
	vCol = aCol;
	gl_Position = uProj * vp;
}`

const colorFrag = `#version 410 core
in vec3 vNrm;
in vec3 vViewPos;
in vec3 vCol;
uniform vec3 uKeyDir;
uniform int uOrtho;
out vec4 oColor;
void main() {
	vec3 n = normalize(vNrm);
	vec3 v = uOrtho == 1 ? vec3(0, 0, 1) : normalize(-vViewPos);
	if (!gl_FrontFacing) n = -n;
	vec3 base = pow(vCol, vec3(2.2)); // the plot's colours are given in sRGB
	float key = max(dot(n, normalize(uKeyDir)), 0.0);
	float head = max(dot(n, v), 0.0);
	vec3 c = base * (0.42 + 0.33 * key + 0.30 * head);
	oColor = vec4(pow(clamp(c, 0.0, 1.0), vec3(1.0 / 2.2)), 1.0);
}`

// ColorMesh is an indexed triangle mesh with a normal and a colour per
// vertex.
type ColorMesh struct {
	vao, vbo, ebo uint32
	count         int32
}

// NewColorMesh uploads triangles with per-vertex colours (sRGB, 0..1).
func NewColorMesh(pos, nrm []geom.Vec3, col [][3]float32, tris [][3]int32) *ColorMesh {
	m := &ColorMesh{}
	data := make([]float32, 0, len(pos)*9)
	for i, p := range pos {
		n, c := nrm[i], col[i]
		data = append(data, float32(p.X), float32(p.Y), float32(p.Z), float32(n.X), float32(n.Y), float32(n.Z), c[0], c[1], c[2])
	}
	idx := make([]uint32, 0, len(tris)*3)
	for _, t := range tris {
		idx = append(idx, uint32(t[0]), uint32(t[1]), uint32(t[2]))
	}
	gl.GenVertexArrays(1, &m.vao)
	gl.BindVertexArray(m.vao)
	gl.GenBuffers(1, &m.vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, m.vbo)
	if len(data) > 0 {
		gl.BufferData(gl.ARRAY_BUFFER, len(data)*4, gl.Ptr(data), gl.STATIC_DRAW)
	}
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(0, 3, gl.FLOAT, false, 36, 0)
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointerWithOffset(1, 3, gl.FLOAT, false, 36, 12)
	gl.EnableVertexAttribArray(3)
	gl.VertexAttribPointerWithOffset(3, 3, gl.FLOAT, false, 36, 24)
	gl.GenBuffers(1, &m.ebo)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, m.ebo)
	if len(idx) > 0 {
		gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(idx)*4, gl.Ptr(idx), gl.STATIC_DRAW)
	}
	gl.BindVertexArray(0)
	m.count = int32(len(idx))
	return m
}

// Free releases the mesh's buffers.
func (m *ColorMesh) Free() {
	if m == nil {
		return
	}
	gl.DeleteBuffers(1, &m.vbo)
	gl.DeleteBuffers(1, &m.ebo)
	gl.DeleteVertexArrays(1, &m.vao)
	m.count = 0
}

var colorShader *Shader

// DrawColored draws a per-vertex coloured mesh, lit.
func (r *Renderer) DrawColored(v View, m *ColorMesh) error {
	if m == nil || m.count == 0 {
		return nil
	}
	if colorShader == nil {
		s, err := NewShader(colorVert, "", colorFrag)
		if err != nil {
			return err
		}
		colorShader = s
	}
	s := colorShader
	s.Use()
	s.Mat4("uView", v.View)
	s.Mat4("uProj", v.Proj)
	clip := v.Clip
	if !v.Section {
		clip[0], clip[1], clip[2], clip[3] = 0, 0, 0, 1
	}
	s.Vec4("uClip", clip)
	s.Vec3("uKeyDir", v.KeyDir)
	o := int32(0)
	if v.Ortho {
		o = 1
	}
	s.Int("uOrtho", o)
	gl.Enable(gl.POLYGON_OFFSET_FILL)
	gl.PolygonOffset(1, 1)
	gl.BindVertexArray(m.vao)
	gl.DrawElementsWithOffset(gl.TRIANGLES, m.count, gl.UNSIGNED_INT, 0)
	gl.Disable(gl.POLYGON_OFFSET_FILL)
	return nil
}
