package doc

import "forma/drawing"

// Sheet finds a drawing sheet by ID.
func (d *Document) Sheet(id int) *drawing.Sheet {
	for _, s := range d.Drawings {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// AddSheet puts a new drawing sheet in the design.
func (d *Document) AddSheet(s *drawing.Sheet) *drawing.Sheet {
	n := 0
	for _, x := range d.Drawings {
		n = max(n, x.ID)
	}
	s.ID = n + 1
	d.Drawings = append(d.Drawings, s)
	return s
}

// RemoveSheet takes a drawing sheet out of the design.
func (d *Document) RemoveSheet(id int) {
	for i, s := range d.Drawings {
		if s.ID == id {
			d.Drawings = append(d.Drawings[:i], d.Drawings[i+1:]...)
			return
		}
	}
}

// DrawingBodies are the bodies a drawing shows: every body of the design
// as it stands whose feature isn't hidden.
func (d *Document) DrawingBodies() []drawing.Body {
	var out []drawing.Body
	for _, r := range d.Bodies {
		if f := d.Feature(r.Owner); f == nil || f.Hidden {
			continue
		}
		out = append(out, drawing.Body{Key: r.Key, Name: r.Name, Body: r.Body, Gen: r.Gen})
	}
	return out
}
