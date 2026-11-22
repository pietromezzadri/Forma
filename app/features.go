package app

import (
	"os"
	"path/filepath"
	"strings"

	imgui "github.com/AllenDang/cimgui-go/imgui"

	"forma/doc"
	"forma/sketch"
	"forma/ui"
)

// PrimKinds in menu order.
var PrimKinds = []string{doc.KBox, doc.KCylinder, doc.KCone, doc.KSphere, doc.KTorus}

// planeFor names a sketch plane: a standard one, or the selected planar face.
func (a *App) planeFor(name string) (doc.PlaneRef, bool) {
	if _, ok := doc.StdPlane(name); ok {
		return doc.PlaneRef{Std: name}, true
	}
	for _, r := range a.sel {
		if r.Kind != KindFace {
			continue
		}
		b := a.doc.Body(r.Item)
		if b == nil {
			continue
		}
		for _, f := range b.Body.Faces() {
			if f.ID != r.Local {
				continue
			}
			if _, ok := doc.PlaneOf(f); ok {
				return doc.PlaneRef{Feature: b.F.ID, Body: b.K, Face: f.Name}, true
			}
			// a curved face: the plane touching it where it was clicked
			at, ok := a.selAt[r]
			if !ok {
				return doc.PlaneRef{}, false
			}
			if _, ok := doc.TangentPlane(f, at); !ok {
				return doc.PlaneRef{}, false
			}
			return doc.PlaneRef{Feature: b.F.ID, Body: b.K, Face: f.Name, At: &at}, true
		}
	}
	return doc.PlaneRef{}, false
}

// selectedProfiles are the selected sketch regions, as references.
func (a *App) selectedProfiles() []doc.ProfileRef {
	var out []doc.ProfileRef
	for _, r := range a.sel {
		if r.Kind == KindSkCurve {
			// a text picked is the whole text as a profile
			if s := a.doc.Sketch(r.Item); s != nil && s.F != nil {
				if e := s.S.Ent(r.Local); e != nil && e.Kind == sketch.Text {
					dup := false
					for _, o := range out {
						dup = dup || o.Text == e.ID && o.Sketch == s.F.ID
					}
					if !dup {
						out = append(out, doc.ProfileRef{Sketch: s.F.ID, Outer: []int{e.ID}, Text: e.ID})
					}
				}
			}
			continue
		}
		if r.Kind != KindSkRegion {
			continue
		}
		s := a.doc.Sketch(r.Item)
		if s == nil || r.Local-1 >= len(s.Regions) {
			continue
		}
		ref := doc.RefFor(s.F.ID, s.Regions[r.Local-1])
		dup := false
		for _, o := range out {
			// any letter of a text means the whole text, once
			dup = dup || ref.Text != 0 && o.Text == ref.Text && o.Sketch == ref.Sketch
		}
		if !dup {
			out = append(out, ref)
		}
	}
	return out
}

// selFeature is the feature picked in the browser or timeline.
func (a *App) selFeature() *doc.Feature {
	if a.browserSel == 0 {
		return nil
	}
	return a.doc.D.Feature(featureOf(a.browserSel))
}

// newSweep extrudes or revolves the selected profiles (arg: inputs as
// "Name=expr,…").
func (a *App) newSweep(kind, arg string) {
	profiles := a.selectedProfiles()
	if len(profiles) == 0 {
		a.notify("Select one or more profiles (closed regions of a sketch) first")
		return
	}
	axis := doc.AxisRef{Kind: "y"}
	for _, r := range a.sel {
		if r.Kind != KindSkCurve {
			continue
		}
		if s := a.doc.Sketch(r.Item); s != nil && s.F.ID == profiles[0].Sketch {
			if e := s.S.Ent(r.Local); e != nil && e.Kind == sketch.Line {
				axis = doc.AxisRef{Kind: "line", Line: e.ID}
			}
		}
	}
	if a.sk != nil {
		a.finishSketch()
	}
	var f *doc.Feature
	if kind == doc.KExtrude {
		f = doc.NewExtrude(profiles)
	} else {
		f = doc.NewRevolve(profiles, axis)
	}
	// a profile sketched on a body joins it by default, as in Fusion
	if sk := a.doc.D.Feature(profiles[0].Sketch); sk != nil && sk.Plane != nil && sk.Plane.Feature != 0 {
		f.Inputs["Operation"] = "Join"
	}
	for _, kv := range strings.Split(arg, ",") {
		if name, val, ok := strings.Cut(kv, "="); ok {
			f.Inputs[strings.TrimSpace(name)] = strings.TrimSpace(val)
		}
	}
	// the sketch is used now: hide it, as Fusion does
	if sk := a.doc.D.Feature(profiles[0].Sketch); sk != nil {
		sk.Hidden = true
	}
	a.doc.D.Insert(f)
	a.doc.Commit()
	a.sel = a.sel[:0]
	a.browserSel = bodyKey(f.ID, 0)
	if f.Err != "" {
		a.notify("%s: %s", f.Name, f.Err)
	}
}

