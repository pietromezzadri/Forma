package render

import (
	"math"

	"github.com/go-gl/gl/v4.1-core/gl"
)

func uintBits(v uint32) float32 { return math.Float32frombits(v) }

// ColorTarget is a multisampled colour + depth framebuffer resolved into a
// texture the UI shows.
type ColorTarget struct {
	W, H       int
	msFBO      uint32
	msColor    uint32
	msDepth    uint32
	resolveFBO uint32
	Texture    uint32
	samples    int32
}

// PickTarget is the ID buffer: R32UI IDs and a float depth texture.
type PickTarget struct {
	W, H  int
	fbo   uint32
	ids   uint32
	depth uint32
}

func NewColorTarget(samples int) *ColorTarget { return &ColorTarget{samples: int32(samples)} }

// Resize (re)allocates for w × h pixels.
func (t *ColorTarget) Resize(w, h int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	if w == t.W && h == t.H && t.msFBO != 0 {
		return
	}
	t.free()
	t.W, t.H = w, h
	gl.GenFramebuffers(1, &t.msFBO)
	gl.BindFramebuffer(gl.FRAMEBUFFER, t.msFBO)
	gl.GenRenderbuffers(1, &t.msColor)
	gl.BindRenderbuffer(gl.RENDERBUFFER, t.msColor)
	gl.RenderbufferStorageMultisample(gl.RENDERBUFFER, t.samples, gl.RGBA8, int32(w), int32(h))
	gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.RENDERBUFFER, t.msColor)
	gl.GenRenderbuffers(1, &t.msDepth)
	gl.BindRenderbuffer(gl.RENDERBUFFER, t.msDepth)
	gl.RenderbufferStorageMultisample(gl.RENDERBUFFER, t.samples, gl.DEPTH_COMPONENT32F, int32(w), int32(h))
	gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.RENDERBUFFER, t.msDepth)

	gl.GenFramebuffers(1, &t.resolveFBO)
	gl.BindFramebuffer(gl.FRAMEBUFFER, t.resolveFBO)
	gl.GenTextures(1, &t.Texture)
	gl.BindTexture(gl.TEXTURE_2D, t.Texture)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, int32(w), int32(h), 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, t.Texture, 0)
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
}

func (t *ColorTarget) free() {
	if t.msFBO == 0 {
		return
	}
	gl.DeleteFramebuffers(1, &t.msFBO)
	gl.DeleteRenderbuffers(1, &t.msColor)
	gl.DeleteRenderbuffers(1, &t.msDepth)
	gl.DeleteFramebuffers(1, &t.resolveFBO)
	gl.DeleteTextures(1, &t.Texture)
	t.msFBO = 0
}

// Bind makes it the draw target, with the viewport set.
func (t *ColorTarget) Bind() {
	gl.BindFramebuffer(gl.FRAMEBUFFER, t.msFBO)
	gl.Viewport(0, 0, int32(t.W), int32(t.H))
}

// Resolve copies the multisampled image into Texture.
func (t *ColorTarget) Resolve() {
	gl.BindFramebuffer(gl.READ_FRAMEBUFFER, t.msFBO)
	gl.BindFramebuffer(gl.DRAW_FRAMEBUFFER, t.resolveFBO)
	gl.BlitFramebuffer(0, 0, int32(t.W), int32(t.H), 0, 0, int32(t.W), int32(t.H), gl.COLOR_BUFFER_BIT, gl.NEAREST)
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
}

// ReadRGBA reads the resolved image (bottom row first).
func (t *ColorTarget) ReadRGBA() []byte {
	px := make([]byte, t.W*t.H*4)
	gl.BindFramebuffer(gl.READ_FRAMEBUFFER, t.resolveFBO)
	gl.ReadPixels(0, 0, int32(t.W), int32(t.H), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(px))
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	return px
}

func NewPickTarget() *PickTarget { return &PickTarget{} }

func (t *PickTarget) Resize(w, h int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	if w == t.W && h == t.H && t.fbo != 0 {
		return
	}
	if t.fbo != 0 {
		gl.DeleteFramebuffers(1, &t.fbo)
		gl.DeleteTextures(1, &t.ids)
		gl.DeleteTextures(1, &t.depth)
	}
	t.W, t.H = w, h
	gl.GenFramebuffers(1, &t.fbo)
	gl.BindFramebuffer(gl.FRAMEBUFFER, t.fbo)
	gl.GenTextures(1, &t.ids)
	gl.BindTexture(gl.TEXTURE_2D, t.ids)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.R32UI, int32(w), int32(h), 0, gl.RED_INTEGER, gl.UNSIGNED_INT, nil)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, t.ids, 0)
	gl.GenTextures(1, &t.depth)
	gl.BindTexture(gl.TEXTURE_2D, t.depth)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.DEPTH_COMPONENT32F, int32(w), int32(h), 0, gl.DEPTH_COMPONENT, gl.FLOAT, nil)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.TEXTURE_2D, t.depth, 0)
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
}

// Bind clears the IDs to 0 and the depth to far.
func (t *PickTarget) Bind() {
	gl.BindFramebuffer(gl.FRAMEBUFFER, t.fbo)
	gl.Viewport(0, 0, int32(t.W), int32(t.H))
	zero := [4]uint32{}
	gl.ClearBufferuiv(gl.COLOR, 0, &zero[0])
	one := float32(1)
	gl.ClearBufferfv(gl.DEPTH, 0, &one)
}

// Read returns the IDs in a (2r+1)² square round pixel (x, y) (top-left
// origin), row-major from the top, and the depth at (x, y) in [0, 1].
func (t *PickTarget) Read(x, y, r int) ([]uint32, int, float32) {
	gl.BindFramebuffer(gl.READ_FRAMEBUFFER, t.fbo)
	defer gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	gy := t.H - 1 - y
	x0, y0 := x-r, gy-r
	n := 2*r + 1
	out := make([]uint32, n*n)
	x1, y1 := x0+n, y0+n
	cx0, cy0, cx1, cy1 := max(x0, 0), max(y0, 0), min(x1, t.W), min(y1, t.H)
	depth := float32(1)
	if cx1 <= cx0 || cy1 <= cy0 {
		return out, n, depth
	}
	buf := make([]uint32, (cx1-cx0)*(cy1-cy0))
	gl.ReadPixels(int32(cx0), int32(cy0), int32(cx1-cx0), int32(cy1-cy0), gl.RED_INTEGER, gl.UNSIGNED_INT, gl.Ptr(buf))
	w := cx1 - cx0
	for yy := cy0; yy < cy1; yy++ {
		for xx := cx0; xx < cx1; xx++ {
			// flip to top-down rows
			row := (n - 1) - (yy - y0)
			out[row*n+(xx-x0)] = buf[(yy-cy0)*w+(xx-cx0)]
		}
	}
	if x >= 0 && x < t.W && gy >= 0 && gy < t.H {
		gl.ReadPixels(int32(x), int32(gy), 1, 1, gl.DEPTH_COMPONENT, gl.FLOAT, gl.Ptr(&depth))
	}
	return out, n, depth
}
