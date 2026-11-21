package app

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"forma/geom"
)

// Camera orbits a target point in a Z-up world. It looks along
// fwd(yaw, pitch) = (cos p cos y, cos p sin y, sin p) from Dist away; screen
// right is (sin y, −cos y, 0), so pitch may reach ±90° (top / bottom views).
type Camera struct {
	Target     geom.Vec3
	Dist       float64
	Yaw, Pitch float64 // radians
	Ortho      bool
	FovY       float64

	anim *camAnim
}

type camAnim struct {
	from, to Camera
	t, dur   float64
}

func NewCamera() *Camera {
	c := &Camera{Dist: 250, FovY: 35 * math.Pi / 180}
	c.SetView(ViewIso)
	return c
}

// Basis returns the view direction and the screen right and up axes.
func (c *Camera) Basis() (fwd, right, up geom.Vec3) {
	cp, sp := math.Cos(c.Pitch), math.Sin(c.Pitch)
	cy, sy := math.Cos(c.Yaw), math.Sin(c.Yaw)
	fwd = geom.V3(cp*cy, cp*sy, sp)
	right = geom.V3(sy, -cy, 0)
	up = right.Cross(fwd)
	return
}

func (c *Camera) Eye() geom.Vec3 {
	f, _, _ := c.Basis()
	return c.Target.Sub(f.Scale(c.Dist))
}

// ViewMatrix is the world-to-camera matrix.
func (c *Camera) ViewMatrix() mgl32.Mat4 {
	f, _, up := c.Basis()
	e := c.Target.Sub(f.Scale(c.Dist))
	return mgl32.LookAtV(v32(e), v32(c.Target), v32(up))
}

// OrthoHalfHeight is the half height of the view at the target (mm).
func (c *Camera) OrthoHalfHeight() float64 { return c.Dist * math.Tan(c.FovY/2) }

// Proj is the projection for an aspect ratio, with near / far planes
// fitted round the scene's bounding sphere.
func (c *Camera) Proj(aspect float64, scene geom.Box) mgl32.Mat4 {
	r := 1000.0
	center := c.Target
	if !scene.Empty() {
		r = scene.Diagonal()/2 + 1
		center = scene.Center()
	}
	f, _, _ := c.Basis()
	eye := c.Eye()
	d := center.Sub(eye).Dot(f)
	far := d + r*2 + c.Dist
	if far < c.Dist*4 {
		far = c.Dist * 4
	}
	if c.Ortho {
		h := c.OrthoHalfHeight()
		w := h * aspect
		return mgl32.Ortho(float32(-w), float32(w), float32(-h), float32(h), float32(-far), float32(far))
	}
	near := math.Max(math.Min(d-r*2, c.Dist*0.5), far*1e-5)
	if near <= 0 {
		near = far * 1e-5
	}
	return mgl32.Perspective(float32(c.FovY), float32(aspect), float32(near), float32(far))
}

// Ray from a viewport point (normalised device coordinates).
func (c *Camera) Ray(ndcX, ndcY, aspect float64) (o, d geom.Vec3) {
	f, r, u := c.Basis()
	if c.Ortho {
		h := c.OrthoHalfHeight()
		o = c.Eye().Add(r.Scale(ndcX * h * aspect)).Add(u.Scale(ndcY * h))
		return o, f
	}
	t := math.Tan(c.FovY / 2)
	d = f.Add(r.Scale(ndcX * t * aspect)).Add(u.Scale(ndcY * t)).Norm()
	return c.Eye(), d
}

// WorldPerPixel at the target's depth for a viewport h pixels tall.
func (c *Camera) WorldPerPixel(h int) float64 {
	return 2 * c.OrthoHalfHeight() / float64(max(h, 1))
}

// Orbit turns the view by (dyaw, dpitch) round pivot, keeping the pivot
// where it is on screen.
func (c *Camera) Orbit(dyaw, dpitch float64, pivot geom.Vec3) {
	c.anim = nil
	f0, r0, u0 := c.Basis()
	c.Yaw += dyaw
	c.Pitch = math.Max(-math.Pi/2, math.Min(math.Pi/2, c.Pitch+dpitch))
	f1, r1, u1 := c.Basis()
	// rotation taking the old camera frame to the new: R = B1 · B0ᵀ
	rel := c.Target.Sub(pivot)
	local := geom.V3(rel.Dot(r0), rel.Dot(u0), rel.Dot(f0))
	c.Target = pivot.Add(r1.Scale(local.X)).Add(u1.Scale(local.Y)).Add(f1.Scale(local.Z))
}

// Pan moves the view by a screen offset in pixels.
func (c *Camera) Pan(dx, dy float64, h int) {
	c.anim = nil
	_, r, u := c.Basis()
	k := c.WorldPerPixel(h)
	c.Target = c.Target.Sub(r.Scale(dx * k)).Add(u.Scale(dy * k))
}

