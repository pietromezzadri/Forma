package sketch

// Text in a sketch: an entity whose anchor is an ordinary point (it can be
// dimensioned and constrained) and whose letters are the outlines of a
// TrueType / OpenType font's glyphs — lines and Bézier curves, closed
// contours that make profiles like any other curves. The letters are
// worked out from the text, font, height and angle; they aren't solver
// geometry.

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"forma/geom"
)

// TextAlign is where the anchor sits on the text's baseline.
const (
	AlignLeft = iota
	AlignCenter
	AlignRight
)

// AddText adds text anchored (baseline, by align) at an existing point.
// size is the capital letters' height in mm.
func (s *Sketch) AddText(anchor int, text, fontName string, size float64) *Entity {
	e := &Entity{ID: s.id(), Kind: Text, Pts: []int{anchor}, Str: text, Font: fontName, Size: size}
	s.Ents = append(s.Ents, e)
	return e
}

// TextOf is what a text entity says: its text, or (for "=name") what the
// design resolved the parameter to.
func (e *Entity) TextOf() string {
	if strings.HasPrefix(e.Str, "=") {
		return e.Shown
	}
	return e.Str
}

// FontDirs are searched for a font named by its file's base name.
var FontDirs = defaultFontDirs()

func defaultFontDirs() []string {
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Join(filepath.Dir(exe), "assets", "fonts"))
	}
	dirs = append(dirs, filepath.Join("assets", "fonts"))
	switch runtime.GOOS {
	case "windows":
		win := os.Getenv("WINDIR")
		if win == "" {
			win = `C:\Windows`
		}
		dirs = append(dirs, filepath.Join(win, "Fonts"))
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			dirs = append(dirs, filepath.Join(la, "Microsoft", "Windows", "Fonts"))
		}
	case "darwin":
		dirs = append(dirs, "/System/Library/Fonts", "/Library/Fonts", filepath.Join(os.Getenv("HOME"), "Library", "Fonts"))
	default:
		dirs = append(dirs, "/usr/share/fonts", "/usr/local/share/fonts", filepath.Join(os.Getenv("HOME"), ".fonts"))
	}
	return dirs
}

// DefaultFont is used when a text names none (or one that can't be found).
const DefaultFont = "Inter-Medium"

