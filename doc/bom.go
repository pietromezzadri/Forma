package doc

// The bill of materials: one row per part — each component's design, its
// copies counted together, and each body made in this design itself.

import (
	"encoding/csv"
	"fmt"
	"hash/fnv"
	"io"
	"strconv"

	"forma/brep"
)

// BOMRow is a line of the bill of materials. Volume and Area are one
// item's; mass comes from a density when it is listed.
type BOMRow struct {
	Item   int
	Name   string
	File   string // the component's file ("" for a body made here)
	Qty    int
	Bodies int // bodies in one item
	Volume float64
	Area   float64
	Comp   int  // the component (its source's ID), 0 for a body made here
	Ground bool // a copy of it is grounded
}

// BOM lists the design's parts in timeline order: components by their
// source (copies made with Copy, and the same file inserted again, counted
// as one part), then the bodies of the design's own features. A component
// whose bodies a join or cut has changed counts as plain bodies (A4).
func (d *Document) BOM() []BOMRow {
	var rows []BOMRow
	byPart := map[string]int{} // a part (its file and its design) → its row
	seen := map[int]bool{}     // components counted
	for _, r := range d.Bodies {
		if r.Comp == 0 || seen[r.Comp] {
			continue
		}
		seen[r.Comp] = true
		c := d.Feature(r.Comp)
		if c == nil {
			continue
		}
		src := c
		if c.Ref != 0 {
			if s := d.Feature(c.Ref); s != nil {
				src = s
			}
		}
		// copies, and components inserted from the same file as it stands,
		// are one part
		data, _ := d.componentData(c)
		h := fnv.New64a()
		h.Write([]byte(data))
		key := fmt.Sprintf("%s|%x", src.Inputs["File"], h.Sum64())
		k, ok := byPart[key]
		if !ok {
			k = len(rows)
			byPart[key] = k
			row := BOMRow{Name: src.Name, File: src.Inputs["File"], Comp: src.ID}
			for _, b := range c.Local {
				mp := brep.Mass(b, 1e-4)
				row.Volume += mp.Volume
				row.Area += mp.Area
				row.Bodies++
			}
			rows = append(rows, row)
		}
		rows[k].Qty++
		rows[k].Ground = rows[k].Ground || Grounded(c)
	}
	for _, r := range d.Bodies {
		if r.Comp != 0 {
			continue
		}
		mp := brep.Mass(r.Body, 1e-4)
		rows = append(rows, BOMRow{Name: r.Name, Qty: 1, Bodies: 1, Volume: mp.Volume, Area: mp.Area})
	}
	for i := range rows {
		rows[i].Item = i + 1
	}
	return rows
}

// WriteBOM writes a bill of materials as CSV, with each item's mass and
// the line's at density (g/cm³; 0 leaves the masses out).
func WriteBOM(w io.Writer, rows []BOMRow, density float64) error {
	cw := csv.NewWriter(w)
	head := []string{"Item", "Name", "File", "Quantity", "Bodies", "Volume (mm3)", "Area (mm2)"}
	if density > 0 {
		head = append(head, "Mass each (g)", "Mass (g)")
	}
	if err := cw.Write(head); err != nil {
		return err
	}
	num := func(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }
	for _, r := range rows {
		file := r.File
		if r.Comp == 0 {
			file = "(this design)"
		}
		rec := []string{strconv.Itoa(r.Item), r.Name, file, strconv.Itoa(r.Qty), strconv.Itoa(r.Bodies), num(r.Volume), num(r.Area)}
		if density > 0 {
			m := r.Volume * density / 1000
			rec = append(rec, num(m), num(m*float64(r.Qty)))
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		return fmt.Errorf("bill of materials: %w", err)
	}
	return nil
}