// Zoom scales the distance by factor, toward the world point at.
func (c *Camera) Zoom(factor float64, at geom.Vec3) {
	c.anim = nil
	f, _, _ := c.Basis()
	// slide the target along the view plane toward `at`, keeping depth
	rel := at.Sub(c.Target)
	lateral := rel.Sub(f.Scale(rel.Dot(f)))
	c.Target = c.Target.Add(lateral.Scale(1 - factor))
	c.Dist *= factor
	c.Dist = math.Max(1e-3, math.Min(c.Dist, 1e7))
}

// Standard views.
type StdView int

const (
	ViewIso StdView = iota
	ViewFront
	ViewBack
	ViewRight
	ViewLeft
	ViewTop
	ViewBottom
)

func stdAngles(v StdView) (yaw, pitch float64) {
	d := math.Pi / 180
	switch v {
	case ViewFront:
		return 90 * d, 0
	case ViewBack:
		return -90 * d, 0
	case ViewRight:
		return 180 * d, 0
	case ViewLeft:
		return 0, 0
	case ViewTop:
		return 90 * d, -90 * d
	case ViewBottom:
		return 90 * d, 90 * d
	}
	return 135 * d, -math.Atan(1 / math.Sqrt2)
}

// SetView jumps to a standard view.
func (c *Camera) SetView(v StdView) {
	c.Yaw, c.Pitch = stdAngles(v)
	c.anim = nil
}

// AnimateTo eases to another camera over dur seconds.
func (c *Camera) AnimateTo(to Camera, dur float64) {
	from := *c
	from.anim = nil
	to.anim = nil
	// turn the short way round
	for to.Yaw-from.Yaw > math.Pi {
		to.Yaw -= 2 * math.Pi
	}
	for to.Yaw-from.Yaw < -math.Pi {
		to.Yaw += 2 * math.Pi
	}
	c.anim = &camAnim{from: from, to: to, dur: dur}
}

// AnimateView eases to a standard view.
func (c *Camera) AnimateView(v StdView) {
	to := *c
	to.Yaw, to.Pitch = stdAngles(v)
	c.AnimateTo(to, 0.3)
}

// Tick advances an animation.
func (c *Camera) Tick(dt float64) bool {
	a := c.anim
	if a == nil {
		return false
	}
	a.t += dt
	s := math.Min(a.t/a.dur, 1)
	s = s * s * (3 - 2*s)
	c.Target = a.from.Target.Lerp(a.to.Target, s)
	c.Dist = a.from.Dist * math.Pow(a.to.Dist/a.from.Dist, s)
	c.Yaw = a.from.Yaw + (a.to.Yaw-a.from.Yaw)*s
	c.Pitch = a.from.Pitch + (a.to.Pitch-a.from.Pitch)*s
	if a.t >= a.dur {
		c.anim = nil
	}
	return true
}

// Fit frames a box.
func (c *Camera) Fit(b geom.Box, aspect float64, animate bool) {
	if b.Empty() {
		return
	}
	to := *c
	if c.anim != nil {
		// keep turning toward where an animation was going
		to.Yaw, to.Pitch = c.anim.to.Yaw, c.anim.to.Pitch
	}
	to.anim = nil
	to.Target = b.Center()
	r := math.Max(b.Diagonal()/2, 1)
	fov := c.FovY / 2
	if aspect < 1 {
		fov = math.Atan(math.Tan(fov) * aspect)
	}
	to.Dist = r / math.Sin(fov) * 1.05
	if animate {
		c.AnimateTo(to, 0.3)
	} else {
		*c = to
	}
}

// LookAlong eases to look along a direction (a sketch plane's normal
// reversed), with screen up as close to up as can be.
func (c *Camera) LookAlong(dir, up, target geom.Vec3) {
	dir = dir.Norm()
	to := *c
	to.Target = target
	to.Pitch = math.Asin(math.Max(-1, math.Min(1, dir.Z)))
	if math.Abs(dir.Z) > 0.9999 {
		// straight down / up: yaw from the wanted screen up
		to.Yaw = math.Atan2(up.Y, up.X)
		if dir.Z > 0 {
			to.Yaw = math.Atan2(-up.Y, -up.X)
		}
	} else {
		to.Yaw = math.Atan2(dir.Y, dir.X)
	}
	c.AnimateTo(to, 0.35)
}

func v32(p geom.Vec3) mgl32.Vec3 { return mgl32.Vec3{float32(p.X), float32(p.Y), float32(p.Z)} }

// GizmoProj is the projection the gizmo works with: the view's, but with a
// depth range about the target rather than round the whole scene. ImGuizmo
// turns the mouse into a ray through the inverse of view × projection in
// float32, and a near plane a hundred-thousandth of the far one (the
// scene's sphere round the camera) made that ray wrong by an amount that
// changed with the view: drags moved the wrong way, or along two axes.
func (c *Camera) GizmoProj(aspect float64) mgl32.Mat4 {
	if c.Ortho {
		h := c.OrthoHalfHeight()
		w := h * aspect
		return mgl32.Ortho(float32(-w), float32(w), float32(-h), float32(h), float32(-c.Dist*20), float32(c.Dist*20))
	}
	return mgl32.Perspective(float32(c.FovY), float32(aspect), float32(c.Dist*0.02), float32(c.Dist*50))
}