// FontNames lists the fonts found (base names, sorted).
func FontNames() []string {
	seen := map[string]bool{}
	for _, d := range FontDirs {
		filepath.Walk(d, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			switch strings.ToLower(filepath.Ext(p)) {
			case ".ttf", ".otf", ".ttc":
				seen[strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))] = true
			}
			return nil
		})
	}
	var out []string
	for n := range seen {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

var fontCache = struct {
	sync.Mutex
	m map[string]*sfnt.Font
}{m: map[string]*sfnt.Font{}}

// loadFont finds and parses a font by base name or path.
func loadFont(name string) (*sfnt.Font, error) {
	if name == "" {
		name = DefaultFont
	}
	fontCache.Lock()
	defer fontCache.Unlock()
	if f, ok := fontCache.m[name]; ok {
		return f, nil
	}
	path := ""
	if _, err := os.Stat(name); err == nil {
		path = name
	}
	for _, d := range FontDirs {
		if path != "" {
			break
		}
		for _, ext := range []string{".ttf", ".otf", ".ttc", ".TTF", ".OTF", ".TTC"} {
			p := filepath.Join(d, name+ext)
			if _, err := os.Stat(p); err == nil {
				path = p
				break
			}
		}
	}
	if path == "" {
		return nil, fmt.Errorf("font %q not found", name)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f *sfnt.Font
	if strings.EqualFold(filepath.Ext(path), ".ttc") {
		c, err := sfnt.ParseCollection(b)
		if err != nil {
			return nil, err
		}
		if f, err = c.Font(0); err != nil {
			return nil, err
		}
	} else if f, err = sfnt.Parse(b); err != nil {
		return nil, err
	}
	fontCache.m[name] = f
	return f, nil
}

// TextContours are a text entity's letters as closed loops of segments in
// sketch coordinates (lines, and Béziers with Ctrl set), each Src the
// entity. A font that can't be found falls back to the default one.
func (s *Sketch) TextContours(e *Entity) [][]Seg {
	f, err := loadFont(e.Font)
	if err != nil {
		if f, err = loadFont(DefaultFont); err != nil {
			return nil
		}
	}
	text := e.TextOf()
	if text == "" || e.Size <= 0 {
		return nil
	}
	var buf sfnt.Buffer
	upem := fixed.Int26_6(f.UnitsPerEm()) << 6
	m, err := f.Metrics(&buf, upem, font.HintingNone)
	if err != nil {
		return nil
	}
	cap := float64(m.CapHeight) / 64
	if cap <= 0 {
		cap = float64(m.Ascent) / 64 * 0.7
	}
	k := e.Size / cap
	line := float64(m.Height) / 64 * k
	anchor := s.P(e.Pts[0])
	cs, sn := math.Cos(e.Rot), math.Sin(e.Rot)
	place := func(x, y float64) geom.Vec2 {
		return geom.Vec2{X: anchor.X + x*cs - y*sn, Y: anchor.Y + x*sn + y*cs}
	}
	var out [][]Seg
	for li, row := range strings.Split(text, "\n") {
		// lay the line out in font units first, to align it
		type placed struct {
			idx sfnt.GlyphIndex
			x   float64
		}
		var gs []placed
		x := 0.0
		var prev sfnt.GlyphIndex
		for i, r := range row {
			idx, err := f.GlyphIndex(&buf, r)
			if err != nil || idx == 0 {
				idx, _ = f.GlyphIndex(&buf, '?')
			}
			if i > 0 {
				if kern, err := f.Kern(&buf, prev, idx, upem, font.HintingNone); err == nil {
					x += float64(kern) / 64
				}
			}
			gs = append(gs, placed{idx, x})
			adv, err := f.GlyphAdvance(&buf, idx, upem, font.HintingNone)
			if err == nil {
				x += float64(adv) / 64
			}
			prev = idx
		}
		shift := 0.0
		switch e.Align {
		case AlignCenter:
			shift = -x / 2
		case AlignRight:
			shift = -x
		}
		y0 := -float64(li) * line
		for _, g := range gs {
			segs, err := f.LoadGlyph(&buf, g.idx, upem, nil)
			if err != nil {
				continue
			}
			pt := func(p fixed.Point26_6) geom.Vec2 {
				// font units, y up, then the text's scale, angle and place
				return place((float64(p.X)/64+g.x+shift)*k, y0-float64(p.Y)/64*k)
			}
			var cur []Seg
			var start, at geom.Vec2
			closeLoop := func() {
				if len(cur) == 0 {
					return
				}
				if at.Dist(start) > 1e-9 {
					cur = append(cur, Seg{A: at, B: start, Src: e.ID})
				}
				out = append(out, cur)
				cur = nil
			}
			for _, sg := range segs {
				switch sg.Op {
				case sfnt.SegmentOpMoveTo:
					closeLoop()
					start = pt(sg.Args[0])
					at = start
				case sfnt.SegmentOpLineTo:
					b := pt(sg.Args[0])
					if b.Dist(at) > 1e-9 {
						cur = append(cur, Seg{A: at, B: b, Src: e.ID})
					}
					at = b
				case sfnt.SegmentOpQuadTo:
					c, b := pt(sg.Args[0]), pt(sg.Args[1])
					cur = append(cur, Seg{A: at, B: b, Ctrl: []geom.Vec2{at, c, b}, Src: e.ID})
					at = b
				case sfnt.SegmentOpCubeTo:
					c1, c2, b := pt(sg.Args[0]), pt(sg.Args[1]), pt(sg.Args[2])
					cur = append(cur, Seg{A: at, B: b, Ctrl: []geom.Vec2{at, c1, c2, b}, Src: e.ID})
					at = b
				}
			}
			closeLoop()
		}
	}
	return OffsetContours(out, e.Grow)
}

// SegPoints2 samples a loop of segments as one polyline (the closing point
// left off).
func SegPoints2(l []Seg, chordTol float64) []geom.Vec2 { return loopPoly(l, chordTol) }
