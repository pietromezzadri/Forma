package app

import (
	"path/filepath"
	"strings"

	imgui "github.com/AllenDang/cimgui-go/imgui"
)

// unsavedAsk is what waits on the unsaved-changes question: what it is
// (said in the question) and what to do once the changes are saved or let go.
type unsavedAsk struct {
	before string // "closing", "starting a new design", …
	then   func()
}

// whenSaved runs then at once when nothing is unsaved (or a capture script
// runs the command), and otherwise asks first: save the changes, let them
// go, or stay.
func (a *App) whenSaved(before string, then func()) {
	if !a.doc.Dirty() || a.scripted {
		then()
		return
	}
	a.unsaved = &unsavedAsk{before: before, then: then}
	a.askNow = true
}

// mayClose says whether the main loop may end: the window's close button or
// Quit was used and nothing is left unsaved, or the user answered the
// dialog. Closing with unsaved changes is held back and the dialog asked
// instead.
func (a *App) mayClose() bool {
	if !a.win.ShouldClose() && !a.quit {
		return false
	}
	if a.leave || !a.doc.Dirty() {
		return true
	}
	a.win.SetShouldClose(false)
	a.quit = false
	a.unsaved = &unsavedAsk{before: "closing", then: func() { a.leave, a.quit = true, true }}
	a.askNow = true
	return false
}

// closeDialog asks what to do with unsaved changes before the window is
// closed or the design replaced (New, Open, Recover autosave): save them
// (picking a file if the design has none), let them go, or stay.
func (a *App) closeDialog() {
	const id = "Unsaved changes###closeask"
	if a.askNow {
		a.askNow = false
		imgui.OpenPopupStr(id)
	}
	vp := imgui.MainViewport()
	imgui.SetNextWindowPosV(imgui.Vec2{X: vp.Pos().X + vp.Size().X/2, Y: vp.Pos().Y + vp.Size().Y/2}, imgui.CondAppearing, imgui.Vec2{X: 0.5, Y: 0.5})
	if !imgui.BeginPopupModalV(id, nil, imgui.WindowFlagsAlwaysAutoResize|imgui.WindowFlagsNoSavedSettings|imgui.WindowFlagsNoMove) {
		return
	}
	defer imgui.EndPopup()
	ask := a.unsaved
	if ask == nil {
		imgui.CloseCurrentPopup()
		return
	}
	name := "Untitled"
	if a.doc.Path != "" {
		name = strings.TrimSuffix(filepath.Base(a.doc.Path), ".forma")
	}
	imgui.Text("Save the changes to " + name + " before " + ask.before + "?")
	imgui.TextDisabled("Your changes are lost if you don't save them.")
	imgui.Spacing()
	if imgui.Button("Save") || imgui.IsKeyPressedBool(imgui.KeyEnter) {
		imgui.CloseCurrentPopup()
		a.unsaved = nil
		a.saveThen(ask.then)
	}
	a.markItem("btn:close.save")
	imgui.SameLine()
	if imgui.Button("Don't save") {
		imgui.CloseCurrentPopup()
		a.unsaved = nil
		ask.then()
	}
	a.markItem("btn:close.discard")
	imgui.SameLine()
	if imgui.Button("Cancel") || imgui.IsKeyPressedBool(imgui.KeyEscape) {
		imgui.CloseCurrentPopup()
		a.unsaved = nil
	}
	a.markItem("btn:close.cancel")
}

// saveThen saves the design and then goes on; a design with no file yet
// asks for one first, and a cancelled or failed save goes no further.
func (a *App) saveThen(then func()) {
	done := func(path string) {
		a.saveFile(path)
		if !a.doc.Dirty() {
			then()
		}
	}
	if a.doc.Path != "" {
		done(a.doc.Path)
		return
	}
	a.pickFile(true, "Save design", "Save", defaultPath("design.forma"), designFilter, done)
}
