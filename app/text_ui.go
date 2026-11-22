package app

import (
	"math"
	"strings"
	"sync"

	imgui "github.com/AllenDang/cimgui-go/imgui"

	"forma/sketch"
	"forma/ui"
)

var fontList struct {
	sync.Once
	names []string
}

func fonts() []string {
	fontList.Do(func() { fontList.names = sketch.FontNames() })
	return fontList.names
}

// textProperties edits a sketch's texts: what each says (or =parameter),
// its font, height, angle and alignment.
func (a *App) textProperties(s *SketchItem) {
	var texts []*sketch.Entity
	for _, e := range s.S.Ents {
		if e.Kind == sketch.Text {
			texts = append(texts, e)
		}
	}
	if len(texts) == 0 {
		return
	}
	imgui.SeparatorText(ui.Icon(0xf031, "Text"))
	edited := func() {
		s.markDirty()
		a.doc.changed = true
		if a.sk != nil && a.sk.item == s {
			s.S.Solve()
		}
	}
	commit := func() {
		if imgui.IsItemDeactivatedAfterEdit() {
			a.doc.Commit()
		}
	}
	for i, e := range texts {
		imgui.PushIDInt(int32(e.ID))
		if i > 0 {
			imgui.Separator()
		}
		buf := e.Str
		imgui.SetNextItemWidth(-1)
		if imgui.InputTextMultiline("##text", &buf, imgui.Vec2{X: -1, Y: imgui.TextLineHeight()*2.6 + 8}, 0, nil) {
			e.Str = buf
			edited()
		}
		commit()
		a.markItemLeft("btn:text.edit")
		if strings.HasPrefix(e.Str, "=") {
			name := strings.TrimSpace(e.Str[1:])
			if t, ok := a.doc.D.Texts()[name]; ok {
				imgui.TextDisabled("says \"" + t + "\" (parameter " + name + ")")
			} else {
				imgui.TextColored(colErr, "no text parameter \""+name+"\"")
			}
		} else {
			imgui.TextDisabled("=name uses a text parameter")
		}
		// the design's text parameters, to pick from
		var params []string
		for _, p := range a.doc.D.Params {
			if p.IsText {
				params = append(params, p.Name)
			}
		}
		if len(params) > 0 {
			imgui.SetNextItemWidth(-1)
			cur := "Use a parameter…"
			if imgui.BeginCombo("##param", cur) {
				for _, n := range params {
					if imgui.SelectableBool(n + "  (\"" + a.doc.D.Texts()[n] + "\")") {
						e.Str = "=" + n
						edited()
						a.doc.Commit()
					}
				}
				imgui.EndCombo()
			}
		}
		// font
		cur := e.Font
		if cur == "" {
			cur = sketch.DefaultFont
		}
		imgui.SetNextItemWidth(-60)
		if imgui.BeginCombo("Font", cur) {
			for _, n := range fonts() {
				if imgui.SelectableBoolV(n, n == cur, 0, imgui.Vec2{}) {
					e.Font = n
					edited()
					a.doc.Commit()
				}
			}
			imgui.EndCombo()
		}
		size := float32(e.Size)
		imgui.SetNextItemWidth(-60)
		if imgui.InputFloatV("Height", &size, 0.5, 5, "%.2f mm", 0) && size > 0 {
			e.Size = float64(size)
			edited()
		}
		commit()
		deg := float32(e.Rot * 180 / math.Pi)
		imgui.SetNextItemWidth(-60)
		if imgui.InputFloatV("Angle", &deg, 5, 45, "%.1f°", 0) {
			e.Rot = float64(deg) * math.Pi / 180
			edited()
		}
		commit()
		for k, n := range []string{"Left", "Centre", "Right"} {
			if k > 0 {
				imgui.SameLine()
			}
			if imgui.RadioButtonBool(n+"##al", e.Align == k) && e.Align != k {
				e.Align = k
				edited()
				a.doc.Commit()
			}
		}
		imgui.PopID()
	}
}

// registerTextCommands: "text.set:<text>" and "text.font:<name>" on the
// texts selected in the sketch being edited (every text in it if none is).
func (a *App) registerTextCommands(c *Commands) {
	texts := func() []*sketch.Entity {
		if a.sk == nil {
			return nil
		}
		s := a.sk.item.S
		var sel, all []*sketch.Entity
		for _, e := range s.Ents {
			if e.Kind != sketch.Text {
				continue
			}
			all = append(all, e)
			for _, r := range a.sel {
				if r.Item == a.sk.item.ID && r.Local == e.ID {
					sel = append(sel, e)
				}
			}
		}
		if len(sel) > 0 {
			return sel
		}
		return all
	}
	apply := func(f func(e *sketch.Entity)) {
		ts := texts()
		for _, e := range ts {
			f(e)
		}
		if len(ts) > 0 {
			a.sk.changed()
		}
	}
	c.add(&Command{ID: "text.set", Label: "Set the text", Group: "Sketch",
		Run: func(arg string) { apply(func(e *sketch.Entity) { e.Str = arg }) }})
	c.add(&Command{ID: "text.font", Label: "Set the text's font", Group: "Sketch",
		Run: func(arg string) { apply(func(e *sketch.Entity) { e.Font = arg }) }})
	c.add(&Command{ID: "text.size", Label: "Set the text's height", Group: "Sketch",
		Run: func(arg string) {
			if v, err := a.doc.D.Eval(arg); err == nil && v > 0 {
				apply(func(e *sketch.Entity) { e.Size = v })
			}
		}})
	c.add(&Command{ID: "text.align", Label: "Align the text (left, centre, right)", Group: "Sketch",
		Run: func(arg string) {
			k := map[string]int{"left": 0, "centre": 1, "center": 1, "right": 2}[strings.ToLower(strings.TrimSpace(arg))]
			apply(func(e *sketch.Entity) { e.Align = k })
		}})
}
