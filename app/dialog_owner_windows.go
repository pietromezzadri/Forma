package app

import (
	"unsafe"

	"github.com/ncruces/zenity"
)

// dialogOwner makes a file dialog modal to Forma's window.
func (a *App) dialogOwner() []zenity.Option {
	if hwnd := a.win.GetWin32Window(); hwnd != nil {
		return []zenity.Option{zenity.Attach(uintptr(unsafe.Pointer(hwnd)))}
	}
	return nil
}
