package doc

import (
	"fmt"
	"sort"
	"strconv"
)

// Part models: the electronics library's parts as components with their
// real outlines (board, mounting holes, shafts, connectors), built from
// primitives, so a case or a chassis is designed round them and the
// interference check catches what doesn't fit. Sizes are the common
// modules' (mm). A model's shaft is along +Z, its top rim's centre given
// in Shaft, where a joint goes.

// PartModel is a 3D part model.
type PartModel struct {
	ID, Label string
	// Shaft is the output shaft's top rim (centre and radius) in the
	// model's frame, if it has one
	Shaft *[4]float64
	prims []prim
}

type prim struct {
	kind string
	v    []float64 // box: w d h x y z; cylinder: r h x y z
	op   string    // "" (the first: new body) or Join / Cut
}

func box(w, d, h, x, y, z float64, op string) prim {
	return prim{KBox, []float64{w, d, h, x, y, z}, op}
}
func cyl(r, h, x, y, z float64, op string) prim {
	return prim{KCylinder, []float64{r, h, x, y, z}, op}
}

// PartModels are the library's 3D models, by the electronics library's
// model ID.
var PartModels = map[string]*PartModel{
	"uno": {ID: "uno", Label: "Arduino Uno", prims: []prim{
		box(68.6, 53.3, 1.6, 0, 0, 0, ""),
		cyl(1.6, 3, 14, 2.5, -0.7, "Cut"), cyl(1.6, 3, 15.3, 50.7, -0.7, "Cut"),
		cyl(1.6, 3, 66.1, 7.6, -0.7, "Cut"), cyl(1.6, 3, 66.1, 35.5, -0.7, "Cut"),
		box(16, 12, 10.9, -6.2, 32, 1.2, "Join"),   // USB-B
		box(13.5, 9, 10.9, -1.8, 3.3, 1.2, "Join"), // power jack
		box(35.6, 2.5, 8.5, 26.6, 50.3, 1.2, "Join"),
		box(28, 2.5, 8.5, 18.4, 0.6, 1.2, "Join"), box(15.2, 2.5, 8.5, 48.5, 0.6, 1.2, "Join"), // headers
		box(35.3, 9.9, 4, 30, 6, 1.2, "Join"), // the ATmega328P
	}},
	"sg90": {ID: "sg90", Label: "SG90 micro servo", Shaft: &[4]float64{6, 6.1, 27.5, 2.4}, prims: []prim{
		box(23, 12.2, 22, 0, 0, 0, ""),
		box(32.2, 12.2, 2.5, -4.6, 0, 15.5, "Join"),
		cyl(1, 4, -2.3, 6.1, 14.5, "Cut"), cyl(1, 4, 25.3, 6.1, 14.5, "Cut"),
		cyl(5.75, 1.5, 6, 6.1, 21.8, "Join"),
		cyl(2.4, 4.3, 6, 6.1, 23.2, "Join"),
	}},
	"mg996r": {ID: "mg996r", Label: "MG996R servo", Shaft: &[4]float64{10, 9.85, 46, 3}, prims: []prim{
		box(40.7, 19.7, 37, 0, 0, 0, ""),
		box(54.5, 19.7, 2.5, -6.9, 0, 27.5, "Join"),
		cyl(2.2, 4, -3.5, 9.85, 26.5, "Cut"), cyl(2.2, 4, 44.2, 9.85, 26.5, "Cut"),
		cyl(9, 3, 10, 9.85, 36.8, "Join"),
		cyl(3, 6.2, 10, 9.85, 39.8, "Join"),
	}},
	"tt-motor": {ID: "tt-motor", Label: "TT gear motor", Shaft: &[4]float64{11.25, 11.25, 28.4, 2.7}, prims: []prim{
		box(22.5, 37, 18.8, 0, 0, 0, ""),         // gearbox
		box(18, 32, 15, 2.25, 36.5, 1.9, "Join"), // the motor can
		cyl(2.7, 10.1, 11.25, 11.25, 18.3, "Join"),
		cyl(1.5, 22, 11.25, 27, -1.5, "Cut"), // mounting holes through the gearbox
	}},
	"n20-motor": {ID: "n20-motor", Label: "N20 gear motor", Shaft: &[4]float64{6, 5, 19, 1.5}, prims: []prim{
		box(12, 10, 9, 0, 0, 0, ""), // gearbox
		cyl(5, 15, 6, 5, -15, "Join"),
		cyl(1.5, 10, 6, 5, 9, "Join"),
	}},
	"battery-4aa": {ID: "battery-4aa", Label: "4 × AA holder", prims: []prim{
		box(62.5, 57.5, 15.5, 0, 0, 0, ""),
		box(58.5, 53.5, 14, 2, 2, 2, "Cut"),
		box(1.5, 53.5, 10, 16.5, 2, 1, "Join"), box(1.5, 53.5, 10, 31.5, 2, 1, "Join"), box(1.5, 53.5, 10, 46.5, 2, 1, "Join"), // ribs between the cells
	}},
	"battery-2s": {ID: "battery-2s", Label: "2S Li-ion pack (2 × 18650)", prims: []prim{
		box(37, 18.5, 65, 0, 0, 0, ""),        // shrink-wrapped
		box(10, 6, 4, 13.5, 6.25, 64, "Join"), // the balance lead
	}},
	"tb6612": {ID: "tb6612", Label: "TB6612 driver board", prims: []prim{
		box(20.3, 20.3, 1.6, 0, 0, 0, ""),
		box(20.3, 2.5, 8.5, 0, 0.5, -7, "Join"), box(20.3, 2.5, 8.5, 0, 17.3, -7, "Join"),
		box(6.5, 5.5, 1.2, 6.9, 7.4, 1.2, "Join"),
	}},
	"l298n": {ID: "l298n", Label: "L298N driver board", prims: []prim{
		box(43, 43, 1.6, 0, 0, 0, ""),
		cyl(1.5, 3, 3, 3, -0.7, "Cut"), cyl(1.5, 3, 40, 3, -0.7, "Cut"),
		cyl(1.5, 3, 3, 40, -0.7, "Cut"), cyl(1.5, 3, 40, 40, -0.7, "Cut"),
		box(23, 16, 25, 10, 26, 1.2, "Join"), // the heat sink
		box(10, 8, 10, 0.5, 9, 1.2, "Join"), box(10, 8, 10, 32.5, 9, 1.2, "Join"),
	}},
}

