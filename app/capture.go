package app

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	imgui "github.com/AllenDang/cimgui-go/imgui"
	"github.com/go-gl/gl/v4.1-core/gl"

	"forma/geom"
	"forma/sketch"
)

// A capture script (FORMA_CAPTURE=<file.json>) drives a hidden window that
// never takes focus and writes screenshots, so the UI can be checked without
// touching the mouse or keyboard:
//
//	{"width": 1600, "height": 900, "out": "captures",
//	 "shots": [{"name": "box", "do": ["create.box:Width=40", "view.fit"], "frames": 8}]}
//
// Each action runs on its own frame: a command ID with an optional
// ":argument", or one of hover:<fx>,<fy> (the cursor at that fraction of the
// viewport), nohover, click, cam:<yaw°>,<pitch°>,<dist>, sk.demo, sk.free.
type captureScript struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Out    string `json:"out"`
	// Record writes every frame to <out>/frames/ for a video.
	Record bool `json:"record"`
	// Ini reads (and saves) the layout in that file, as a normal run does,
	// instead of always laying the panels out afresh.
	Ini   string `json:"ini"`
	Shots []struct {
		Name   string   `json:"name"`
		Do     []string `json:"do"`
		Frames int      `json:"frames"`
	} `json:"shots"`
}

type captureRun struct {
	script captureScript
	dir    string
	app    *App
	shot   int
	action int
	wait   int
	grab   bool
	done   bool
	fbo    uint32
	tex    uint32
	w, h   int
	pause  int

	cont    func() bool // a multi-frame action in progress (true when done)
	frame   int
	frames  chan framePix
	writers sync.WaitGroup
	caption string
	ripple  float32
	ripPos  imgui.Vec2
}