// undo steps back (or forward) through the history.
func (a *App) undo(redo bool) {
	var nd *doc.Document
	var err error
	if redo {
		nd, err = a.doc.hist.Redo()
	} else {
		nd, err = a.doc.hist.Undo()
	}
	if err != nil {
		a.notify("%v", err)
		return
	}
	a.doc.replace(nd)
	a.sel, a.hover = a.sel[:0], Ref{}
	if a.sk != nil {
		if it := a.doc.Sketch(a.sk.item.ID); it != nil {
			a.sk.item = it
			a.sk.cancel()
			a.sk.editOpen = false
		} else {
			a.sk = nil
		}
	}
	if a.doc.Body(a.browserSel) == nil && a.doc.Sketch(a.browserSel) == nil && a.selFeature() == nil {
		a.browserSel = 0
	}
	a.updateTitle()
}

// rollTo moves the timeline marker after n features (-1: the end).
func (a *App) rollTo(n int) {
	if a.sk != nil {
		a.finishSketch()
	}
	if n >= len(a.doc.D.Features) {
		n = -1
	}
	a.doc.D.Rollback = n
	a.doc.Commit()
}

func pickStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func defaultPath(name string) string {
	dir, _ := os.UserHomeDir()
	return filepath.Join(dir, "Documents", "Forma", name)
}

func (a *App) saveFile(path string) {
	if !strings.HasSuffix(strings.ToLower(path), ".forma") {
		path += ".forma"
	}
	if err := a.doc.D.Save(path); err != nil {
		a.notify("Save failed: %v", err)
		return
	}
	a.doc.Path = path
	a.doc.saved = a.doc.D.Marshal()
	a.updateTitle()
	a.notify("Saved %s", path)
}

func (a *App) openFile(path string) {
	nd, err := doc.Open(path)
	if err != nil {
		a.notify("Open failed: %v", err)
		return
	}
	a.sk = nil
	a.doc.replace(nd)
	a.doc.hist = doc.NewHistory(nd)
	a.doc.saved = nd.Marshal()
	a.doc.Path = path
	if path == autosavePath() {
		a.doc.Path = ""
		a.doc.saved = nil
	}
	a.sel, a.hover, a.browserSel = a.sel[:0], Ref{}, 0
	a.cam.Fit(a.doc.Bounds(), a.aspect(), false)
	a.updateTitle()
	a.notify("Opened %s", filepath.Base(path))
}

func (a *App) updateTitle() {
	name := "Untitled"
	if a.doc.Path != "" {
		name = strings.TrimSuffix(filepath.Base(a.doc.Path), ".forma")
	}
	mark := ""
	if a.doc.Dirty() {
		mark = " ●"
	}
	t := name + mark + " — Forma " + Version
	if t != a.title {
		a.title = t
		a.win.SetTitle(t)
	}
}

func autosavePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "Forma", "autosave.forma")
}

func autosaveExists() bool {
	_, err := os.Stat(autosavePath())
	return err == nil
}

// autosave writes the design every minute while it has unsaved changes.
func (a *App) autosave(dt float64) {
	if a.capture != nil {
		return
	}
	a.autoT += dt
	if a.autoT < 60 {
		return
	}
	a.autoT = 0
	if !a.doc.Dirty() {
		return
	}
	b := a.doc.D.Marshal()
	if string(b) == string(a.lastAuto) {
		return
	}
	if err := a.doc.D.Save(autosavePath()); err == nil {
		a.lastAuto = b
	}
}

// cleanExit drops the autosave when nothing is left unsaved.
func (a *App) cleanExit() {
	if a.capture == nil && !a.doc.Dirty() {
		os.Remove(autosavePath())
	}
}

// fileDialog asks for a path (no native dialogs yet).
type fileDialog struct {
	show   bool
	title  string
	button string
	path   string
	fn     func(string)
}

func (d *fileDialog) ask(title, button, path string, fn func(string)) {
	d.show, d.title, d.button, d.path, d.fn = true, title, button, path, fn
}

func (d *fileDialog) draw() {
	if !d.show {
		return
	}
	imgui.SetNextWindowSizeV(imgui.Vec2{X: 560, Y: 0}, imgui.CondAlways)
	if imgui.BeginV(d.title+"###filedlg", &d.show, imgui.WindowFlagsNoResize|imgui.WindowFlagsNoSavedSettings) {
		imgui.SetNextItemWidth(-1)
		if imgui.IsWindowAppearing() {
			imgui.SetKeyboardFocusHere()
		}
		enter := imgui.InputTextWithHint("##path", "file path", &d.path, imgui.InputTextFlagsEnterReturnsTrue, nil)
		if imgui.Button(ui.Icon(ui.FaCheck, d.button)) || enter {
			d.show = false
			d.fn(d.path)
		}
		imgui.SameLine()
		if imgui.Button("Cancel") || imgui.IsKeyPressedBool(imgui.KeyEscape) {
			d.show = false
		}
	}
	imgui.End()
}
