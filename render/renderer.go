package render

import (
	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/go-gl/mathgl/mgl32"

	"forma/geom"
)

// View is one frame's camera as the shaders see it.
type View struct {
	View, Proj mgl32.Mat4
	W, H       int
	Ortho      bool
	Eye        geom.Vec3
	KeyDir     mgl32.Vec3 // view space, toward the key light
	// Clip is a section plane (kept where n·p + w ≥ 0); the zero value
	// keeps everything
	Clip    mgl32.Vec4
	Section bool
}

// Paint is how a draw call colours what it draws.
type Paint struct {
	Color      mgl32.Vec4
	HoverColor mgl32.Vec4
	Hover      uint32 // local ID drawn in HoverColor (0: none)
	Width      float32
	Dash       float32    // world units per dash period, 0 = solid
	DepthBias  float32    // clip-space z pulled toward the camera
	IDBase     uint32     // OR'ed into IDs in the pick pass
	OnTop      bool       // drawn over everything (no depth test)
	NoDepth    bool       // faces: drawn without writing depth (see-through: what is behind still shows)
	CullBack   bool       // faces: their backs left out (a see-through body's far side isn't drawn red)
	Behind     bool       // faces: drawn at the far plane, so only where nothing else is (a see-through body in the ID buffer)
	Offset     mgl32.Vec3 // moves what is drawn (a component in an exploded view)
}

type Renderer struct {
	face, facePick   *Shader
	line, linePick   *Shader
	point, pointPick *Shader
	flat, flatPick   *Shader
	grid, bg         *Shader
	emptyVAO         uint32
	Color            *ColorTarget
	Pick             *PickTarget
}

const bgFrag = `#version 410 core
in vec2 vNDC;
uniform vec4 uTop;
uniform vec4 uBottom;
out vec4 oColor;
void main() {
	float t = vNDC.y * 0.5 + 0.5;
	oColor = mix(uBottom, uTop, t * t * (3.0 - 2.0 * t));
	gl_FragDepth = 1.0;
}`

func NewRenderer() (*Renderer, error) {
	r := &Renderer{Color: NewColorTarget(4), Pick: NewPickTarget()}
	var err error
	mk := func(dst **Shader, v, g, f string, defs ...string) {
		if err != nil {
			return
		}
		*dst, err = NewShader(v, g, f, defs...)
	}
	mk(&r.face, faceVert, "", faceFrag)
	mk(&r.facePick, faceVert, "", faceFrag, "PICK")
	mk(&r.line, lineVert, lineGeom, lineFrag)
	mk(&r.linePick, lineVert, lineGeom, lineFrag, "PICK")
	mk(&r.point, lineVert, pointGeom, pointFrag)
	mk(&r.pointPick, lineVert, pointGeom, pointFrag, "PICK")
	mk(&r.flat, flatVert, "", flatFrag)
	mk(&r.flatPick, flatVert, "", flatFrag, "PICK")
	mk(&r.grid, gridVert, "", gridFrag)
	mk(&r.bg, gridVert, "", bgFrag)
	if err != nil {
		return nil, err
	}
	gl.GenVertexArrays(1, &r.emptyVAO)
	return r, nil
}

// Resize both targets.
func (r *Renderer) Resize(w, h int) {
	r.Color.Resize(w, h)
	r.Pick.Resize(w, h)
}

// BeginColor binds the colour target and paints the background gradient.
func (r *Renderer) BeginColor(top, bottom mgl32.Vec4) {
	r.Color.Bind()
	gl.ClearColor(bottom[0], bottom[1], bottom[2], 1)
	gl.ClearDepth(1)
	gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
	gl.Disable(gl.DEPTH_TEST)
	gl.Disable(gl.BLEND)
	gl.Disable(gl.CULL_FACE)
	gl.Disable(gl.SCISSOR_TEST)
	r.bg.Use()
	r.bg.Vec4("uTop", top)
	r.bg.Vec4("uBottom", bottom)
	gl.BindVertexArray(r.emptyVAO)
	gl.DrawArrays(gl.TRIANGLES, 0, 3)
	gl.Enable(gl.DEPTH_TEST)
	gl.DepthFunc(gl.LEQUAL)
	gl.Enable(gl.CLIP_DISTANCE0)
	gl.Enable(gl.BLEND)
	gl.BlendFuncSeparate(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA, gl.ONE, gl.ONE_MINUS_SRC_ALPHA)
}

