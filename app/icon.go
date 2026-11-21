package app

import (
	"embed"
	"image"
	"image/png"
	"log"

	"github.com/go-gl/glfw/v3.4/glfw"
)

// The window icon at the sizes the title bar and taskbar ask for
// (made by tools/icon; the executable's own icon is rsrc_windows_amd64.syso).
//
//go:embed icons/forma_16.png icons/forma_32.png icons/forma_48.png icons/forma_256.png
var iconFiles embed.FS

func setWindowIcon(win *glfw.Window) {
	var imgs []image.Image
	for _, n := range []string{"16", "32", "48", "256"} {
		f, err := iconFiles.Open("icons/forma_" + n + ".png")
		if err != nil {
			continue
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			log.Printf("icon %s: %v", n, err)
			continue
		}
		imgs = append(imgs, img)
	}
	if len(imgs) > 0 {
		win.SetIcon(imgs)
	}
}
