package doc

// The emboss feature: sketch profiles (a text, mostly) projected along the
// sketch's normal onto a face of a body and raised from it, engraved into
// it, or inlaid: a pocket for each letter grown by the clearance, and the
// letters as bodies of their own, flush with the face.

import (
	"fmt"
	"strings"

	"forma/brep"
	"forma/geom"
	"forma/ops"
	"forma/sketch"
)

// Emboss inputs and choices.
var (
	EmbossInputs = [][2]string{{"Type", "Inlay"}, {"Depth", "2"}, {"Clearance", "0.2"}}
	EmbossTypes  = []string{"Raise", "Engrave", "Inlay"}
)

// NewEmboss embosses profiles onto face of body target.
func NewEmboss(profiles []ProfileRef, target int, face FaceRef) *Feature {
	return withInputs(&Feature{Kind: KEmboss, Profiles: profiles, Targets: []int{target}, Faces: []FaceRef{face}}, EmbossInputs)
}

// emboss runs an emboss feature on the bodies there are.
func (d *Document) emboss(f *Feature, out []*BodyRef) ([]*BodyRef, error) {
	if len(f.Targets) != 1 || len(f.Faces) != 1 {
		return nil, fmt.Errorf("pick a face to emboss on")
	}
	i := findRef(out, f.Targets[0])
	if i < 0 {
		return nil, fmt.Errorf("its body is gone (joined into another, or rolled back)")
	}
	b := out[i].Body
	var fc *brep.Face
	for _, c := range b.Faces() {
		if c.Name == f.Faces[0].Face {
			fc = c
		}
	}
	if fc == nil {
		return nil, fmt.Errorf("its face (%s) isn't on the body any more", shortName(f.Faces[0].Face))
	}
	depth, err := d.input(f, "Depth")
	if err != nil {
		return nil, err
	}
	sp := ops.EmbossSpec{Depth: depth, Flip: f.Inputs["Flip"] == "1"}
	switch f.Inputs["Type"] {
	case "Raise":
		sp.Mode = ops.EmbossRaise
	case "Engrave":
		sp.Mode = ops.EmbossCut
	default:
		sp.Mode = ops.EmbossInlay
		if sp.Clearance, err = d.input(f, "Clearance"); err != nil {
			return nil, err
		}
	}
	regs, grown, plane, err := d.embossProfiles(f, sp.Clearance)
	if err != nil {
		return nil, err
	}
	nb, inlays, err := ops.EmbossFace(b, fc, regs, grown, plane, sp)
	if err != nil {
		return nil, err
	}
	qualifyLetters(nb, f.ID)
	nb.Name = b.Name
	r := out[i]
	out[i] = &BodyRef{Key: r.Key, Name: r.Name, Body: nb, Owner: r.Owner, Gen: d.nextGen()}
	for k, l := range inlays {
		if k >= KeysPerFeature {
			return nil, fmt.Errorf("too many letters (%d at most)", KeysPerFeature)
		}
		qualifyLetters(l, f.ID)
		l.Name = fmt.Sprintf("%s #%d", f.Name, k+1)
		out = append(out, &BodyRef{Key: BodyKey(f.ID, k), Name: l.Name, Body: l, Owner: f.ID, Gen: d.nextGen()})
	}
	return out, nil
}

// embossProfiles are the regions an emboss's profiles pick (every letter of
// a text) on one sketch, and the same grown by clearance for an inlay's
// pockets (texts only: their outlines can be offset).
func (d *Document) embossProfiles(f *Feature, clearance float64) ([]sketch.Region, []sketch.Region, geom.Frame, error) {
	if len(f.Profiles) == 0 {
		return nil, nil, geom.Frame{}, fmt.Errorf("no profile")
	}
	var regs, grown []sketch.Region
	var plane geom.Frame
	for i, ref := range f.Profiles {
		sk := d.Feature(ref.Sketch)
		if sk == nil || sk.Kind != KSketch {
			return nil, nil, plane, fmt.Errorf("its sketch is gone")
		}
		if d.Index(sk.ID) > d.Index(f.ID) {
			return nil, nil, plane, fmt.Errorf("its sketch comes after it in the timeline")
		}
		if i == 0 {
			plane = sk.Sketch.Plane
		} else if sk.Sketch.Plane != plane {
			return nil, nil, plane, fmt.Errorf("its profiles must all be on one sketch")
		}
		if ref.Text != 0 {
			n := 0
			for _, r := range sk.Regions {
				if r.Text == ref.Text && r.Ink {
					regs = append(regs, r)
					n++
				}
			}
			if n == 0 {
				return nil, nil, plane, fmt.Errorf("the text of profile %d of %s is gone or says nothing", i+1, sk.Name)
			}
			if clearance > 0 {
				grown = append(grown, grownText(sk.Sketch, ref.Text, clearance)...)
			}
			continue
		}
		ri, ok := matchRegion(ref, sk.Regions)
		if !ok {
			return nil, nil, plane, fmt.Errorf("profile %d of %s is gone (is the loop still closed?)", i+1, sk.Name)
		}
		if clearance > 0 {
			return nil, nil, plane, fmt.Errorf("an inlay's clearance needs a text profile (other outlines can't be offset yet)")
		}
		regs = append(regs, sk.Regions[ri])
	}
	return regs, grown, plane, nil
}

// grownText is a text's letters grown by by (a copy of it on its own).
func grownText(s *sketch.Sketch, id int, by float64) []sketch.Region {
	e := s.Ent(id)
	if e == nil || e.Kind != sketch.Text {
		return nil
	}
	t := sketch.New("grown", s.Plane)
	a := t.AddPoint(s.P(e.Pts[0])).ID
	g := t.AddText(a, e.TextOf(), e.Font, e.Size)
	g.Rot, g.Align, g.Grow = e.Rot, e.Align, e.Grow+by
	var out []sketch.Region
	for _, r := range t.Regions() {
		if r.Ink {
			out = append(out, r)
		}
	}
	return out
}

// qualifyLetters names the letters' faces as the feature's (f12/letter3/top).
func qualifyLetters(b *brep.Body, id int) {
	for _, fc := range b.Faces() {
		if strings.HasPrefix(fc.Name, "letter") || strings.HasPrefix(fc.Name, "inlay") {
			fc.Name = fmt.Sprintf("f%d/%s", id, fc.Name)
		}
	}
}
