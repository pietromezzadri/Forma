package app

import (
	"math"

	imgui "github.com/AllenDang/cimgui-go/imgui"

	"forma/doc"
	"forma/geom"
)

// dragHandle is an arrow from base along dir whose tip, at t along it, is
// dragged to set one of the selected feature's sizes.
type dragHandle struct {
	base, dir geom.Vec3
	t         float64
	min       float64         // the smallest t it may take
	set       func(t float64) // writes the feature's inputs for tip t
	label     func(t float64) string
}

func (h *dragHandle) tip() geom.Vec3 { return h.base.Add(h.dir.Scale(h.t)) }

// handleSet is the drag arrows of the selected feature: an extrude's
// distance, a primitive's sizes. Dragging snaps to the increment (see
// snapping.go): Ctrl free, Shift fine.
type handleSet struct {
	f        *doc.Feature
	hs       []dragHandle
	hot      int // the handle under the cursor, −1 none
	active   int // the one being dragged
	dragging bool
	fd       fineDrag
	value    string // the dragged value, for the readout
}

// update places the handles for the selected feature (outside sketch mode).
func (s *handleSet) update(a *App) {
	s.hs = s.hs[:0]
	f := a.selFeature()
	if a.sk != nil || f == nil || !f.Active || a.gizmoMode != modeResize {
		s.dragging = false
		s.f = nil
		return
	}
	s.f = f
	ev := func(k string) (float64, bool) {
		v, err := a.doc.D.Eval(f.Inputs[k])
		return v, err == nil
	}
	put := func(k string) func(float64) {
		return func(t float64) { f.Inputs[k] = fmtLen(t) }
	}
	mm := func(t float64) string { return fmtLen(math.Abs(t)) + " mm" }
	add := func(base, dir geom.Vec3, t, min float64, set func(float64), label func(float64) string) {
		s.hs = append(s.hs, dragHandle{base: base, dir: dir, t: t, min: min, set: set, label: label})
	}
	X, Y, Z := geom.XAxis, geom.YAxis, geom.ZAxis
	switch f.Kind {
	case doc.KExtrude:
		base, dir, ok := extrudeBase(a, f)
		d, ok2 := ev("Distance")
		if !ok || !ok2 {
			return
		}
		if f.Inputs["Flip"] == "1" {
			d = -d
		}
		sym := f.Inputs["Direction"] == "Symmetric"
		if sym {
			d = math.Abs(d) / 2
		}
		add(base, dir, d, math.Inf(-1), func(t float64) {
			switch {
			case sym:
				f.Inputs["Distance"] = fmtLen(2 * math.Abs(t))
			default:
				if f.Inputs["Flip"] == "1" {
					t = -t
				}
				f.Inputs["Distance"] = fmtLen(t)
			}
		}, func(t float64) string {
			if sym {
				return fmtLen(2*math.Abs(t)) + " mm"
			}
			return mm(t)
		})
	case doc.KBox, doc.KCylinder, doc.KCone, doc.KSphere, doc.KTorus:
		x, ok1 := ev("X")
		y, ok2 := ev("Y")
		z, ok3 := ev("Z")
		if !ok1 || !ok2 || !ok3 {
			return
		}
		at := geom.V3(x, y, z)
		switch f.Kind {
		case doc.KBox:
			w, ok1 := ev("Width")
			d, ok2 := ev("Depth")
			h, ok3 := ev("Height")
			if !ok1 || !ok2 || !ok3 {
				return
			}
			add(at.Add(geom.V3(0, d/2, h/2)), X, w, 0, put("Width"), mm)
			add(at.Add(geom.V3(w/2, 0, h/2)), Y, d, 0, put("Depth"), mm)
			add(at.Add(geom.V3(w/2, d/2, 0)), Z, h, 0, put("Height"), mm)
		case doc.KCylinder:
			r, ok1 := ev("Radius")
			h, ok2 := ev("Height")
			if !ok1 || !ok2 {
				return
			}
			add(at.Add(geom.V3(0, 0, h)), X, r, 0, put("Radius"), func(t float64) string { return "R " + mm(t) })
			add(at, Z, h, 0, put("Height"), mm)
		case doc.KCone:
			rb, ok1 := ev("Base radius")
			rt, ok2 := ev("Top radius")
			h, ok3 := ev("Height")
			if !ok1 || !ok2 || !ok3 {
				return
			}
			add(at, X, rb, 0, put("Base radius"), func(t float64) string { return "R " + mm(t) })
			add(at.Add(geom.V3(0, 0, h)), X, rt, 0, put("Top radius"), func(t float64) string { return "R " + mm(t) })
			add(at, Z, h, 0, put("Height"), mm)
		case doc.KSphere:
			r, ok := ev("Radius")
			if !ok {
				return
			}
			add(at, X, r, 0, put("Radius"), func(t float64) string { return "R " + mm(t) })
		case doc.KTorus:
			R, ok1 := ev("Major radius")
			rm, ok2 := ev("Tube radius")
			if !ok1 || !ok2 {
				return
			}
			add(at, X, R, 0, put("Major radius"), func(t float64) string { return "R " + mm(t) })
			add(at.Add(geom.V3(R, 0, 0)), Z, rm, 0, put("Tube radius"), func(t float64) string { return "r " + mm(t) })
		}
	}
	if s.active >= len(s.hs) {
		s.dragging = false
	}
}

