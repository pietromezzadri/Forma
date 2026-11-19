// Package render draws Forma's viewport with OpenGL 4.1: shaded faces,
// screen-space edges and points, the infinite grid, and an ID buffer for
// picking.
package render

import (
	"fmt"
	"strings"

	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/go-gl/mathgl/mgl32"
)

// Shader wraps a program with a uniform-location cache (after Attalo's
// engine/render/shader.go).
type Shader struct {
	id       uint32
	uniforms map[string]int32
}

// NewShader compiles and links vertex, optional geometry and fragment
// sources; defines are inserted after the #version line.
func NewShader(vert, geomSrc, frag string, defines ...string) (*Shader, error) {
	prep := func(src string) string {
		if len(defines) == 0 {
			return src
		}
		i := strings.Index(src, "\n")
		var b strings.Builder
		b.WriteString(src[:i+1])
		for _, d := range defines {
			b.WriteString("#define " + d + "\n")
		}
		b.WriteString(src[i+1:])
		return b.String()
	}
	var shaders []uint32
	for _, s := range []struct {
		src  string
		kind uint32
		name string
	}{{vert, gl.VERTEX_SHADER, "vertex"}, {geomSrc, gl.GEOMETRY_SHADER, "geometry"}, {frag, gl.FRAGMENT_SHADER, "fragment"}} {
		if s.src == "" {
			continue
		}
		id, err := compile(prep(s.src), s.kind)
		if err != nil {
			for _, d := range shaders {
				gl.DeleteShader(d)
			}
			return nil, fmt.Errorf("%s shader: %w", s.name, err)
		}
		shaders = append(shaders, id)
	}
	prog := gl.CreateProgram()
	for _, s := range shaders {
		gl.AttachShader(prog, s)
	}
	gl.LinkProgram(prog)
	for _, s := range shaders {
		gl.DeleteShader(s)
	}
	var status int32
	gl.GetProgramiv(prog, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var n int32
		gl.GetProgramiv(prog, gl.INFO_LOG_LENGTH, &n)
		log := strings.Repeat("\x00", int(n+1))
		gl.GetProgramInfoLog(prog, n, nil, gl.Str(log))
		return nil, fmt.Errorf("link: %s", log)
	}
	return &Shader{id: prog, uniforms: map[string]int32{}}, nil
}

func compile(src string, kind uint32) (uint32, error) {
	s := gl.CreateShader(kind)
	cs, free := gl.Strs(src + "\x00")
	gl.ShaderSource(s, 1, cs, nil)
	free()
	gl.CompileShader(s)
	var status int32
	gl.GetShaderiv(s, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var n int32
		gl.GetShaderiv(s, gl.INFO_LOG_LENGTH, &n)
		log := strings.Repeat("\x00", int(n+1))
		gl.GetShaderInfoLog(s, n, nil, gl.Str(log))
		gl.DeleteShader(s)
		return 0, fmt.Errorf("%s", log)
	}
	return s, nil
}

func (s *Shader) Use() { gl.UseProgram(s.id) }

func (s *Shader) loc(name string) int32 {
	if v, ok := s.uniforms[name]; ok {
		return v
	}
	v := gl.GetUniformLocation(s.id, gl.Str(name+"\x00"))
	s.uniforms[name] = v
	return v
}

func (s *Shader) Int(name string, v int32)       { gl.Uniform1i(s.loc(name), v) }
func (s *Shader) Uint(name string, v uint32)     { gl.Uniform1ui(s.loc(name), v) }
func (s *Shader) Float(name string, v float32)   { gl.Uniform1f(s.loc(name), v) }
func (s *Shader) Vec2(name string, v mgl32.Vec2) { gl.Uniform2fv(s.loc(name), 1, &v[0]) }
func (s *Shader) Vec3(name string, v mgl32.Vec3) { gl.Uniform3fv(s.loc(name), 1, &v[0]) }
func (s *Shader) Vec4(name string, v mgl32.Vec4) { gl.Uniform4fv(s.loc(name), 1, &v[0]) }
func (s *Shader) Mat4(name string, m mgl32.Mat4) { gl.UniformMatrix4fv(s.loc(name), 1, false, &m[0]) }
func (s *Shader) Delete()                        { gl.DeleteProgram(s.id) }
