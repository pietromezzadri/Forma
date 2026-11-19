package app

import (
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	imgui "github.com/AllenDang/cimgui-go/imgui"
	"github.com/go-gl/gl/v4.1-core/gl"

	"forma/geom"
)

// Input actions for capture scripts. They feed ImGui events (never the OS),
// so the app's real input paths run — toolbar clicks, orbit drags, typing:
//
//	move:<where>[@frames]   ease the cursor there (default 15 frames)
//	click[:l|r|m]  dclick   down / up:<l|r|m>
//	drag:<where>[@frames]   left drag;  rdrag:<dx>,<dy>[@frames] right drag by pixels;  mdrag likewise
//	wheel:<steps>[@frames]  key:<name> (ctrl+ / shift+ prefixes)   type:<text>
//	say:<caption>           a caption over the bottom of the window ("say:" clears it)
//
// <where> is vp:<fx>,<fy> (fraction of the viewport), sk:<x>,<y> (sketch
// mm), w:<x>,<y>,<z> (world mm), ui:<x>,<y> (window pixels) or the name of a
// widget the UI marked (btn:<command>, prop:<field>, item:<browser name>).
func (c *captureRun) input(id, arg string) bool {
	a := c.app
	io := imgui.CurrentIO()
	spec, frames := splitFrames(arg, 15)
	switch id {
	case "move":
		to, ok := c.where(spec)
		if !ok {
			log.Printf("capture: can't find %q", spec)
			return true
		}
		c.moveTo(to, frames, nil)
	case "click", "dclick":
		btn := button(arg)
		n := 1
		if id == "dclick" {
			n = 2
		}
		c.clicks(btn, n)
	case "down":
		io.AddMouseButtonEvent(button(arg), true)
		c.ripPos, c.ripple = io.MousePos(), 1
	case "up":
		io.AddMouseButtonEvent(button(arg), false)
	case "drag":
		to, ok := c.where(spec)
		if !ok {
			log.Printf("capture: can't find %q", spec)
			return true
		}
		io.AddMouseButtonEvent(0, true)
		c.ripPos, c.ripple = io.MousePos(), 1
		c.moveTo(to, frames, func() { io.AddMouseButtonEvent(0, false) })
	case "rdrag", "mdrag":
		n := floats(spec)
		if len(n) != 2 {
			return true
		}
		b := int32(1)
		if id == "mdrag" {
			b = 2
		}
		from := io.MousePos()
		io.AddMouseButtonEvent(b, true)
		to := imgui.Vec2{X: from.X + float32(n[0]), Y: from.Y + float32(n[1])}
		c.moveTo(to, frames, func() {
			io.AddMouseButtonEvent(b, false)
		})
	case "wheel":
		steps, _ := strconv.ParseFloat(spec, 64)
		i := 0
		c.cont = func() bool {
			io.AddMouseWheelEvent(0, float32(steps/float64(frames)))
			i++
			return i >= frames
		}
	case "key":
		c.key(arg)
	case "type":
		runes := []rune(arg)
		i := 0
		c.cont = func() bool {
			if i%2 == 0 && i/2 < len(runes) {
				io.AddInputCharacter(uint32(runes[i/2]))
			}
			i++
			return i >= 2*len(runes)+1
		}
	case "hold", "release":
		// a modifier held down across later actions (shift-click to add to the selection)
		m := map[string]imgui.Key{"shift": imgui.ModShift, "ctrl": imgui.ModCtrl}[strings.ToLower(arg)]
		if m != 0 {
			io.AddKeyEvent(m, id == "hold")
		}
	case "say":
		c.caption = arg
	default:
		_ = a
		return false
	}
	return true
}

func splitFrames(arg string, def int) (string, int) {
	if i := strings.LastIndex(arg, "@"); i >= 0 {
		if n, err := strconv.Atoi(arg[i+1:]); err == nil && n > 0 {
			return arg[:i], n
		}
	}
	return arg, def
}

func floats(s string) []float64 {
	var out []float64
	for _, f := range strings.Split(s, ",") {
		v, err := strconv.ParseFloat(strings.TrimSpace(f), 64)
		if err != nil {
			return nil
		}
		out = append(out, v)
	}
	return out
}

func button(s string) int32 {
	switch s {
	case "r":
		return 1
	case "m":
		return 2
	}
	return 0
}