// EndColor resolves the frame into the texture.
func (r *Renderer) EndColor() {
	gl.Disable(gl.BLEND)
	gl.Disable(gl.CLIP_DISTANCE0)
	r.Color.Resolve()
}

// BeginPick binds the ID buffer.
func (r *Renderer) BeginPick() {
	r.Pick.Bind()
	gl.Enable(gl.CLIP_DISTANCE0)
	gl.Enable(gl.DEPTH_TEST)
	gl.DepthFunc(gl.LEQUAL)
	gl.Disable(gl.BLEND)
	gl.Disable(gl.CULL_FACE)
	gl.Disable(gl.SCISSOR_TEST)
}

func (r *Renderer) EndPick() {
	gl.Disable(gl.CLIP_DISTANCE0)
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
}

func (r *Renderer) common(s *Shader, v View, p Paint, pick bool) {
	s.Use()
	s.Mat4("uView", v.View)
	s.Mat4("uProj", v.Proj)
	clip := v.Clip
	if !v.Section {
		clip = mgl32.Vec4{0, 0, 0, 1}
	}
	s.Vec4("uClip", clip)
	s.Vec3("uOffset", p.Offset)
	if pick {
		s.Uint("uIDBase", p.IDBase)
		return
	}
	s.Vec4("uColor", p.Color)
	s.Vec4("uHoverColor", p.HoverColor)
	s.Uint("uHover", p.Hover)
}

func pickOr(pick bool, a, b *Shader) *Shader {
	if pick {
		return b
	}
	return a
}

func (r *Renderer) faceSetup(v View, p Paint, pick bool) {
	s := pickOr(pick, r.face, r.facePick)
	r.common(s, v, p, pick)
	if !pick {
		s.Vec3("uKeyDir", v.KeyDir)
		o := int32(0)
		if v.Ortho {
			o = 1
		}
		s.Int("uOrtho", o)
		sec := int32(0)
		if v.Section {
			sec = 1
		}
		s.Int("uSection", sec)
	}
	gl.Enable(gl.POLYGON_OFFSET_FILL)
	gl.PolygonOffset(1, 1)
}

// DrawFaces draws shaded triangles (or their IDs).
func (r *Renderer) DrawFaces(v View, m *TriMesh, p Paint, pick bool) {
	r.faceSetup(v, p, pick)
	if p.OnTop {
		gl.Disable(gl.DEPTH_TEST)
	}
	if p.NoDepth {
		gl.DepthMask(false)
	}
	if p.CullBack {
		gl.Enable(gl.CULL_FACE)
		gl.CullFace(gl.BACK)
	}
	if p.Behind {
		gl.DepthRange(1, 1)
	}
	m.Draw()
	if p.Behind {
		gl.DepthRange(0, 1)
	}
	if p.CullBack {
		gl.Disable(gl.CULL_FACE)
	}
	if p.NoDepth {
		gl.DepthMask(true)
	}
	if p.OnTop {
		gl.Enable(gl.DEPTH_TEST)
	}
	gl.Disable(gl.POLYGON_OFFSET_FILL)
}

