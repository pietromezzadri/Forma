package app

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	imgui "github.com/AllenDang/cimgui-go/imgui"
)

// snapSettings is how drags and clicks round what they set: on the grid's
// increment (following the zoom, or a fixed length), with Ctrl held for free
// movement and Shift for fine control (a tenth of the increment, and drags
// moving a tenth as far as the mouse).
type snapSettings struct {
	off   bool    // grid snapping off (Ctrl then snaps)
	step  float64 // a fixed increment in mm; 0 follows the grid
	angle float64 // rotation increment in degrees
}

var snapSteps = []float64{0, 0.1, 0.5, 1, 2, 5, 10, 25}
var snapAngles = []float64{1, 5, 15, 45, 90}

func (s snapSettings) angleStep() float64 {
	if s.angle <= 0 {
		return 15
	}
	return s.angle
}

// dragKeys is what the modifier keys held now do to snapping.
type dragKeys struct {
	snap bool // round to the increment
	fine bool // a tenth of the increment, and slower drags
}

func (a *App) dragKeys() dragKeys {
	io := imgui.CurrentIO()
	snap := !a.snap.off
	if io.KeyCtrl() {
		snap = !snap
	}
	return dragKeys{snap: snap, fine: io.KeyShift()}
}

// increment is the length a drag or click snaps to: the fixed step, or
// half the grid's minor spacing at this zoom.
func (a *App) increment() float64 {
	if a.snap.step > 0 {
		return a.snap.step
	}
	return niceStep(a.cam.WorldPerPixel(a.vp.py)*24*float64(a.vp.scale)) / 2
}

// snapLen rounds a length to the increment (or a tenth of it, fine) when
// snapping, else to a hundredth of a mm so typed-looking values come out.
func (a *App) snapLen(v float64, k dragKeys) float64 {
	if !k.snap {
		return math.Round(v*1000) / 1000
	}
	st := a.increment()
	if k.fine {
		st /= 10
	}
	return math.Round(v/st) * st
}

// snapAngle rounds degrees to the angle increment (a tenth of it, fine).
func (a *App) snapAngle(deg float64, k dragKeys) float64 {
	if !k.snap {
		return math.Round(deg*100) / 100
	}
	st := a.snap.angleStep()
	if k.fine {
		st /= 10
	}
	return math.Round(deg/st) * st
}

// fineDrag keeps a drag's own position, which moves a tenth as far as the
// mouse while Shift is held.
type fineDrag struct {
	on       bool
	raw, pos float64 // last raw mouse value, and where the drag has got to
}

// follow moves the drag on by the mouse's change since last time.
func (d *fineDrag) follow(raw float64, fine bool) float64 {
	if !d.on {
		d.on, d.raw, d.pos = true, raw, raw
		return raw
	}
	k := 1.0
	if fine {
		k = 0.1
	}
	d.pos += (raw - d.raw) * k
	d.raw = raw
	return d.pos
}

func (d *fineDrag) reset() { d.on = false }

// registerSnapCommands: the increments from the palette (and captures):
// "snap.step:0.5" (mm; 0 follows the grid), "snap.angle:15" (degrees).
func (a *App) registerSnapCommands(c *Commands) {
	c.add(&Command{ID: "snap.step", Label: "Snap increment (mm, 0 = grid)", Group: "View",
		Run: func(arg string) {
			if v, err := strconv.ParseFloat(strings.TrimSpace(arg), 64); err == nil && v >= 0 {
				a.snap.step = v
			}
		}})
	c.add(&Command{ID: "snap.angle", Label: "Snap angle (degrees)", Group: "View",
		Run: func(arg string) {
			if v, err := strconv.ParseFloat(strings.TrimSpace(arg), 64); err == nil && v > 0 {
				a.snap.angle = v
			}
		}})
}

// snapLabel is the status bar's snapping readout.
func (a *App) snapLabel() string {
	if a.snap.off {
		return "Snap off"
	}
	if a.snap.step > 0 {
		return "Snap " + fmtLen(a.snap.step) + " mm"
	}
	return "Snap grid " + fmtLen(a.increment()) + " mm"
}

// snapPopup sets the snapping (from the status bar).
func (a *App) snapPopup() {
	if !imgui.BeginPopup("##snap") {
		return
	}
	on := !a.snap.off
	if imgui.Checkbox("Snap to grid (G)", &on) {
		a.snap.off = !on
	}
	imgui.SeparatorText("Length increment")
	for _, s := range snapSteps {
		label := "Follow the grid (" + fmtLen(niceStep(a.cam.WorldPerPixel(a.vp.py)*24*float64(a.vp.scale))/2) + " mm now)"
		if s > 0 {
			label = fmtLen(s) + " mm"
		}
		if imgui.RadioButtonBool(label, a.snap.step == s) {
			a.snap.step = s
		}
	}
	imgui.SeparatorText("Angle increment")
	for _, s := range snapAngles {
		if imgui.RadioButtonBool(fmt.Sprintf("%g°##ang", s), a.snap.angleStep() == s) {
			a.snap.angle = s
		}
		imgui.SameLine()
	}
	imgui.NewLine()
	imgui.Separator()
	imgui.TextDisabled("While dragging: Ctrl moves freely, Shift for fine control")
	imgui.EndPopup()
}
