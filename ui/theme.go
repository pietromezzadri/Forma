package ui

import (
	"os"
	"path/filepath"

	imgui "github.com/AllenDang/cimgui-go/imgui"
)

// The look, after Attalo's engine/ui_theme.go: dark slate, a warm amber
// accent, rounded frames, the Inter typeface with Font Awesome merged in.

func rgba(r, g, b, a float32) imgui.Vec4 { return imgui.Vec4{X: r, Y: g, Z: b, W: a} }

// Accent is the UI's highlight colour (also used in the viewport).
var Accent = rgba(0.88, 0.62, 0.28, 1)

// ApplyTheme sets the style; scale is the DPI scale.
func ApplyTheme(scale float32) {
	s := imgui.CurrentStyle()
	s.SetWindowRounding(6)
	s.SetChildRounding(5)
	s.SetFrameRounding(4)
	s.SetPopupRounding(6)
	s.SetGrabRounding(4)
	s.SetTabRounding(4)
	s.SetScrollbarRounding(6)
	s.SetScrollbarSize(12)
	s.SetWindowBorderSize(1)
	s.SetFrameBorderSize(0)
	s.SetPopupBorderSize(1)
	s.SetDockingSeparatorSize(2)
	s.SetWindowPadding(imgui.Vec2{X: 10, Y: 8})
	s.SetFramePadding(imgui.Vec2{X: 8, Y: 4})
	s.SetItemSpacing(imgui.Vec2{X: 8, Y: 6})
	s.SetItemInnerSpacing(imgui.Vec2{X: 6, Y: 4})
	s.SetWindowTitleAlign(imgui.Vec2{X: 0.02, Y: 0.5})

	bg := rgba(0.085, 0.095, 0.115, 1)
	panel := rgba(0.11, 0.125, 0.15, 1)
	frame := rgba(0.16, 0.18, 0.22, 1)
	hover := rgba(0.22, 0.25, 0.30, 1)
	accentDim := rgba(0.88, 0.62, 0.28, 0.55)
	warm := rgba(0.30, 0.27, 0.22, 1)
	c := s.Colors()
	set := func(k imgui.Col, v imgui.Vec4) { c[k] = v }
	set(imgui.ColText, rgba(0.93, 0.92, 0.89, 1))
	set(imgui.ColTextDisabled, rgba(0.55, 0.56, 0.58, 1))
	set(imgui.ColWindowBg, bg)
	set(imgui.ColChildBg, rgba(0, 0, 0, 0))
	set(imgui.ColPopupBg, rgba(0.095, 0.105, 0.13, 0.98))
	set(imgui.ColBorder, rgba(0.30, 0.32, 0.36, 0.55))
	set(imgui.ColBorderShadow, rgba(0, 0, 0, 0))
	set(imgui.ColFrameBg, frame)
	set(imgui.ColFrameBgHovered, hover)
	set(imgui.ColFrameBgActive, rgba(0.27, 0.30, 0.36, 1))
	set(imgui.ColTitleBg, panel)
	set(imgui.ColTitleBgActive, rgba(0.15, 0.17, 0.21, 1))
	set(imgui.ColTitleBgCollapsed, panel)
	set(imgui.ColMenuBarBg, panel)
	set(imgui.ColScrollbarBg, rgba(0, 0, 0, 0.15))
	set(imgui.ColScrollbarGrab, rgba(0.30, 0.32, 0.37, 1))
	set(imgui.ColScrollbarGrabHovered, rgba(0.38, 0.40, 0.46, 1))
	set(imgui.ColScrollbarGrabActive, accentDim)
	set(imgui.ColCheckMark, Accent)
	set(imgui.ColSliderGrab, accentDim)
	set(imgui.ColSliderGrabActive, Accent)
	set(imgui.ColButton, rgba(0.19, 0.21, 0.26, 1))
	set(imgui.ColButtonHovered, warm)
	set(imgui.ColButtonActive, accentDim)
	set(imgui.ColHeader, rgba(0.20, 0.22, 0.27, 1))
	set(imgui.ColHeaderHovered, warm)
	set(imgui.ColHeaderActive, accentDim)
	set(imgui.ColSeparator, rgba(0.30, 0.32, 0.36, 0.6))
	set(imgui.ColSeparatorHovered, accentDim)
	set(imgui.ColSeparatorActive, Accent)
	set(imgui.ColResizeGrip, rgba(0.30, 0.32, 0.37, 0.4))
	set(imgui.ColResizeGripHovered, accentDim)
	set(imgui.ColResizeGripActive, Accent)
	set(imgui.ColTab, panel)
	set(imgui.ColTabHovered, warm)
	set(imgui.ColTabSelected, rgba(0.24, 0.22, 0.19, 1))
	set(imgui.ColTabSelectedOverline, Accent)
	set(imgui.ColTabDimmed, panel)
	set(imgui.ColTabDimmedSelected, rgba(0.18, 0.18, 0.18, 1))
	set(imgui.ColDockingPreview, accentDim)
	set(imgui.ColDockingEmptyBg, bg)
	set(imgui.ColPlotLines, Accent)
	set(imgui.ColPlotHistogram, Accent)
	set(imgui.ColTableHeaderBg, panel)
	set(imgui.ColTextSelectedBg, rgba(0.88, 0.62, 0.28, 0.3))
	set(imgui.ColNavCursor, Accent)
	set(imgui.ColModalWindowDimBg, rgba(0, 0, 0, 0.55))
	s.SetColors(&c)
	if scale > 0 && scale != 1 {
		s.ScaleAllSizes(scale)
		s.SetFontScaleDpi(scale)
	}
}

