package app

import (
	"fmt"
	"strings"

	"forma/sketch"
)

// Kinds of pickable things. An ID-buffer value is kind (4 bits) | item
// slot (8 bits) | local ID (20 bits).
const (
	KindNone = iota
	KindFace
	KindEdge
	KindVertex
	KindSkPoint
	KindSkCurve
	KindSkRegion
	KindDim // a dimension label (picked in the UI overlay, not the ID buffer)
)

// Ref names one pickable thing: a face of a body, a point of a sketch, …
type Ref struct {
	Kind  int
	Item  int // body or sketch ID
	Local int // face / edge / vertex / entity ID, region index+1, constraint ID
}

func (r Ref) IsZero() bool { return r.Kind == KindNone }

func (r Ref) String() string {
	return fmt.Sprintf("%s %d/%d", kindName(r.Kind), r.Item, r.Local)
}

func kindName(k int) string {
	return [...]string{"none", "face", "edge", "vertex", "point", "curve", "profile", "dimension"}[k]
}

func idBase(kind, slot int) uint32 { return uint32(kind)<<28 | uint32(slot&0xff)<<20 }

func decodeID(v uint32) (kind, slot, local int) {
	return int(v >> 28), int(v >> 20 & 0xff), int(v & 0xfffff)
}

// slots map item IDs to the 8-bit slots used in the ID buffer this frame.
type slots struct {
	ids []int
}

func (s *slots) reset() { s.ids = s.ids[:0] }

func (s *slots) slot(item int) int {
	s.ids = append(s.ids, item)
	return len(s.ids) - 1
}

func (s *slots) item(slot int) int {
	if slot < 0 || slot >= len(s.ids) {
		return 0
	}
	return s.ids[slot]
}

// pickPriority ranks what wins under the cursor's square: points, then
// curves and edges, then faces and profiles.
func pickPriority(kind int) int {
	switch kind {
	case KindSkPoint, KindVertex:
		return 3
	case KindSkCurve, KindEdge:
		return 2
	case KindFace, KindSkRegion:
		return 1
	}
	return 0
}

// describe names a ref for the UI.
func (a *App) describe(r Ref) string {
	switch r.Kind {
	case KindFace, KindEdge, KindVertex:
		b := a.doc.Body(r.Item)
		if b == nil {
			return "?"
		}
		switch r.Kind {
		case KindFace:
			for _, f := range b.Body.Faces() {
				if f.ID == r.Local {
					return fmt.Sprintf("%s · face %s, %s", b.Name, a.faceLabel(f.Name), f.Surf.Kind())
				}
			}
		case KindEdge:
			for _, e := range b.Body.Edges() {
				if e.ID == r.Local {
					return fmt.Sprintf("%s · edge %d (%s)", b.Name, e.ID, e.Curve.Kind())
				}
			}
		case KindVertex:
			for _, v := range b.Body.Vertices() {
				if v.ID == r.Local {
					return fmt.Sprintf("%s · vertex (%.3f, %.3f, %.3f)", b.Name, v.P.X, v.P.Y, v.P.Z)
				}
			}
		}
	case KindSkPoint, KindSkCurve:
		s := a.doc.Sketch(r.Item)
		if s == nil || s.S.Ent(r.Local) == nil {
			return "?"
		}
		return s.Name + " · " + s.S.Describe(r.Local)
	case KindSkRegion:
		s := a.doc.Sketch(r.Item)
		if s == nil || r.Local-1 >= len(s.Regions) {
			return "?"
		}
		reg := s.Regions[r.Local-1]
		return fmt.Sprintf("%s · profile (%.2f mm², %d holes)", s.Name, reg.Area, len(reg.Holes))
	case KindDim:
		s := a.doc.Sketch(r.Item)
		if s == nil {
			return "?"
		}
		if c := s.S.Con(r.Local); c != nil {
			return s.Name + " · " + c.Kind.String() + " " + dimText(c)
		}
	}
	return ""
}

func (a *App) isSelected(r Ref) bool {
	for _, s := range a.sel {
		if s == r {
			return true
		}
	}
	return false
}

func (a *App) toggleSelect(r Ref, add bool) {
	if !add {
		a.sel = a.sel[:0]
		if !r.IsZero() {
			a.sel = append(a.sel, r)
		}
		return
	}
	for i, s := range a.sel {
		if s == r {
			a.sel = append(a.sel[:i], a.sel[i+1:]...)
			return
		}
	}
	if !r.IsZero() {
		a.sel = append(a.sel, r)
	}
}

// selectedSketchEnts are the selected entities of sketch item id, in order.
func (a *App) selectedSketchEnts(item int) []int {
	var out []int
	for _, r := range a.sel {
		if (r.Kind == KindSkPoint || r.Kind == KindSkCurve) && r.Item == item {
			out = append(out, r.Local)
		}
	}
	return out
}

func dimText(c *sketch.Constraint) string {
	switch c.Kind {
	case sketch.Angle:
		return fmt.Sprintf("%.2f°", abs(c.Value)*180/3.141592653589793)
	case sketch.Diameter:
		return fmt.Sprintf("Ø%s", fmtLen(c.Value))
	case sketch.Radius:
		return fmt.Sprintf("R%s", fmtLen(c.Value))
	case sketch.Offset:
		return fmtLen(c.Value) // signed: the side it is on
	}
	return fmtLen(abs(c.Value))
}

func fmtLen(v float64) string {
	s := fmt.Sprintf("%.3f", v)
	for len(s) > 1 && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}
	if s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	return s
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// faceLabel shows a persistent face name ("f12/top") as "top (Extrude 2)".
func (a *App) faceLabel(name string) string {
	var id int
	var role string
	if n, _ := fmt.Sscanf(name, "f%d/", &id); n == 1 {
		if i := strings.Index(name, "/"); i >= 0 {
			role = name[i+1:]
		}
		if f := a.doc.D.Feature(id); f != nil {
			return fmt.Sprintf("%s (%s)", role, f.Name)
		}
		return role
	}
	return name
}
