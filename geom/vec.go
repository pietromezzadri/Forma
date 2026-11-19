// Package geom is Forma's float64 geometry: vectors, frames, curves and
// analytic surfaces. Lengths are millimetres.
package geom

import "math"

// Tol is the modelling tolerance for lengths (mm): points closer than this
// are the same point.
const Tol = 1e-7

// AngTol is the tolerance for angles (radians).
const AngTol = 1e-10

type Vec2 struct{ X, Y float64 }

type Vec3 struct{ X, Y, Z float64 }

func V2(x, y float64) Vec2    { return Vec2{x, y} }
func V3(x, y, z float64) Vec3 { return Vec3{x, y, z} }

func (a Vec2) Add(b Vec2) Vec2             { return Vec2{a.X + b.X, a.Y + b.Y} }
func (a Vec2) Sub(b Vec2) Vec2             { return Vec2{a.X - b.X, a.Y - b.Y} }
func (a Vec2) Scale(s float64) Vec2        { return Vec2{a.X * s, a.Y * s} }
func (a Vec2) Dot(b Vec2) float64          { return a.X*b.X + a.Y*b.Y }
func (a Vec2) Cross(b Vec2) float64        { return a.X*b.Y - a.Y*b.X }
func (a Vec2) Len() float64                { return math.Hypot(a.X, a.Y) }
func (a Vec2) Dist(b Vec2) float64         { return a.Sub(b).Len() }
func (a Vec2) Perp() Vec2                  { return Vec2{-a.Y, a.X} }
func (a Vec2) Lerp(b Vec2, t float64) Vec2 { return a.Add(b.Sub(a).Scale(t)) }
func (a Vec2) Angle() float64              { return math.Atan2(a.Y, a.X) }
func (a Vec2) Norm() Vec2 {
	l := a.Len()
	if l == 0 {
		return a
	}
	return a.Scale(1 / l)
}

func (a Vec3) Add(b Vec3) Vec3      { return Vec3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a Vec3) Sub(b Vec3) Vec3      { return Vec3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a Vec3) Scale(s float64) Vec3 { return Vec3{a.X * s, a.Y * s, a.Z * s} }
func (a Vec3) Neg() Vec3            { return Vec3{-a.X, -a.Y, -a.Z} }
func (a Vec3) Dot(b Vec3) float64   { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func (a Vec3) Cross(b Vec3) Vec3 {
	return Vec3{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X}
}
func (a Vec3) Len() float64                { return math.Sqrt(a.Dot(a)) }
func (a Vec3) Dist(b Vec3) float64         { return a.Sub(b).Len() }
func (a Vec3) Lerp(b Vec3, t float64) Vec3 { return a.Add(b.Sub(a).Scale(t)) }
func (a Vec3) Norm() Vec3 {
	l := a.Len()
	if l == 0 {
		return a
	}
	return a.Scale(1 / l)
}
func (a Vec3) Near(b Vec3) bool { return a.Dist(b) <= Tol }
func (a Vec3) Min(b Vec3) Vec3 {
	return Vec3{math.Min(a.X, b.X), math.Min(a.Y, b.Y), math.Min(a.Z, b.Z)}
}
func (a Vec3) Max(b Vec3) Vec3 {
	return Vec3{math.Max(a.X, b.X), math.Max(a.Y, b.Y), math.Max(a.Z, b.Z)}
}

// AnyPerp is some unit vector perpendicular to a (a unit).
func (a Vec3) AnyPerp() Vec3 {
	if math.Abs(a.X) < 0.9 {
		return a.Cross(Vec3{1, 0, 0}).Norm()
	}
	return a.Cross(Vec3{0, 1, 0}).Norm()
}

var (
	XAxis = Vec3{1, 0, 0}
	YAxis = Vec3{0, 1, 0}
	ZAxis = Vec3{0, 0, 1}
)

// Frame is a right-handed orthonormal frame: an origin and three unit axes.
type Frame struct {
	O       Vec3
	X, Y, Z Vec3
}

// World is the identity frame.
var World = Frame{X: XAxis, Y: YAxis, Z: ZAxis}

// FrameZ builds a frame at o whose Z is z, X as close to x as possible.
func FrameZ(o, z, x Vec3) Frame {
	z = z.Norm()
	x = x.Sub(z.Scale(x.Dot(z)))
	if x.Len() < 1e-12 {
		x = z.AnyPerp()
	}
	x = x.Norm()
	return Frame{O: o, X: x, Y: z.Cross(x), Z: z}
}

// ToWorld maps local coordinates to world.
func (f Frame) ToWorld(l Vec3) Vec3 {
	return f.O.Add(f.X.Scale(l.X)).Add(f.Y.Scale(l.Y)).Add(f.Z.Scale(l.Z))
}

// ToLocal maps a world point to local coordinates.
func (f Frame) ToLocal(p Vec3) Vec3 {
	d := p.Sub(f.O)
	return Vec3{d.Dot(f.X), d.Dot(f.Y), d.Dot(f.Z)}
}

// Dir maps a local direction to world.
func (f Frame) Dir(l Vec3) Vec3 {
	return f.X.Scale(l.X).Add(f.Y.Scale(l.Y)).Add(f.Z.Scale(l.Z))
}

// At2 maps 2D plane coordinates (z = 0) to world.
func (f Frame) At2(p Vec2) Vec3 { return f.ToWorld(Vec3{p.X, p.Y, 0}) }

// Flip turns the frame over (Z negated, Y negated, X kept).
func (f Frame) Flip() Frame { return Frame{O: f.O, X: f.X, Y: f.Y.Neg(), Z: f.Z.Neg()} }

// Box is an axis-aligned bounding box.
type Box struct {
	Min, Max Vec3
	set      bool
}

func (b *Box) Add(p Vec3) {
	if !b.set {
		b.Min, b.Max, b.set = p, p, true
		return
	}
	b.Min, b.Max = b.Min.Min(p), b.Max.Max(p)
}
func (b *Box) Empty() bool      { return !b.set }
func (b Box) Center() Vec3      { return b.Min.Add(b.Max).Scale(0.5) }
func (b Box) Diagonal() float64 { return b.Max.Dist(b.Min) }
func (b *Box) Union(o Box) {
	if o.set {
		b.Add(o.Min)
		b.Add(o.Max)
	}
}

// WrapAngle maps a to [0, 2π).
func WrapAngle(a float64) float64 {
	a = math.Mod(a, 2*math.Pi)
	if a < 0 {
		a += 2 * math.Pi
	}
	return a
}
