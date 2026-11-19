package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"github.com/ncruces/zenity"
)

// Native file dialogs: Windows' own Open and Save As windows (zenity: the
// IFileOpenDialog / IFileSaveDialog COM dialogs), shown on a goroutine of
// their own and modal to Forma's window, so it keeps drawing; the chosen
// path comes back to the main loop through post. Captures keep the in-app
// path box (they must never pop a window up).

var (
	designFilter = zenity.FileFilters{{Name: "Forma designs (*.forma)", Patterns: []string{"*.forma"}}}
	exportFilter = zenity.FileFilters{
		{Name: "STEP (*.step, *.stp)", Patterns: []string{"*.step", "*.stp"}},
		{Name: "STL mesh (*.stl)", Patterns: []string{"*.stl"}},
		{Name: "Wavefront OBJ (*.obj)", Patterns: []string{"*.obj"}},
	}
	importFilter = zenity.FileFilters{
		{Name: "STEP and STL (*.step, *.stp, *.stl)", Patterns: []string{"*.step", "*.stp", "*.stl"}},
		{Name: "STEP (*.step, *.stp)", Patterns: []string{"*.step", "*.stp"}},
		{Name: "STL mesh (*.stl)", Patterns: []string{"*.stl"}},
	}
)

// pickFile asks for a file to open (save false) or save, starting at path
// (a file, or a folder ending in a separator), and hands the choice to fn
// on the main loop. Nothing happens when the user cancels.
func (a *App) pickFile(save bool, title, button, path string, filters zenity.FileFilters, fn func(string)) {
	if a.capture != nil {
		a.files.ask(title, button, path, fn)
		return
	}
	if a.dialogOpen {
		return // one at a time
	}
	a.dialogOpen = true
	if dir := filepath.Dir(path); dir != "" {
		_ = os.MkdirAll(dir, 0o755) // the dialog falls back elsewhere for a missing folder
	}
	opts := []zenity.Option{zenity.Title(title), zenity.Filename(path), filters}
	if hwnd := a.win.GetWin32Window(); hwnd != nil {
		opts = append(opts, zenity.Attach(uintptr(unsafe.Pointer(hwnd))))
	}
	if save {
		opts = append(opts, zenity.ConfirmOverwrite())
	}
	go func() {
		var p string
		var err error
		if save {
			p, err = zenity.SelectFileSave(opts...)
		} else {
			p, err = zenity.SelectFile(opts...)
		}
		a.post(func() {
			a.dialogOpen = false
			switch {
			case errors.Is(err, zenity.ErrCanceled):
			case err != nil:
				a.notify("The file dialog failed (%v): type the path instead", err)
				a.files.ask(title, button, path, fn)
			case p != "":
				fn(p)
			}
		})
	}()
}

// post runs f on the main loop at the start of the next frame.
func (a *App) post(f func()) {
	a.postMu.Lock()
	a.posted = append(a.posted, f)
	a.postMu.Unlock()
}

// runPosted runs what other goroutines posted.
func (a *App) runPosted() {
	a.postMu.Lock()
	fs := a.posted
	a.posted = nil
	a.postMu.Unlock()
	for _, f := range fs {
		f()
	}
}

type postQueue struct {
	postMu sync.Mutex
	posted []func()
}

// withExt gives a path the extension def when it has none of the allowed.
func withExt(path, def string, allowed ...string) string {
	ext := strings.ToLower(filepath.Ext(path))
	for _, a := range allowed {
		if ext == a {
			return path
		}
	}
	return path + def
}

// startIn is where a dialog opens: the last path used, else Documents\Forma.
func startIn(last, name string) string {
	if last != "" {
		return last
	}
	if name == "" { // the folder (a trailing separator: no file name filled in)
		return defaultPath("") + string(os.PathSeparator)
	}
	return defaultPath(name)
}