// extrudeBase is where an extrude's arrow starts (the middle of its
// profile) and the way it points (the sketch's normal).
func extrudeBase(a *App, f *doc.Feature) (geom.Vec3, geom.Vec3, bool) {
	if len(f.Profiles) == 0 {
		return geom.Vec3{}, geom.Vec3{}, false
	}
	sk := a.doc.D.Feature(f.Profiles[0].Sketch)
	if sk == nil || sk.Sketch == nil || len(sk.Regions) == 0 {
		return geom.Vec3{}, geom.Vec3{}, false
	}
	ri, best := -1, -1.0
	for i, r := range sk.Regions {
		ref := doc.RefFor(sk.ID, r)
		if s := overlap(ref.Outer, f.Profiles[0].Outer); s > best {
			best, ri = s, i
		}
	}
	if ri < 0 {
		return geom.Vec3{}, geom.Vec3{}, false
	}
	pl := sk.Sketch.Plane
	return pl.At2(centroid(sk.Regions[ri].Poly)), pl.Z, true
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

// draw draws the arrows and finds which tip is under the cursor.
func (s *handleSet) draw(a *App, dl *imgui.DrawList) {
	s.hot = -1
	m := vadd(a.vp.pos, a.vp.mouse)
	bestD := float32(12)
	type shown struct{ B, T imgui.Vec2 }
	var vis []shown
	for i := range s.hs {
		B, ok1 := a.toScreen(s.hs[i].base)
		T, ok2 := a.toScreen(s.hs[i].tip())
		if !ok1 || !ok2 {
			vis = append(vis, shown{})
			continue
		}
		vis = append(vis, shown{B, T})
		if d := vlen(vsub(m, T)); a.vp.hovered && d < bestD {
			s.hot, bestD = i, d
		}
	}
	if s.dragging {
		s.hot = s.active
	}
	for i, h := range s.hs {
		B, T := vis[i].B, vis[i].T
		if B == (imgui.Vec2{}) && T == (imgui.Vec2{}) {
			continue
		}
		col := accentU32(1)
		dl.AddLineArgs(B, T, col, 2.5)
		if vlen(vsub(T, B)) > 1 {
			d := vnorm(vsub(T, B))
			n := vperp(d)
			back := vsub(T, vscale(d, 14))
			dl.AddTriangleFilled(T, vadd(back, vscale(n, 6)), vsub(back, vscale(n, 6)), col)
		}
		r := float32(7)
		if i == s.hot {
			r = 10
		}
		dl.AddCircleFilled(T, r, col)
		dl.AddCircleV(T, r, u32(0.1, 0.1, 0.1, 0.9), 24, 1.5)
		if i == s.hot || len(s.hs) == 1 {
			label := h.label(h.t)
			if s.dragging && i == s.active {
				label = s.value
			}
			dl.AddRectFilledV(vadd(T, imgui.Vec2{X: 14, Y: -22}), vadd(T, imgui.Vec2{X: 22 + imgui.CalcTextSize(label).X, Y: -2}), u32(1, 1, 1, 0.9), 4, 0)
			dl.AddTextVec2(vadd(T, imgui.Vec2{X: 18, Y: -21}), u32(0.1, 0.12, 0.16, 1), label)
		}
	}
}

// press starts a drag when a tip is under the cursor.
func (s *handleSet) press() bool {
	if s.hot >= 0 && s.hot < len(s.hs) {
		s.active, s.dragging = s.hot, true
		s.fd.reset()
		return true
	}
	return false
}

// drag puts the tip where the cursor's ray passes closest to the handle's
// line, snapped.
func (s *handleSet) drag(a *App) {
	if !s.dragging || s.active < 0 || s.active >= len(s.hs) {
		return
	}
	h := &s.hs[s.active]
	o, d := a.mouseRay()
	w := o.Sub(h.base)
	dn := d.Dot(h.dir)
	den := 1 - dn*dn
	if den < 1e-6 {
		return // looking straight along it
	}
	k := a.dragKeys()
	raw := (w.Dot(h.dir) - dn*w.Dot(d)) / den
	t := a.snapLen(s.fd.follow(raw, k.fine), k)
	if h.min > math.Inf(-1) {
		lo := h.min
		if lo == 0 {
			lo = a.increment() / 10
		}
		t = math.Max(t, lo)
	} else if math.Abs(t) < a.increment()/10 {
		t = a.increment() / 10 * sgn(t)
	}
	h.set(t)
	s.value = h.label(t)
	a.doc.changed = true
}

func sgn(x float64) float64 {
	if x < 0 {
		return -1
	}
	return 1
}

func (s *handleSet) release(a *App) bool {
	if !s.dragging {
		return false
	}
	s.dragging = false
	a.doc.Commit()
	return true
}
