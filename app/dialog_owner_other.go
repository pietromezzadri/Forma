//go:build !windows

package app

import "github.com/ncruces/zenity"

// dialogOwner: off Windows the dialog stands on its own (zenity can attach
// it only to a Windows window or, on Linux, an X11 one by its ID).
func (a *App) dialogOwner() []zenity.Option { return nil }
