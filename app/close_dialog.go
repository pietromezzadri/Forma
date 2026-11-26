package app

import (
	"path/filepath"
	"strings"

	imgui "github.com/AllenDang/cimgui-go/imgui"
)

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
	a.askClose = true
	return false
}

// closeDialog asks what to do with unsaved changes when the window is
// closed: save them (picking a file if the design has none), let them go,
// or stay.
func (a *App) closeDialog() {
	const id = "Unsaved changes###closeask"
	if a.askClose {
		a.askClose = false
		imgui.OpenPopupStr(id)
	}
	vp := imgui.MainViewport()
	imgui.SetNextWindowPosV(imgui.Vec2{X: vp.Pos().X + vp.Size().X/2, Y: vp.Pos().Y + vp.Size().Y/2}, imgui.CondAppearing, imgui.Vec2{X: 0.5, Y: 0.5})
	if !imgui.BeginPopupModalV(id, nil, imgui.WindowFlagsAlwaysAutoResize|imgui.WindowFlagsNoSavedSettings|imgui.WindowFlagsNoMove) {
		return
	}
	defer imgui.EndPopup()
	name := "Untitled"
	if a.doc.Path != "" {
		name = strings.TrimSuffix(filepath.Base(a.doc.Path), ".forma")
	}
	imgui.Text("Save the changes to " + name + " before closing?")
	imgui.TextDisabled("Your changes are lost if you don't save them.")
	imgui.Spacing()
	if imgui.Button("Save") || imgui.IsKeyPressedBool(imgui.KeyEnter) {
		imgui.CloseCurrentPopup()
		a.saveAndClose()
	}
	a.markItem("btn:close.save")
	imgui.SameLine()
	if imgui.Button("Don't save") {
		imgui.CloseCurrentPopup()
		a.leave, a.quit = true, true
	}
	a.markItem("btn:close.discard")
	imgui.SameLine()
	if imgui.Button("Cancel") || imgui.IsKeyPressedBool(imgui.KeyEscape) {
		imgui.CloseCurrentPopup()
	}
	a.markItem("btn:close.cancel")
}

// saveAndClose saves the design and then closes; a design with no file yet
// asks for one first, and staying open is what a cancelled or failed save
// leaves.
func (a *App) saveAndClose() {
	done := func(path string) {
		a.saveFile(path)
		if !a.doc.Dirty() {
			a.leave, a.quit = true, true
		}
	}
	if a.doc.Path != "" {
		done(a.doc.Path)
		return
	}
	a.pickFile(true, "Save design", "Save", defaultPath("design.forma"), designFilter, done)
}
