// Package ui runs Dear ImGui (cimgui-go, docking branch) on a go-gl GLFW
// window through ImGui's own GLFW and OpenGL 3 backends, and holds the look.
package ui

import (
	"unsafe"

	imgui "github.com/AllenDang/cimgui-go/imgui"
	"github.com/AllenDang/cimgui-go/imguizmo"
	implglfw "github.com/AllenDang/cimgui-go/impl/glfw"
	implgl "github.com/AllenDang/cimgui-go/impl/opengl3"
	"github.com/go-gl/glfw/v3.4/glfw"
)

// Backend owns the ImGui context for one window.
type Backend struct {
	win *glfw.Window
	gw  *implglfw.GLFWwindow
}

// Init creates the context, docking on, and hooks the window up. iniPath ""
// keeps no layout file (captures).
func Init(win *glfw.Window, iniPath string) *Backend {
	imgui.CreateContext()
	io := imgui.CurrentIO()
	io.SetConfigFlags(io.ConfigFlags() | imgui.ConfigFlagsDockingEnable | imgui.ConfigFlagsNavEnableKeyboard)
	io.SetIniFilename(iniPath)
	io.SetConfigWindowsMoveFromTitleBarOnly(true)
	b := &Backend{win: win, gw: implglfw.NewGLFWwindowFromC(unsafe.Pointer(win.Handle()))}
	implglfw.InitForOpenGL(b.gw, false)
	implgl.InitV("#version 410")
	b.forward()
	return b
}

// forward routes go-gl's callbacks to ImGui's GLFW backend (installing
// ImGui's own would fight go-gl over the window's callback slots).
func (b *Backend) forward() {
	w := b.win
	w.SetKeyCallback(func(_ *glfw.Window, key glfw.Key, sc int, a glfw.Action, m glfw.ModifierKey) {
		implglfw.KeyCallback(b.gw, int32(key), int32(sc), int32(a), int32(m))
	})
	w.SetCharCallback(func(_ *glfw.Window, c rune) { implglfw.CharCallback(b.gw, uint32(c)) })
	w.SetMouseButtonCallback(func(_ *glfw.Window, bt glfw.MouseButton, a glfw.Action, m glfw.ModifierKey) {
		implglfw.MouseButtonCallback(b.gw, int32(bt), int32(a), int32(m))
	})
	w.SetCursorPosCallback(func(_ *glfw.Window, x, y float64) { implglfw.CursorPosCallback(b.gw, x, y) })
	w.SetScrollCallback(func(_ *glfw.Window, x, y float64) { implglfw.ScrollCallback(b.gw, x, y) })
	w.SetCursorEnterCallback(func(_ *glfw.Window, e bool) {
		v := int32(0)
		if e {
			v = 1
		}
		implglfw.CursorEnterCallback(b.gw, v)
	})
	w.SetFocusCallback(func(_ *glfw.Window, f bool) {
		v := int32(0)
		if f {
			v = 1
		}
		implglfw.WindowFocusCallback(b.gw, v)
	})
}

// NewFrame starts a UI frame.
func (b *Backend) NewFrame() {
	implgl.NewFrame()
	implglfw.NewFrame()
	imgui.NewFrame()
	imguizmo.BeginFrame()
}

// Render draws the UI into the bound framebuffer.
func (b *Backend) Render() {
	imgui.Render()
	implgl.RenderDrawData(imgui.CurrentDrawData())
}

// ContentScale is the window's DPI scale.
func (b *Backend) ContentScale() float32 { return implglfw.ContentScaleForWindow(b.gw) }

func (b *Backend) Shutdown() {
	implgl.Shutdown()
	implglfw.Shutdown()
	imgui.DestroyContext()
}
