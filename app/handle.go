package app

import (
	"math"

	imgui "github.com/AllenDang/cimgui-go/imgui"

	"forma/doc"
	"forma/geom"
)

// extrudeHandle is the arrow on a selected extrude's profile: drag its tip
// to set the distance.
type extrudeHandle struct {
	ok       bool
	f        *doc.Feature
	base     geom.Vec3 // on the profile
	dir      geom.Vec3 // the sketch normal
	t        float64   // where the tip is along dir
	hot      bool
	dragging bool
}

// update places the handle for the selected feature (outside sketch mode).
func (h *extrudeHandle) update(a *App) {
	h.ok = false
	f := a.selFeature()
	if a.sk != nil || f == nil || f.Kind != doc.KExtrude || !f.Active || len(f.Profiles) == 0 {
		h.dragging = false
		return
	}
	sk := a.doc.D.Feature(f.Profiles[0].Sketch)
	if sk == nil || sk.Sketch == nil || len(sk.Regions) == 0 {
		return
	}
	// the profile's centre: the centroid of the matching region's outline
	ri := -1
	best := -1.0
	for i, r := range sk.Regions {
		ref := doc.RefFor(sk.ID, r)
		if s := overlap(ref.Outer, f.Profiles[0].Outer); s > best {
			best, ri = s, i
		}
	}
	if ri < 0 {
		return
	}
	c := centroid(sk.Regions[ri].Poly)
	pl := sk.Sketch.Plane
	h.f = f
	h.base = pl.At2(c)
	h.dir = pl.Z
	d, err := a.doc.D.Eval(f.Inputs["Distance"])
	if err != nil {
		return
	}
	if f.Inputs["Flip"] == "1" {
		d = -d
	}
	if f.Inputs["Direction"] == "Symmetric" {
		d = math.Abs(d) / 2
	}
	h.t = d
	h.ok = true
}

func overlap(a, b []int) float64 {
	m := map[int]bool{}
	for _, x := range a {
		m[x] = true
	}
	n := 0
	for _, x := range b {
		if m[x] {
			n++
		}
	}
	return float64(n)
}

func centroid(p []geom.Vec2) geom.Vec2 {
	var cx, cy, a float64
	for i := range p {
		q, r := p[i], p[(i+1)%len(p)]
		k := q.Cross(r)
		a += k
		cx += (q.X + r.X) * k
		cy += (q.Y + r.Y) * k
	}
	if math.Abs(a) < 1e-12 {
		if len(p) > 0 {
			return p[0]
		}
		return geom.Vec2{}
	}
	return geom.Vec2{X: cx / (3 * a), Y: cy / (3 * a)}
}

func (h *extrudeHandle) tip() geom.Vec3 { return h.base.Add(h.dir.Scale(h.t)) }

// draw draws the arrow and finds whether the cursor is on its tip.
func (h *extrudeHandle) draw(a *App, dl *imgui.DrawList) {
	h.hot = false
	if !h.ok {
		return
	}
	B, ok1 := a.toScreen(h.base)
	T, ok2 := a.toScreen(h.tip())
	if !ok1 || !ok2 {
		return
	}
	m := vadd(a.vp.pos, a.vp.mouse)
	h.hot = a.vp.hovered && vlen(vsub(m, T)) < 12 || h.dragging
	col := accentU32(1)
	dl.AddLineArgs(B, T, col, 2.5)
	d := vnorm(vsub(T, B))
	if vlen(vsub(T, B)) > 1 {
		n := vperp(d)
		back := vsub(T, vscale(d, 14))
		dl.AddTriangleFilled(T, vadd(back, vscale(n, 6)), vsub(back, vscale(n, 6)), col)
	}
	r := float32(7)
	if h.hot {
		r = 10
	}
	dl.AddCircleFilled(T, r, col)
	dl.AddCircleV(T, r, u32(0.1, 0.1, 0.1, 0.9), 24, 1.5)
	dist, _ := a.doc.D.Eval(h.f.Inputs["Distance"])
	label := fmtLen(math.Abs(dist)) + " mm"
	dl.AddRectFilledV(vadd(T, imgui.Vec2{X: 14, Y: -22}), vadd(T, imgui.Vec2{X: 22 + imgui.CalcTextSize(label).X, Y: -2}), u32(1, 1, 1, 0.9), 4, 0)
	dl.AddTextVec2(vadd(T, imgui.Vec2{X: 18, Y: -21}), u32(0.1, 0.12, 0.16, 1), label)
}

// press starts a drag when the tip is under the cursor.
func (h *extrudeHandle) press() bool {
	if h.ok && h.hot {
		h.dragging = true
		return true
	}
	return false
}

// drag puts the tip where the cursor's ray passes closest to the handle's line.
func (h *extrudeHandle) drag(a *App) {
	if !h.dragging || !h.ok {
		return
	}
	o, d := a.mouseRay()
	w := o.Sub(h.base)
	dn := d.Dot(h.dir)
	den := 1 - dn*dn
	if den < 1e-6 {
		return // looking straight along it
	}
	t := (w.Dot(h.dir) - dn*w.Dot(d)) / den
	step := niceStep(a.cam.WorldPerPixel(a.vp.py)*8) / 2
	t = math.Round(t/step) * step
	f := h.f
	switch f.Inputs["Direction"] {
	case "Symmetric":
		f.Inputs["Distance"] = fmtLen(2 * math.Abs(t))
	default:
		if f.Inputs["Flip"] == "1" {
			t = -t
		}
		if math.Abs(t) < step {
			t = step * sgn(t)
		}
		f.Inputs["Distance"] = fmtLen(t)
	}
	a.doc.changed = true
}

func sgn(x float64) float64 {
	if x < 0 {
		return -1
	}
	return 1
}

func (h *extrudeHandle) release(a *App) bool {
	if !h.dragging {
		return false
	}
	h.dragging = false
	a.doc.Commit()
	return true
}
