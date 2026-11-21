// Command icon makes Forma's program icons from one large render of the
// mark (docs/logo/forma_mark.svg drawn at 1024 px on a transparent
// background, e.g. by a headless browser):
//
//	go run ./tools/icon <render.png>
//
// It writes app/icons/forma_<n>.png (the window icon, embedded in the
// program) and assets/forma.ico (the executable's icon), then
//
//	go run github.com/akavel/rsrc -ico assets/forma.ico -o rsrc_windows_amd64.syso
//
// puts the .ico into the Windows build.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"
)

// sizes are the icon's sizes; Windows picks among them for the taskbar,
// Explorer and the title bar.
var sizes = []int{16, 24, 32, 48, 64, 128, 256}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: go run ./tools/icon <render.png>")
	}
	f, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	src, err := png.Decode(f)
	f.Close()
	if err != nil {
		log.Fatal(err)
	}

	var pngs [][]byte
	for _, n := range sizes {
		dst := image.NewNRGBA(image.Rect(0, 0, n, n))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
		var buf bytes.Buffer
		if err := png.Encode(&buf, dst); err != nil {
			log.Fatal(err)
		}
		pngs = append(pngs, buf.Bytes())
		write(filepath.Join("app", "icons", fmt.Sprintf("forma_%d.png", n)), buf.Bytes())
	}
	write(filepath.Join("assets", "forma.ico"), ico(pngs))
}

// ico packs PNG images into an .ico file (Vista and later read PNG entries).
func ico(pngs [][]byte) []byte {
	var b bytes.Buffer
	le := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
	le(uint16(0)) // reserved
	le(uint16(1)) // type: icon
	le(uint16(len(pngs)))
	off := 6 + 16*len(pngs)
	for i, p := range pngs {
		n := sizes[i]
		if n >= 256 {
			n = 0 // 0 means 256
		}
		b.WriteByte(byte(n))
		b.WriteByte(byte(n))
		b.WriteByte(0) // no palette
		b.WriteByte(0)
		le(uint16(1))  // planes
		le(uint16(32)) // bits per pixel
		le(uint32(len(p)))
		le(uint32(off))
		off += len(p)
	}
	for _, p := range pngs {
		b.Write(p)
	}
	return b.Bytes()
}

func write(path string, data []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Println("wrote", path)
}
