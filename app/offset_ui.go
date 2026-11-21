package app

import (
	"forma/sketch"
)

const icOffset = 0xf24d // clone

// registerOffsetCommands adds the sketch's offset tool: the selected chain
// of curves copied to one side, its distance a dimension.
func (a *App) registerOffsetCommands(c *Commands) {
	c.add(&Command{ID: "sketch.offset", Label: "Offset curves", Group: "Sketch", Shortcut: "O", Icon: icOffset,
		Enabled: func() bool { return len(a.offsetPicks()) > 0 },
		Run:     func(string) { a.offsetSelection() }})
}

// offsetPicks are the selected curves an offset can take (not those of a
// copy, not texts or points).
func (a *App) offsetPicks() []int {
	if a.sk == nil {
		return nil
	}
	s := a.sk.item.S
	var out []int
	for _, id := range a.selectedSketchEnts(a.sk.item.ID) {
		e := s.Ent(id)
		if e == nil || e.Off != 0 {
			continue
		}
		switch e.Kind {
		case sketch.Line, sketch.Arc, sketch.Spline, sketch.Circle:
			out = append(out, id)
		}
	}
	return out
}

// offsetSelection offsets the selected curves 2 mm (out from a loop, left
// of a chain) and opens the distance for typing; a negative distance puts
// the copy on the other side.
func (a *App) offsetSelection() {
	sm := a.sk
	ids := a.offsetPicks()
	if sm == nil || len(ids) == 0 {
		a.notify("Select the curves to offset")
		return
	}
	c, err := sm.item.S.AddOffset(ids, 2)
	if err != nil {
		a.notify("Offset: %v", err)
		return
	}
	sm.changed()
	sm.openEdit(c.ID)
}