// where resolves a target to window coordinates.
func (c *captureRun) where(spec string) (imgui.Vec2, bool) {
	a := c.app
	kind, rest, _ := strings.Cut(spec, ":")
	n := floats(rest)
	switch kind {
	case "vp":
		if len(n) == 2 {
			return imgui.Vec2{X: a.vp.pos.X + float32(n[0])*a.vp.size.X, Y: a.vp.pos.Y + float32(n[1])*a.vp.size.Y}, true
		}
	case "ui":
		if len(n) == 2 {
			return imgui.Vec2{X: float32(n[0]), Y: float32(n[1])}, true
		}
	case "sk":
		if a.sk != nil && len(n) == 2 {
			return a.skScreen(a.sk.item.S, geom.V2(n[0], n[1])), true
		}
	case "w":
		if len(n) == 3 {
			return a.toScreen(geom.V3(n[0], n[1], n[2]))
		}
	case "gz":
		// along the move gizmo's x / y / z arrow, a fraction of its length
		// (gz:x,0.7)
		if p, ok := a.gizmoAt(); ok && len(rest) > 0 {
			ax := map[byte]geom.Vec3{'x': geom.XAxis, 'y': geom.YAxis, 'z': geom.ZAxis}[rest[0]]
			frac := 0.75
			if _, f, ok := strings.Cut(rest, ","); ok {
				frac, _ = strconv.ParseFloat(f, 64)
			}
			s0, ok0 := a.toScreen(p)
			s1, ok1 := a.toScreen(p.Add(ax.Scale(a.cam.WorldPerPixel(a.vp.py) * 10)))
			if ok0 && ok1 {
				d := imgui.Vec2{X: s1.X - s0.X, Y: s1.Y - s0.Y}
				l := float32(math.Hypot(float64(d.X), float64(d.Y)))
				// ImGuizmo's arrows span its clip-space size of the view's height
				L := float32(0.17) * a.vp.size.Y / 2 * float32(frac)
				return imgui.Vec2{X: s0.X + d.X/l*L, Y: s0.Y + d.Y/l*L}, true
			}
		}
	}
	p, ok := a.btnRects[spec]
	return p, ok
}

// moveTo eases the cursor to `to` over n frames, then calls then.
func (c *captureRun) moveTo(to imgui.Vec2, n int, then func()) {
	io := imgui.CurrentIO()
	from := io.MousePos()
	if from.X < -1e6 { // no position yet
		from = imgui.Vec2{X: float32(c.script.Width) / 2, Y: float32(c.script.Height) / 2}
	}
	i := 0
	c.cont = func() bool {
		i++
		t := float64(i) / float64(n)
		t = t * t * (3 - 2*t)
		io.AddMousePosEvent(from.X+(to.X-from.X)*float32(t), from.Y+(to.Y-from.Y)*float32(t))
		if i >= n {
			if then != nil {
				then()
			}
			return true
		}
		return false
	}
}

// clicks presses and releases a button n times, a frame apart.
func (c *captureRun) clicks(btn int32, n int) {
	io := imgui.CurrentIO()
	i := 0
	c.cont = func() bool {
		io.AddMouseButtonEvent(btn, i%2 == 0)
		if i%2 == 0 {
			c.ripPos, c.ripple = io.MousePos(), 1
		}
		i++
		return i >= 2*n
	}
}

var keyNames = map[string]imgui.Key{
	"escape": imgui.KeyEscape, "enter": imgui.KeyEnter, "delete": imgui.KeyDelete, "tab": imgui.KeyTab,
	"backspace": imgui.KeyBackspace, "up": imgui.KeyUpArrow, "down": imgui.KeyDownArrow, "f1": imgui.KeyF1,
	"num0": imgui.KeyKeypad0, "num1": imgui.KeyKeypad1, "num3": imgui.KeyKeypad3, "num5": imgui.KeyKeypad5, "num7": imgui.KeyKeypad7,
}

// key presses a key (with ctrl+ / shift+) and lets go on the next frame.
func (c *captureRun) key(name string) {
	io := imgui.CurrentIO()
	var mods []imgui.Key
	for {
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "ctrl+") {
			mods, name = append(mods, imgui.ModCtrl), name[5:]
		} else if strings.HasPrefix(lower, "shift+") {
			mods, name = append(mods, imgui.ModShift), name[6:]
		} else {
			break
		}
	}
	k, ok := keyNames[strings.ToLower(name)]
	if !ok && len(name) == 1 {
		r := strings.ToUpper(name)[0]
		switch {
		case r >= 'A' && r <= 'Z':
			k, ok = imgui.KeyA+imgui.Key(r-'A'), true
		case r >= '0' && r <= '9':
			k, ok = imgui.Key0+imgui.Key(r-'0'), true
		}
	}
	if !ok {
		log.Printf("capture: no key %q", name)
		return
	}
	i := 0
	c.cont = func() bool {
		switch i {
		case 0:
			for _, m := range mods {
				io.AddKeyEvent(m, true)
			}
			io.AddKeyEvent(k, true)
		case 1:
			io.AddKeyEvent(k, false)
			for _, m := range mods {
				io.AddKeyEvent(m, false)
			}
		}
		i++
		return i >= 2
	}
}