func loadCapture(path string) (*captureRun, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s captureScript
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if s.Width == 0 {
		s.Width, s.Height = 1600, 900
	}
	if s.Out == "" {
		s.Out = "captures"
	}
	dir := s.Out
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(filepath.Dir(path), dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &captureRun{script: s, dir: dir, wait: -1}, nil
}

// tick runs the next action, or counts down to the screenshot.
func (c *captureRun) tick() {
	if c.done || c.app == nil {
		return
	}
	if c.shot >= len(c.script.Shots) {
		c.done = true
		return
	}
	if c.cont != nil {
		if c.cont() {
			c.cont = nil
		}
		return
	}
	if c.pause > 0 {
		c.pause--
		return
	}
	sh := c.script.Shots[c.shot]
	if c.action < len(sh.Do) {
		c.do(sh.Do[c.action])
		c.action++
		return
	}
	if c.wait < 0 {
		c.wait = sh.Frames
		if c.wait <= 0 {
			c.wait = 6
		}
	}
	c.wait--
	if c.wait == 0 {
		c.grab = true
	}
}

func (c *captureRun) do(act string) {
	a := c.app
	id, arg, _ := strings.Cut(act, ":")
	nums := func() []float64 {
		var out []float64
		for _, f := range strings.Split(arg, ",") {
			v, _ := strconv.ParseFloat(strings.TrimSpace(f), 64)
			out = append(out, v)
		}
		return out
	}
	if c.input(id, arg) {
		return
	}
	switch id {
	case "hover":
		n := nums()
		if len(n) == 2 {
			v := imgui.Vec2{X: float32(n[0]) * a.vp.size.X, Y: float32(n[1]) * a.vp.size.Y}
			a.vp.virtual = &v
		}
		return
	case "wait":
		n, _ := strconv.Atoi(arg)
		c.pause = n
		return
	case "export":
		if !filepath.IsAbs(arg) {
			arg = filepath.Join(c.dir, arg)
		}
		if err := a.writeExport(arg); err != nil {
			log.Printf("capture: export: %v", err)
		} else {
			log.Printf("capture: exported %s", arg)
		}
		return
	case "log":
		if a.sk != nil {
			s := a.sk.item.S
			log.Printf("capture: sketch %d entities, %d constraints, DOF %d, %d profiles", len(s.Ents), len(s.Cons), s.Status.DOF, len(a.sk.item.Regions))
			for _, cn := range s.Cons {
				log.Printf("capture:   %s %v %.3f", cn.Kind, cn.E, cn.Value)
			}
		}
		log.Printf("capture: hover %s, %d selected", a.describe(a.hover), len(a.sel))
		return
	case "nohover":
		a.vp.virtual = nil
		return
	case "click":
		a.toggleSelect(a.hover, true)
		return
	case "camto":
		// ease the camera to yaw, pitch (degrees)
		n := nums()
		if len(n) >= 2 {
			to := *a.cam
			to.Yaw, to.Pitch = n[0]*math.Pi/180, n[1]*math.Pi/180
			a.cam.AnimateTo(to, 0.9)
		}
		return
	case "cam":
		n := nums()
		if len(n) >= 2 {
			a.cam.anim = nil
			a.cam.Yaw, a.cam.Pitch = n[0]*math.Pi/180, n[1]*math.Pi/180
			if len(n) >= 3 {
				a.cam.Dist = n[2]
			}
		}
		return
	case "sk.demo", "sk.free":
		if a.sk == nil {
			log.Printf("capture: %s needs an open sketch", id)
			return
		}
		demoSketch(a.sk.item.S, id == "sk.demo")
		a.sk.changed()
		return
	case "sk.tool":
		for i, n := range toolNames {
			if strings.EqualFold(n, arg) {
				a.sk.setTool(sketchTool(i))
			}
		}
		return
	case "sk.click":
		// a tool click where the cursor is
		if a.sk != nil {
			a.sk.press(a)
			a.sk.release(a, false)
		}
		return
	}
	if err := a.cmds.Run(id, arg); err != nil {
		log.Printf("capture: %v", err)
	}
}

// demoSketch draws a mounting plate: an 80 × 50 rectangle with a hole and a
// slot; with full it is fully constrained by dimensions.
func demoSketch(s *sketch.Sketch, full bool) {
	p := func(x, y float64) int { return s.AddPoint(geom.V2(x, y)).ID }
	a, b, c, d := p(0, 0), p(78, 2), p(80, 52), p(1, 49)
	l0, l1, l2, l3 := s.AddLine(a, b), s.AddLine(b, c), s.AddLine(c, d), s.AddLine(d, a)
	s.AddConstraint(sketch.Horizontal, []int{l0.ID}, 0)
	s.AddConstraint(sketch.Horizontal, []int{l2.ID}, 0)
	s.AddConstraint(sketch.Vertical, []int{l1.ID}, 0)
	s.AddConstraint(sketch.Vertical, []int{l3.ID}, 0)
	hc := p(22, 26)
	hole := s.AddCircle(hc, 9)
	// a slot: two arcs and two lines
	c1, c2 := p(50, 25), p(64, 25)
	s1, e1 := p(50, 31), p(50, 19)
	s2, e2 := p(64, 19), p(64, 31)
	arc1 := s.AddArc(c1, s1, e1)
	arc2 := s.AddArc(c2, s2, e2)
	top := s.AddLine(e2, s1)
	bot := s.AddLine(e1, s2)
	for _, k := range []struct {
		k   sketch.ConKind
		ids []int
	}{
		{sketch.Tangent, []int{top.ID, arc1.ID}}, {sketch.Tangent, []int{top.ID, arc2.ID}},
		{sketch.Tangent, []int{bot.ID, arc1.ID}}, {sketch.Tangent, []int{bot.ID, arc2.ID}},
		{sketch.Equal, []int{arc1.ID, arc2.ID}}, {sketch.Horizontal, []int{c1, c2}},
	} {
		s.AddConstraint(k.k, k.ids, 0)
	}
	if !full {
		s.Solve()
		return
	}
	s.AddConstraint(sketch.Fix, []int{a}, 0)
	s.AddConstraint(sketch.Distance, []int{a, b}, 80)
	s.AddConstraint(sketch.Distance, []int{b, c}, 50)
	s.AddConstraint(sketch.Diameter, []int{hole.ID}, 18)
	s.AddConstraint(sketch.Distance, []int{hc, l3.ID}, 20)
	s.AddConstraint(sketch.Distance, []int{hc, l0.ID}, 25)
	s.AddConstraint(sketch.Radius, []int{arc1.ID}, 6)
	s.AddConstraint(sketch.Distance, []int{c1, c2}, 16)
	s.AddConstraint(sketch.Distance, []int{c1, l0.ID}, 25)
	s.AddConstraint(sketch.Distance, []int{c2, l1.ID}, 14)
	s.Solve()
}

// bindTarget renders the whole window into an offscreen framebuffer (a
// hidden window's own back buffer isn't reliable to read).
func (c *captureRun) bindTarget(w, h int) {
	if c.fbo == 0 || c.w != w || c.h != h {
		if c.fbo != 0 {
			gl.DeleteFramebuffers(1, &c.fbo)
			gl.DeleteTextures(1, &c.tex)
		}
		c.w, c.h = w, h
		gl.GenFramebuffers(1, &c.fbo)
		gl.BindFramebuffer(gl.FRAMEBUFFER, c.fbo)
		gl.GenTextures(1, &c.tex)
		gl.BindTexture(gl.TEXTURE_2D, c.tex)
		gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, int32(w), int32(h), 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, c.tex, 0)
	}
	gl.BindFramebuffer(gl.FRAMEBUFFER, c.fbo)
}

func (c *captureRun) afterFrame(w, h int) {
	c.recordFrame(w, h)
	if !c.grab {
		return
	}
	c.grab = false
	px := make([]byte, w*h*4)
	gl.BindFramebuffer(gl.READ_FRAMEBUFFER, c.fbo)
	gl.ReadPixels(0, 0, int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(px))
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		src := px[(h-1-y)*w*4 : (h-y)*w*4]
		dst := img.Pix[y*img.Stride : y*img.Stride+w*4]
		copy(dst, src)
		for x := 0; x < w; x++ {
			dst[x*4+3] = 255
		}
	}
	name := c.script.Shots[c.shot].Name
	path := filepath.Join(c.dir, name+".png")
	f, err := os.Create(path)
	if err == nil {
		err = png.Encode(f, img)
		f.Close()
	}
	if err != nil {
		log.Printf("capture: %v", err)
	} else {
		log.Printf("capture: wrote %s", path)
	}
	c.shot++
	c.action, c.wait = 0, -1
}