// PartModelIDs are the 3D models in order.
func PartModelIDs() []string {
	ids := make([]string, 0, len(PartModels))
	for id := range PartModels {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func num(x float64) string { return strconv.FormatFloat(x, 'f', -1, 64) }

// PartDesign builds a part model's design.
func PartDesign(id string) (*Document, error) {
	m := PartModels[id]
	if m == nil {
		return nil, fmt.Errorf("no 3D model of %q", id)
	}
	d := New()
	for _, p := range m.prims {
		f := d.Insert(NewPrim(p.kind))
		names := []string{"Width", "Depth", "Height", "X", "Y", "Z"}
		if p.kind == KCylinder {
			names = []string{"Radius", "Height", "X", "Y", "Z"}
		}
		f.Inputs = map[string]string{}
		for i, n := range names {
			f.Inputs[n] = num(p.v[i])
		}
		if p.op != "" {
			f.Inputs["Operation"] = p.op
		}
	}
	d.Regenerate()
	for _, f := range d.Features {
		if f.Err != "" {
			return nil, fmt.Errorf("%s: %s", f.Name, f.Err)
		}
	}
	return d, nil
}

// NewPartComponent is a part model as a component.
func NewPartComponent(id string) (*Feature, error) {
	d, err := PartDesign(id)
	if err != nil {
		return nil, err
	}
	f := NewComponent(PartModels[id].Label+".forma", d.Marshal())
	return f, nil
}
