package brep

import (
	"fmt"
	"math"

	"forma/geom"
)

// Check validates a body's topology and geometry:
//   - loops close: every half-edge's next starts where it ends, prev/next agree;
//   - every edge has both halves, one each way;
//   - the Euler–Poincaré formula V − E + F − R = 2(S − G) gives a whole genus G ≥ 0;
//   - edge ends lie on their vertices and edges on both of their faces' surfaces.
func Check(b *Body) error {
	if err := CheckTopo(b); err != nil {
		return err
	}
	return checkGeom(b)
}

// CheckTopo checks the topology alone (faces may have no surface yet).
func CheckTopo(b *Body) error {
	for _, s := range b.Shells {
		for _, f := range s.Faces {
			if f.Shell != s {
				return fmt.Errorf("face %d: wrong shell", f.ID)
			}
			if len(f.Loops) == 0 {
				return fmt.Errorf("face %d %q has no loop", f.ID, f.Name)
			}
			for _, l := range f.Loops {
				if l.Face != f {
					return fmt.Errorf("loop %d: wrong face", l.ID)
				}
				if l.First == nil {
					if l.Lone == nil {
						return fmt.Errorf("loop %d is empty", l.ID)
					}
					continue
				}
				for _, h := range l.HalfEdges() {
					if h.Loop != l {
						return fmt.Errorf("loop %d: half-edge of edge %d belongs elsewhere", l.ID, h.Edge.ID)
					}
					if h.Next.Prev != h || h.Prev.Next != h {
						return fmt.Errorf("loop %d: broken links at edge %d", l.ID, h.Edge.ID)
					}
					if h.End() != h.Next.Start() {
						return fmt.Errorf("face %q: loop %d not closed at edge %d (ends at v%d, next starts at v%d)",
							f.Name, l.ID, h.Edge.ID, h.End().ID, h.Next.Start().ID)
					}
					want := 1
					if h.Fwd {
						want = 0
					}
					if h.Edge.HE[want] != h {
						return fmt.Errorf("edge %d: half-edge in the wrong slot", h.Edge.ID)
					}
				}
			}
		}
	}
	for _, e := range b.Edges() {
		if e.HE[0] == nil || e.HE[1] == nil {
			return fmt.Errorf("edge %d: open (one side only)", e.ID)
		}
		if !e.HE[0].Fwd || e.HE[1].Fwd {
			return fmt.Errorf("edge %d: halves run the same way", e.ID)
		}
		if e.HE[0].Loop.Face.Shell != e.HE[1].Loop.Face.Shell {
			return fmt.Errorf("edge %d joins two shells", e.ID)
		}
	}
	v, e, f, r, s := b.Counts()
	chi := v - e + f - r
	if (2*s-chi)%2 != 0 || 2*s-chi < 0 {
		return fmt.Errorf("Euler–Poincaré fails: V %d − E %d + F %d − R %d = %d with %d shell(s)", v, e, f, r, chi, s)
	}
	return nil
}

func checkGeom(b *Body) error {
	for _, f := range b.Faces() {
		if f.Surf == nil {
			return fmt.Errorf("face %d %q has no surface", f.ID, f.Name)
		}
	}
	for _, e := range b.Edges() {
		if e.Curve == nil {
			return fmt.Errorf("edge %d has no curve", e.ID)
		}
		if d := e.Curve.Eval(e.T0).Dist(e.Verts[0].P); d > 1e-6 {
			return fmt.Errorf("edge %d (a %s, %.4v to %.4v): start %.3g mm from its vertex", e.ID, e.Curve.Kind(), e.Verts[0].P, e.Verts[1].P, d)
		}
		if d := e.Curve.Eval(e.T1).Dist(e.Verts[1].P); d > 1e-6 {
			return fmt.Errorf("edge %d (a %s): end %.3g mm from its vertex", e.ID, e.Curve.Kind(), d)
		}
		tol := 1e-6
		if tc, ok := e.Curve.(geom.Toleranced); ok {
			tol = math.Max(tol, tc.CurveTol()*1.5)
		}
		for i := 1; i < 8; i++ {
			p := e.Curve.Eval(e.T0 + (e.T1-e.T0)*float64(i)/8)
			for _, h := range e.HE {
				if d := geom.SurfaceDist(h.Loop.Face.Surf, p); d > tol {
					return fmt.Errorf("edge %d (a %s, %.4v to %.4v) leaves face %q's surface by %.3g mm", e.ID, e.Curve.Kind(), e.Verts[0].P, e.Verts[1].P, h.Loop.Face.Name, d)
				}
			}
		}
	}
	return nil
}

// Genus is the number of handles (through-holes) of a valid body.
func Genus(b *Body) int {
	v, e, f, r, s := b.Counts()
	return (2*s - (v - e + f - r)) / 2
}