// DrawFaceRange draws one ID's triangles (a highlight over the shaded body).
func (r *Renderer) DrawFaceRange(v View, m *TriMesh, id uint32, p Paint) {
	r.faceSetup(v, p, false)
	gl.DepthFunc(gl.LEQUAL)
	if p.NoDepth {
		gl.DepthMask(false)
	}
	if p.CullBack {
		gl.Enable(gl.CULL_FACE)
		gl.CullFace(gl.BACK)
	}
	m.DrawRange(id)
	if p.CullBack {
		gl.Disable(gl.CULL_FACE)
	}
	if p.NoDepth {
		gl.DepthMask(true)
	}
	gl.Disable(gl.POLYGON_OFFSET_FILL)
}

func (r *Renderer) lineSetup(s *Shader, v View, p Paint, pick bool) {
	r.common(s, v, p, pick)
	s.Vec2("uViewport", mgl32.Vec2{float32(v.W), float32(v.H)})
	s.Float("uWidth", p.Width)
	s.Float("uDepthBias", p.DepthBias)
	if s == r.line || s == r.linePick {
		s.Float("uDash", p.Dash)
	}
}

// DrawLines draws polylines as screen-space strips p.Width pixels wide.
func (r *Renderer) DrawLines(v View, l *LineSet, p Paint, pick bool) {
	r.lineSetup(pickOr(pick, r.line, r.linePick), v, p, pick)
	if p.OnTop {
		gl.Disable(gl.DEPTH_TEST)
	}
	l.Draw()
	if p.OnTop {
		gl.Enable(gl.DEPTH_TEST)
	}
}

func (r *Renderer) DrawLineRange(v View, l *LineSet, id uint32, p Paint) {
	r.lineSetup(r.line, v, p, false)
	l.DrawRange(id)
}

// DrawPoints draws round points p.Width pixels across.
func (r *Renderer) DrawPoints(v View, s *PointSet, p Paint, pick bool) {
	r.lineSetup(pickOr(pick, r.point, r.pointPick), v, p, pick)
	s.Draw()
}

func (r *Renderer) DrawPoint(v View, s *PointSet, id uint32, p Paint) {
	r.lineSetup(r.point, v, p, false)
	s.DrawOne(id)
}

// DrawFlat draws unlit triangles (sketch regions, previews).
func (r *Renderer) DrawFlat(v View, m *TriMesh, p Paint, pick bool) {
	s := pickOr(pick, r.flat, r.flatPick)
	r.common(s, v, p, pick)
	gl.DepthMask(false)
	m.Draw()
	gl.DepthMask(true)
}

// Grid describes the infinite grid.
type Grid struct {
	Plane                  geom.Frame
	Step, Fade             float32
	Minor, Major           mgl32.Vec4
	AxisXColor, AxisYColor mgl32.Vec4
}

// DrawGrid draws the grid plane (blended, depth tested and written).
func (r *Renderer) DrawGrid(v View, g Grid) {
	s := r.grid
	s.Use()
	vp := v.Proj.Mul4(v.View)
	s.Mat4("uViewProj", vp)
	s.Mat4("uInvViewProj", vp.Inv())
	s.Vec3("uOrigin", v3(g.Plane.O))
	s.Vec3("uAxisX", v3(g.Plane.X))
	s.Vec3("uAxisY", v3(g.Plane.Y))
	s.Float("uStep", g.Step)
	s.Float("uFade", g.Fade)
	s.Vec3("uEye", v3(v.Eye))
	s.Vec4("uMinor", g.Minor)
	s.Vec4("uMajor", g.Major)
	s.Vec4("uAxisXColor", g.AxisXColor)
	s.Vec4("uAxisYColor", g.AxisYColor)
	gl.DepthMask(false)
	gl.Disable(gl.CLIP_DISTANCE0) // its shader writes no clip distance
	gl.BindVertexArray(r.emptyVAO)
	gl.DrawArrays(gl.TRIANGLES, 0, 3)
	gl.Enable(gl.CLIP_DISTANCE0)
	gl.DepthMask(true)
}

func v3(p geom.Vec3) mgl32.Vec3 { return mgl32.Vec3{float32(p.X), float32(p.Y), float32(p.Z)} }
