package doc

import (
	"math"
	"testing"

	"forma/drawing"
	"forma/geom"
)

// A drawing kept in the design: saved and read back with it, and its
// dimensions following an edit of a parameter.
func TestDrawingFollowsTheDesign(t *testing.T) {
	d, _, _ := design(t)
	s := d.AddSheet(drawing.NewSheet("Block", "A4"))
	bodies := d.DrawingBodies()
	if len(bodies) != 1 {
		t.Fatalf("%d bodies", len(bodies))
	}
	s.Standard(bodies)
	res := s.Build(bodies, nil)
	front := res.Views[s.Views[0].ID]
	bx := bodies[0].Body.Bounds()
	// the block's bottom edge in front: as wide as w
	a, ok := front.Pick(front.SheetOf(geom.V3((bx.Min.X+bx.Max.X)/2, bx.Min.Y, bx.Min.Z)), 0.8)
	if !ok {
		t.Fatal("nothing picked")
	}
	dim := s.AddDim(&drawing.Dim{View: front.View.ID, Kind: "h", Anchors: []drawing.Anchor{a}, Place: geom.V2(0, -20)})
	value := func(d *Document) float64 {
		s := d.Sheet(1)
		res := s.Build(d.DrawingBodies(), nil)
		var dm *drawing.Dim
		for _, x := range s.Dims {
			if x.ID == dim.ID {
				dm = x
			}
		}
		if e := res.Dims[dm.ID]; e != "" {
			t.Fatal(e)
		}
		v, err := dm.Value(res.Views[dm.View])
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	if v := value(d); math.Abs(v-40) > 1e-6 {
		t.Fatalf("width %v, want 40", v)
	}
	// saved and read back
	d2, err := Unmarshal(d.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	if len(d2.Drawings) != 1 || len(d2.Drawings[0].Views) != 4 || len(d2.Drawings[0].Dims) != 1 {
		t.Fatalf("read back %d sheets", len(d2.Drawings))
	}
	if v := value(d2); math.Abs(v-40) > 1e-6 {
		t.Fatalf("width read back %v, want 40", v)
	}
	// w made 55: the dimension says so
	for _, p := range d2.Params {
		if p.Name == "w" {
			p.Expr = "55"
		}
	}
	d2.Regenerate()
	if v := value(d2); math.Abs(v-55) > 1e-6 {
		t.Fatalf("width after the edit %v, want 55", v)
	}
	d2.RemoveSheet(1)
	if len(d2.Drawings) != 0 {
		t.Fatal("sheet not removed")
	}
}
