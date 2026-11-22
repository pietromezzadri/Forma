// Package app is Forma's application: the document, the docked UI, the
// viewport and its tools.
package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	imgui "github.com/AllenDang/cimgui-go/imgui"
	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/go-gl/glfw/v3.4/glfw"

	"forma/geom"
	"forma/render"
	"forma/ui"
)

// Version shows in the title bar. Release builds set it from the tag
// (tools/release/build.ps1: -ldflags "-X forma/app.Version=…").
var Version = "0.1.0-dev"

type App struct {
	win     *glfw.Window
	backend *ui.Backend
	r       *render.Renderer

	doc *Document
	cam *Camera
	vp  viewport

	sel   []Ref
	selAt map[Ref]geom.Vec3 // where a selected face was clicked (a sketch on a curved face touches it there)
	hover Ref

	cmds    *Commands
	palette paletteState
	toast   toast

	sk         *sketchMode        // the sketch being edited, nil outside sketch mode
	dw         *drawMode          // the drawing sheet shown, nil outside drawing mode
	ribbonW    map[string]float32 // each ribbon's width, full and compact, as last drawn
	browserSel int                // body or sketch picked in the browser
	multi      []int              // more browser / timeline picks (Ctrl+click), besides browserSel

	showGrid    bool
	layoutSet   bool
	layoutReset bool // lay the panels out afresh, saved layout or not
	exportPath  string
	importPath  string
	showHelp    bool

	files      fileDialog
	dialogOpen bool // a native file dialog is up
	asm        assemblyState
	m13        m13State
	sim        simState
	elec       elecState
	code       codeEditor
	postQueue
	showParams bool

	section   sectionView
	gizmoMode gizmoMode // what dragging a selected feature does (resize, move, turn, scale)
	modeFor   int       // the feature gizmoMode was set for
	scale     scaleDrag
	density   string // g/cm³, for mass properties
	snap      snapSettings
	title     string
	autoT     float64
	lastAuto  []byte
	titleT    float64

	capture  *captureRun
	btnRects map[string]imgui.Vec2 // centres of named widgets (for capture scripts)
	quit     bool
	last     time.Time
}

// Run opens the window and runs until it closes. A FORMA_CAPTURE script
// runs hidden instead and exits after its last shot.
func Run() error {
	if err := glfw.Init(); err != nil {
		return fmt.Errorf("glfw: %w", err)
	}
	defer glfw.Terminate()

	var cap *captureRun
	if path := os.Getenv("FORMA_CAPTURE"); path != "" {
		c, err := loadCapture(path)
		if err != nil {
			return err
		}
		cap = c
	}

	glfw.WindowHint(glfw.ContextVersionMajor, 4)
	glfw.WindowHint(glfw.ContextVersionMinor, 1)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	glfw.WindowHint(glfw.ScaleToMonitor, glfw.True)
	w, h := 1600, 960
	if cap != nil {
		glfw.WindowHint(glfw.Visible, glfw.False)
		glfw.WindowHint(glfw.FocusOnShow, glfw.False)
		glfw.WindowHint(glfw.ScaleToMonitor, glfw.False)
		w, h = cap.script.Width, cap.script.Height
	}
	win, err := glfw.CreateWindow(w, h, "Forma "+Version, nil, nil)
	if err != nil {
		return fmt.Errorf("window: %w", err)
	}
	setWindowIcon(win)
	win.MakeContextCurrent()
	if err := gl.Init(); err != nil {
		return fmt.Errorf("gl: %w", err)
	}
	if cap == nil {
		glfw.SwapInterval(1)
	} else {
		glfw.SwapInterval(0)
	}
	log.Printf("OpenGL %s, %s", gl.GoStr(gl.GetString(gl.VERSION)), gl.GoStr(gl.GetString(gl.RENDERER)))

	ini := ""
	if cap == nil {
		ini = iniPath()
	} else if cap.script.Ini != "" {
		ini = cap.script.Ini
	}
	a := &App{win: win, doc: NewDocument(), cam: NewCamera(), showGrid: true, capture: cap, last: time.Now(), btnRects: map[string]imgui.Vec2{}, density: "7.85"}
	a.backend = ui.Init(win, ini)
	defer a.backend.Shutdown()
	scale := float32(1)
	if cap == nil {
		scale = a.backend.ContentScale()
	}
	ui.ApplyTheme(scale)
	ui.LoadFonts(15)
	if a.r, err = render.NewRenderer(); err != nil {
		return err
	}
	a.cmds = a.registerCommands()
	if cap != nil {
		cap.app = a
		imgui.CurrentIO().AddFocusEvent(true) // ImGui takes the script's events as from a focused window
	}
	for !win.ShouldClose() && !a.quit {
		if cap == nil {
			glfw.PollEvents()
		} else {
			glfw.PollEvents()
			cap.tick()
		}
		a.runPosted()
		a.frame()
		if cap != nil && cap.done {
			break
		}
	}
	if cap != nil {
		cap.finish()
	}
	a.cleanExit()
	return nil
}

func iniPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "forma.ini"
	}
	dir = filepath.Join(dir, "Forma")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "layout.ini")
}

// frame builds the UI, renders the scene into the viewport and draws it all.
func (a *App) frame() {
	now := time.Now()
	dt := now.Sub(a.last).Seconds()
	a.last = now
	if dt > 0.1 {
		dt = 0.1
	}
	if a.capture != nil {
		dt = 1.0 / 30
	}
	a.cam.Tick(dt)
	a.toast.tick(dt)
	if a.doc.changed {
		a.doc.Regenerate()
	}
	a.autosave(dt)
	if a.titleT += dt; a.titleT > 0.5 {
		a.titleT = 0
		a.updateTitle()
	}

	a.backend.NewFrame()
	a.buildUI()
	a.renderScene()

	fw, fh := a.win.GetFramebufferSize()
	if a.capture != nil {
		a.capture.bindTarget(fw, fh)
	} else {
		gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	}
	gl.Viewport(0, 0, int32(fw), int32(fh))
	gl.ClearColor(0.07, 0.08, 0.1, 1)
	gl.Clear(gl.COLOR_BUFFER_BIT)
	a.backend.Render()
	if a.capture != nil {
		a.capture.afterFrame(fw, fh)
	}
	a.win.SwapBuffers()
}

// buildUI lays out every window of the frame.
func (a *App) buildUI() {
	a.animateJoint()
	a.elecTick()
	a.mainMenu()
	a.toolbar()
	a.statusBar()
	vpID := imgui.DockSpaceOverViewportV(0, imgui.MainViewport(), imgui.DockNodeFlagsPassthruCentralNode, dockClass())
	if !a.layoutSet {
		a.defaultLayout(vpID)
		a.layoutSet = true
	}
	a.browserPanel()
	a.propertiesPanel()
	a.timelinePanel()
	a.viewportWindow()
	a.palette.draw(a)
	a.clashWindow()
	a.bomWindow()
	a.explodeWindow()
	a.simWindow()
	a.elecWindow()
	a.codeWindow()
	a.files.draw()
	a.paramsWindow()
	a.helpWindow()
	a.toast.draw()
	a.shortcuts()
	if a.capture != nil {
		a.capture.drawCursor()
	}
}

// notify shows a short message over the viewport.
func (a *App) notify(format string, args ...any) {
	a.toast.show(fmt.Sprintf(format, args...))
	log.Printf(format, args...)
}

var dockWC *imgui.WindowClass

// dockClass is the dockspace's (default) window class, made once.
func dockClass() *imgui.WindowClass {
	if dockWC == nil {
		dockWC = imgui.NewWindowClass()
	}
	return dockWC
}

// animate reports whether camera moves should ease (not in still captures).
func (a *App) animate() bool { return a.capture == nil || a.capture.script.Record }

// markItem records the last widget's centre under a name, so capture
// scripts can move the cursor to it.
func (a *App) markItem(name string) {
	mn, mx := imgui.ItemRectMin(), imgui.ItemRectMax()
	a.btnRects[name] = imgui.Vec2{X: (mn.X + mx.X) / 2, Y: (mn.Y + mx.Y) / 2}
}

// markItemLeft records a point just inside the last widget's left end (the
// text part of an input with step buttons).
func (a *App) markItemLeft(name string) {
	mn, mx := imgui.ItemRectMin(), imgui.ItemRectMax()
	a.btnRects[name] = imgui.Vec2{X: mn.X + 30, Y: (mn.Y + mx.Y) / 2}
}