// Icons is set once the icon font is loaded.
var Icons bool

// LoadFonts adds Inter with Font Awesome merged in, found under assets/fonts
// next to the executable or the working directory; without them ImGui's
// built-in font stays.
func LoadFonts(px float32) {
	dir := findAssets()
	if dir == "" {
		return
	}
	atlas := imgui.CurrentIO().Fonts()
	cfg := imgui.NewFontConfig()
	f := atlas.AddFontFromFileTTFV(filepath.Join(dir, "fonts", "Inter-Medium.ttf"), px, cfg, nil)
	if f == nil {
		return
	}
	icfg := imgui.NewFontConfig()
	icfg.SetMergeMode(true)
	icfg.SetPixelSnapH(true)
	icfg.SetGlyphMinAdvanceX(px)
	icfg.SetGlyphOffset(imgui.Vec2{X: 0, Y: 1})
	if atlas.AddFontFromFileTTFV(filepath.Join(dir, "fonts", "fa-solid-900.ttf"), px*0.9, icfg, nil) != nil {
		Icons = true
	}
	imgui.CurrentIO().SetFontDefault(f)
}

// findAssets locates the assets folder.
func findAssets() string {
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Join(filepath.Dir(exe), "assets"))
	}
	dirs = append(dirs, "assets")
	for _, d := range dirs {
		if _, err := os.Stat(filepath.Join(d, "fonts", "Inter-Medium.ttf")); err == nil {
			return d
		}
	}
	return ""
}

// Icon prefixes a label with a Font Awesome glyph.
func Icon(code rune, label string) string {
	if !Icons {
		return label
	}
	if label == "" {
		return string(code)
	}
	return string(code) + "  " + label
}

// Font Awesome 6 code points used by the UI.
const (
	FaCube      = 0xf1b2
	FaCircle    = 0xf111
	FaPen       = 0xf304
	FaRuler     = 0xf545
	FaFloppy    = 0xf0c7
	FaFolder    = 0xf07c
	FaFile      = 0xf15b
	FaDownload  = 0xf019
	FaEye       = 0xf06e
	FaEyeSlash  = 0xf070
	FaTrash     = 0xf1f8
	FaHouse     = 0xf015
	FaExpand    = 0xf065
	FaSearch    = 0xf002
	FaSlash     = 0xf715
	FaSquare    = 0xf0c8
	FaDotCircle = 0xf192
	FaCheck     = 0xf00c
	FaXmark     = 0xf00d
	FaLink      = 0xf0c1
	FaLock      = 0xf023
	FaTerminal  = 0xf120
	FaLayer     = 0xf5fd
	FaRotate    = 0xf2f1
	FaBezier    = 0xf55b
	FaGrip      = 0xf58d
	FaArrowsH   = 0xf337
	FaArrowsV   = 0xf338
	FaKeyboard  = 0xf11c
	FaQuestion  = 0xf059
	FaCamera    = 0xf030
	FaDiamond   = 0xf219
	FaRing      = 0xf70b
	FaGlobe     = 0xf0ac
	FaClock     = 0xf017
)