// drawCursor draws the scripted cursor, a ring where it clicks and the
// caption, over everything (only while recording a video).
func (c *captureRun) drawCursor() {
	if !c.script.Record {
		return
	}
	dl := imgui.ForegroundDrawListViewportPtr()
	io := imgui.CurrentIO()
	if c.ripple > 0 {
		r := 6 + (1-c.ripple)*22
		dl.AddCircleV(c.ripPos, r, accentU32(c.ripple), 32, 2.5)
		c.ripple -= 0.07
	}
	m := io.MousePos()
	if m.X > -1e6 {
		pts := []imgui.Vec2{{X: 0, Y: 0}, {X: 0, Y: 17}, {X: 4.5, Y: 13}, {X: 8, Y: 20}, {X: 10.5, Y: 19}, {X: 7, Y: 12}, {X: 12.5, Y: 12}}
		var poly []imgui.Vec2
		for _, p := range pts {
			poly = append(poly, imgui.Vec2{X: m.X + p.X, Y: m.Y + p.Y})
		}
		dl.AddConvexPolyFilled(&poly[0], 3, u32(1, 1, 1, 1))
		dl.AddConvexPolyFilled(&[]imgui.Vec2{poly[0], poly[2], poly[6]}[0], 3, u32(1, 1, 1, 1))
		dl.AddConvexPolyFilled(&[]imgui.Vec2{poly[2], poly[3], poly[4], poly[5]}[0], 4, u32(1, 1, 1, 1))
		dl.AddPolylineV(&poly[0], int32(len(poly)), u32(0, 0, 0, 1), 1.3, imgui.DrawFlagsClosed)
	}
	if c.caption != "" {
		font := io.FontDefault()
		size := float32(24)
		var ts imgui.Vec2
		if font != nil {
			ts = font.CalcTextSizeA(size, 1e9, 0, c.caption)
		} else {
			ts = imgui.CalcTextSize(c.caption)
		}
		w, h := float32(c.script.Width), float32(c.script.Height)
		p := imgui.Vec2{X: (w - ts.X) / 2, Y: h - 120}
		dl.AddRectFilledV(imgui.Vec2{X: p.X - 18, Y: p.Y - 10}, imgui.Vec2{X: p.X + ts.X + 18, Y: p.Y + ts.Y + 10}, u32(0.06, 0.07, 0.09, 0.86), 8, 0)
		if font != nil {
			dl.AddTextFontPtr(font, size, p, u32(1, 1, 1, 1), c.caption)
		} else {
			dl.AddTextVec2(p, u32(1, 1, 1, 1), c.caption)
		}
	}
}

type framePix struct {
	n    int
	w, h int
	px   []byte
}

// recordFrame queues this frame's pixels for the PNG writers.
func (c *captureRun) recordFrame(w, h int) {
	if !c.script.Record || c.done {
		return
	}
	if c.frames == nil {
		dir := filepath.Join(c.dir, "frames")
		_ = os.RemoveAll(dir)
		_ = os.MkdirAll(dir, 0o755)
		c.frames = make(chan framePix, 32)
		for i := 0; i < 6; i++ {
			c.writers.Add(1)
			go func() {
				defer c.writers.Done()
				enc := png.Encoder{CompressionLevel: png.BestSpeed}
				for f := range c.frames {
					img := image.NewNRGBA(image.Rect(0, 0, f.w, f.h))
					for y := 0; y < f.h; y++ {
						src := f.px[(f.h-1-y)*f.w*4 : (f.h-y)*f.w*4]
						dst := img.Pix[y*img.Stride : y*img.Stride+f.w*4]
						copy(dst, src)
						for x := 0; x < f.w; x++ {
							dst[x*4+3] = 255
						}
					}
					out, err := os.Create(filepath.Join(dir, fmt.Sprintf("f_%05d.png", f.n)))
					if err == nil {
						err = enc.Encode(out, img)
						out.Close()
					}
					if err != nil {
						log.Printf("capture: frame %d: %v", f.n, err)
					}
				}
			}()
		}
	}
	px := make([]byte, w*h*4)
	gl.BindFramebuffer(gl.READ_FRAMEBUFFER, c.fbo)
	gl.ReadPixels(0, 0, int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(px))
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
	c.frames <- framePix{n: c.frame, w: w, h: h, px: px}
	c.frame++
}

// finish waits for the frame writers.
func (c *captureRun) finish() {
	if c.frames != nil {
		close(c.frames)
		c.writers.Wait()
		log.Printf("capture: %d frames in %s", c.frame, filepath.Join(c.dir, "frames"))
	}
}
