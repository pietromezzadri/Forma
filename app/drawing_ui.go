package app

// Drawings (M16): the design's 2D drawing sheets. The Drawing button turns
// the viewport into the sheet — standard views with hidden lines, centre
// lines, sections, dimensions and notes, the title block — with a ribbon
// of its own. Sheets live in the design (doc.Document.Drawings) and are
// worked out again from its bodies whenever they change; export writes
// SVG, PDF or DXF.

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	imgui "github.com/AllenDang/cimgui-go/imgui"
	"github.com/ncruces/zenity"

	"forma/drawing"
	"forma/geom"
	"forma/ui"
)

const (
	icDrawing = 0xf568 // drafting compass
	icNote    = 0xf249 // note
	icProject = 0xf0b2 // arrows four ways
	icAngle   = 0xf5ae // pen and ruler
)

var drawingFilter = zenity.FileFilters{
	{Name: "PDF (*.pdf)", Patterns: []string{"*.pdf"}},
	{Name: "SVG (*.svg)", Patterns: []string{"*.svg"}},
	{Name: "DXF (*.dxf)", Patterns: []string{"*.dxf"}},
}

// drawMode is the drawing on screen while the viewport shows a sheet.
type drawMode struct {
	sheet int    // the sheet shown (ID)
	tool  string // "" select; "dim:h|v|a|r|d|angle"; "note"
	picks []drawing.Anchor
	pview int // the view the picks are in

	selKind string // "view", "dim", "note"
	selID   int

	zoom float64   // screen units per sheet mm
	off  geom.Vec2 // screen position (viewport-relative) of the sheet's (0, 0)
	fit  bool      // fit the sheet on the next frame

	cache drawing.Cache
	res   *drawing.Result
	sig   uint64

	drag      string // "view", "dim", "note", "pan" while dragging
	dragID    int
	dragFrom  geom.Vec2 // sheet point where it started
	dragStart []geom.Vec2
	dragMoved bool

	mouse   geom.Vec2 // the cursor on the sheet
	hoverOK bool
	hover   drawing.Anchor // what a dimension tool would pick
	hoverV  int
}

func (a *App) registerDrawingCommands(c *Commands) {
	inDrawing := func() bool { return a.dw != nil }
	viewSel := func() bool {
		return a.dw != nil && a.dw.selKind == "view" && a.dwSheet() != nil && a.dwSheet().View(a.dw.selID) != nil
	}
	c.add(&Command{ID: "drawing.open", Label: "Drawing", Group: "Drawing", Icon: icDrawing,
		Enabled: func() bool { return a.sk == nil },
		Run:     func(arg string) { a.openDrawing(arg) }})
	c.add(&Command{ID: "drawing.close", Label: "Finish drawing (back to the model)", Group: "Drawing", Icon: ui.FaCheck,
		Enabled: inDrawing, Run: func(string) { a.dw = nil }})
	for _, z := range drawing.Sizes {
		size := z.Name
		c.add(&Command{ID: "drawing.sheet." + strings.ReplaceAll(strings.ToLower(size), " ", ""), Label: "New " + size + " sheet with the standard views",
			Group: "Drawing", Run: func(string) { a.newSheet(size) }})
	}
	c.add(&Command{ID: "drawing.sheet", Label: "New drawing sheet", Group: "Drawing", Icon: ui.FaFile,
		Run: func(arg string) { a.newSheet(pickStr(arg, "A3")) }})
	c.add(&Command{ID: "drawing.deletesheet", Label: "Delete this drawing sheet", Group: "Drawing", Icon: ui.FaTrash, Enabled: inDrawing,
		Run: func(string) { a.deleteSheet() }})
	for _, d := range drawing.Dirs {
		dir := d
		c.add(&Command{ID: "drawing.base." + dir, Label: "Base view: " + dirLabel(dir), Group: "Drawing", Enabled: inDrawing,
			Run: func(arg string) { a.addBaseView(dir, arg) }})
	}
	for _, s := range []string{"right", "left", "above", "below"} {
		side := s
		c.add(&Command{ID: "drawing.proj." + side, Label: "Projected view " + sideLabel(side), Group: "Drawing", Enabled: viewSel,
			Run: func(string) { a.addSideView("proj", side) }})
		c.add(&Command{ID: "drawing.section." + side, Label: "Section " + sideLabel(side), Group: "Drawing", Enabled: viewSel,
			Run: func(string) { a.addSideView("section", side) }})
	}
	for _, k := range []string{"h", "v", "a", "r", "d", "angle"} {
		kind := k
		c.add(&Command{ID: "drawing.dim." + kind, Label: drawing.DimKinds[kind] + " dimension", Group: "Drawing", Enabled: inDrawing,
			Run: func(string) { a.setDrawTool("dim:" + kind) }})
	}
	c.add(&Command{ID: "drawing.note", Label: "Note", Group: "Drawing", Icon: icNote, Enabled: inDrawing,
		Run: func(arg string) {
			if arg == "" {
				a.setDrawTool("note")
				return
			}
			// "x,y:text" (sheet mm)
			at, text, _ := strings.Cut(arg, ":")
			if n := floats(at); len(n) == 2 {
				a.addNote(geom.V2(n[0], n[1]), text)
			}
		}})
	c.add(&Command{ID: "drawing.adddim", Label: "Dimension at model points", Group: "Drawing", Enabled: inDrawing,
		Run: func(arg string) { a.addDimAt(arg) }})
	c.add(&Command{ID: "drawing.select", Label: "Select on the sheet", Group: "Drawing", Enabled: inDrawing,
		Run: func(arg string) { a.drawSelectArg(arg) }})
	c.add(&Command{ID: "drawing.set", Label: "Set a drawing field", Group: "Drawing", Enabled: inDrawing,
		Run: func(arg string) { a.drawSet(arg) }})
	c.add(&Command{ID: "drawing.export", Label: "Export drawing (PDF, SVG, DXF)…", Group: "Drawing", Icon: ui.FaDownload, Enabled: inDrawing,
		Run: func(arg string) {
			if arg != "" {
				a.exportDrawing(arg)
				return
			}
			s := a.dwSheet()
			name := "drawing.pdf"
			if s != nil {
				name = safeName(s.Name) + ".pdf"
			}
			a.pickFile(true, "Export drawing", "Export", startIn(filepath.Dir(a.doc.Path), name), drawingFilter, a.exportDrawing)
		}})
}

func dirLabel(d string) string {
	return map[string]string{"front": "front", "back": "back", "top": "top", "bottom": "bottom", "left": "left", "right": "right",
		"iso": "isometric (front right)", "iso-fl": "isometric (front left)", "iso-br": "isometric (back right)", "iso-bl": "isometric (back left)"}[d]
}

func sideLabel(s string) string {
	return map[string]string{"right": "to the right", "left": "to the left", "above": "above", "below": "below"}[s]
}

func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, s)
	if s == "" {
		return "drawing"
	}
	return s
}

// dwSheet is the sheet on screen.
func (a *App) dwSheet() *drawing.Sheet {
	if a.dw == nil {
		return nil
	}
	return a.doc.D.Sheet(a.dw.sheet)
}

// openDrawing shows a sheet (arg: its ID), the first one, or a new A3
// with the standard views when the design has none.
func (a *App) openDrawing(arg string) {
	if a.sk != nil {
		return
	}
	a.sel, a.hover = nil, Ref{}
	if id, err := strconv.Atoi(arg); err == nil && a.doc.D.Sheet(id) != nil {
		a.dw = &drawMode{sheet: id, fit: true}
		return
	}
	if len(a.doc.D.Drawings) == 0 {
		a.newSheet(pickStr(arg, "A3"))
		return
	}
	a.dw = &drawMode{sheet: a.doc.D.Drawings[0].ID, fit: true}
}

// newSheet adds a sheet with the design's standard views and shows it.
func (a *App) newSheet(size string) {
	name := "Drawing"
	if a.doc.Path != "" {
		name = strings.TrimSuffix(filepath.Base(a.doc.Path), filepath.Ext(a.doc.Path))
	}
	s := drawing.NewSheet(name, size)
	if len(a.doc.D.Drawings) > 0 {
		s.Name = fmt.Sprintf("%s %d", name, len(a.doc.D.Drawings)+1)
	}
	s.Title.Date = time.Now().Format("2006-01-02")
	bodies := a.doc.D.DrawingBodies()
	if len(bodies) == 0 {
		a.notify("The design has no bodies to draw yet: the sheet is empty")
	}
	s.Standard(bodies)
	a.doc.D.AddSheet(s)
	a.doc.Commit()
	a.sel, a.hover = nil, Ref{}
	a.dw = &drawMode{sheet: s.ID, fit: true}
}

func (a *App) deleteSheet() {
	s := a.dwSheet()
	if s == nil {
		return
	}
	a.doc.D.RemoveSheet(s.ID)
	a.doc.Commit()
	if len(a.doc.D.Drawings) == 0 {
		a.dw = nil
		return
	}
	a.dw = &drawMode{sheet: a.doc.D.Drawings[0].ID, fit: true}
}

func (a *App) setDrawTool(t string) {
	if a.dw == nil {
		return
	}
	if a.dw.tool == t {
		t = ""
	}
	a.dw.tool, a.dw.picks = t, nil
}

// freeSpot is a sheet point for a new view: the middle of the drawing area
// to the right of what is already there, or its middle.
func freeSpot(s *drawing.Sheet, res *drawing.Result) geom.Vec2 {
	x0, y0, x1, y1 := s.Area()
	return geom.V2((x0+x1)/2, (y0+y1)/2)
}

func (a *App) addBaseView(dir, at string) {
	s := a.dwSheet()
	if s == nil {
		return
	}
	p := freeSpot(s, a.dw.res)
	if n := floats(at); len(n) == 2 {
		p = geom.V2(n[0], n[1])
	}
	v := s.AddView(&drawing.View{Kind: "base", Dir: dir, X: p.X, Y: p.Y, Hidden: !strings.HasPrefix(dir, "iso"), Tangent: true})
	a.doc.Commit()
	a.dw.selKind, a.dw.selID = "view", v.ID
}

// addSideView puts a projected view or a section beside the selected view.
func (a *App) addSideView(kind, side string) {
	s := a.dwSheet()
	if s == nil || a.dw.selKind != "view" {
		return
	}
	p := s.View(a.dw.selID)
	if p == nil {
		return
	}
	v := &drawing.View{Kind: kind, Parent: p.ID, Side: side, X: p.X, Y: p.Y, Hidden: kind == "proj" && p.Hidden, Tangent: true}
	s.PlaceBeside(v, a.doc.D.DrawingBodies(), 22)
	if kind == "section" {
		v.Label = s.NextLabel()
		v.Hidden = false
		if fr, err := s.FrameOf(v); err == nil {
			v.At = drawing.SectionAt(a.doc.D.DrawingBodies(), fr)
		}
	}
	s.AddView(v)
	a.doc.Commit()
	a.dw.selKind, a.dw.selID = "view", v.ID
}

func (a *App) addNote(at geom.Vec2, text string) {
	s := a.dwSheet()
	if s == nil {
		return
	}
	if text == "" {
		text = "NOTE"
	}
	n := &drawing.Note{X: at.X, Y: at.Y, Text: strings.ReplaceAll(text, `\n`, "\n")}
	n.ID = s.Next + 1
	s.Next++
	s.Notes = append(s.Notes, n)
	a.doc.Commit()
	a.dw.selKind, a.dw.selID = "note", n.ID
}

// addDimAt adds a dimension from model points (for scripts):
// "<view index>:<kind>:<x,y,z>[;<x,y,z>]:<dx,dy>", each point picked in
// the view as a click there would.
func (a *App) addDimAt(arg string) {
	s := a.dwSheet()
	parts := strings.Split(arg, ":")
	if s == nil || len(parts) != 4 {
		a.notify("drawing.adddim: want view:kind:points:place")
		return
	}
	vi, _ := strconv.Atoi(parts[0])
	if vi < 0 || vi >= len(s.Views) {
		return
	}
	a.dwRebuild()
	b := a.dw.res.Views[s.Views[vi].ID]
	if b == nil || b.Err != "" {
		return
	}
	d := &drawing.Dim{View: b.View.ID, Kind: parts[1]}
	for _, ps := range strings.Split(parts[2], ";") {
		n := floats(ps)
		if len(n) != 3 {
			continue
		}
		an, ok := b.Pick(b.SheetOf(geom.V3(n[0], n[1], n[2])), 1)
		if !ok {
			a.notify("drawing.adddim: nothing at %v", n)
			return
		}
		d.Anchors = append(d.Anchors, an)
	}
	if n := floats(parts[3]); len(n) == 2 {
		d.Place = geom.V2(n[0], n[1])
	}
	if _, err := d.Value(b); err != nil {
		a.notify("Dimension: %v", err)
		return
	}
	s.AddDim(d)
	a.doc.Commit()
	a.dw.selKind, a.dw.selID = "dim", d.ID
}

// drawSelectArg selects by index: "view:2", "dim:0", "note:1", "".
func (a *App) drawSelectArg(arg string) {
	s := a.dwSheet()
	if s == nil {
		return
	}
	kind, is, _ := strings.Cut(arg, ":")
	i, _ := strconv.Atoi(is)
	a.dw.selKind, a.dw.selID = "", 0
	switch kind {
	case "view":
		if i >= 0 && i < len(s.Views) {
			a.dw.selKind, a.dw.selID = kind, s.Views[i].ID
		}
	case "dim":
		if i >= 0 && i < len(s.Dims) {
			a.dw.selKind, a.dw.selID = kind, s.Dims[i].ID
		}
	case "note":
		if i >= 0 && i < len(s.Notes) {
			a.dw.selKind, a.dw.selID = kind, s.Notes[i].ID
		}
	}
}

// drawSet sets a sheet field (scripts): "size=A3", "scale=1:2",
// "angle=first", "portrait=1", "title=…", "material=…", "author=…",
// "number=…", "company=…", "revision=…", "places=1", for the selected
// view "view.scale=2:1", "view.hidden=0", "view.at=12", and for the
// selected dimension "dim.text=4× Ø<>".
func (a *App) drawSet(arg string) {
	s := a.dwSheet()
	k, v, ok := strings.Cut(arg, "=")
	if s == nil || !ok {
		return
	}
	t := &s.Title
	switch k {
	case "size":
		s.Size = v
	case "scale":
		s.Scale = v
	case "angle":
		s.FirstAngle = v == "first"
	case "portrait":
		s.Portrait = v == "1"
	case "places":
		s.Places, _ = strconv.Atoi(v)
	case "name":
		s.Name = v
	case "title":
		t.Title = v
	case "number":
		t.Number = v
	case "material":
		t.Material = v
	case "finish":
		t.Finish = v
	case "author":
		t.Author = v
	case "checked":
		t.Checked = v
	case "date":
		t.Date = v
	case "revision":
		t.Revision = v
	case "company":
		t.Company = v
	case "dim.text":
		for _, d := range s.Dims {
			if a.dw.selKind == "dim" && d.ID == a.dw.selID {
				d.Text = v
			}
		}
	default:
		vw := s.View(a.dw.selID)
		if a.dw.selKind != "view" || vw == nil {
			return
		}
		switch k {
		case "view.scale":
			vw.Scale = v
		case "view.hidden":
			vw.Hidden = v == "1"
		case "view.tangent":
			vw.Tangent = v == "1"
		case "view.centre":
			vw.NoCentre = v == "0"
		case "view.at":
			vw.At, _ = strconv.ParseFloat(v, 64)
		case "view.x":
			vw.X, _ = strconv.ParseFloat(v, 64)
		case "view.y":
			vw.Y, _ = strconv.ParseFloat(v, 64)
		}
	}
	a.doc.Commit()
}

func (a *App) exportDrawing(path string) {
	s := a.dwSheet()
	if s == nil {
		return
	}
	a.dwRebuild()
	pg := a.dw.res.Page
	f, err := os.Create(path)
	if err != nil {
		a.notify("Export: %v", err)
		return
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".svg":
		err = pg.WriteSVG(f)
	case ".dxf":
		err = pg.WriteDXF(f)
	default:
		err = pg.WritePDF(f)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		a.notify("Export: %v", err)
		return
	}
	a.notify("Wrote %s", filepath.Base(path))
}

// --- building ---

// dwRebuild builds the sheet again when it or the bodies changed.
func (a *App) dwRebuild() {
	s := a.dwSheet()
	if s == nil {
		return
	}
	bodies := a.doc.D.DrawingBodies()
	h := fnv.New64a()
	js, _ := json.Marshal(s)
	h.Write(js)
	for _, b := range bodies {
		fmt.Fprintf(h, "|%d:%d:%p", b.Key, b.Gen, b.Body)
	}
	if sig := h.Sum64(); sig != a.dw.sig || a.dw.res == nil {
		a.dw.sig = sig
		a.dw.res = s.Build(bodies, &a.dw.cache)
		a.dw.cache.Keep(a.dw.res.Keys)
	}
}

// --- the sheet on screen ---

func (dm *drawMode) toScreen(vp *viewport, p geom.Vec2) imgui.Vec2 {
	return imgui.Vec2{X: vp.pos.X + float32(dm.off.X+p.X*dm.zoom), Y: vp.pos.Y + float32(dm.off.Y-p.Y*dm.zoom)}
}

func (dm *drawMode) toSheet(vp *viewport, m imgui.Vec2) geom.Vec2 {
	return geom.V2((float64(m.X)-dm.off.X)/dm.zoom, (dm.off.Y-float64(m.Y))/dm.zoom)
}

func (dm *drawMode) fitTo(vp *viewport, w, h float64) {
	vw, vh := float64(vp.size.X), float64(vp.size.Y)
	dm.zoom = math.Min((vw-40)/w, (vh-40)/h)
	if dm.zoom <= 0 {
		dm.zoom = 1
	}
	dm.off = geom.V2((vw-w*dm.zoom)/2, (vh+h*dm.zoom)/2)
}

func rgba(r, g, b, al float64) uint32 {
	return imgui.ColorConvertFloat4ToU32(imgui.Vec4{X: float32(r), Y: float32(g), Z: float32(b), W: float32(al)})
}

// drawingViewport is the viewport while a sheet is shown.
func (a *App) drawingViewport() {
	vp := &a.vp
	dm := a.dw
	s := a.dwSheet()
	if s == nil {
		a.dw = nil
		return
	}
	imgui.InvisibleButtonV("##sheet", vp.size, imgui.ButtonFlagsMouseButtonLeft|imgui.ButtonFlagsMouseButtonRight|imgui.ButtonFlagsMouseButtonMiddle)
	vp.hovered = imgui.IsItemHovered()
	if vp.virtual != nil {
		vp.mouse = *vp.virtual
		vp.hovered = true
	} else {
		m := imgui.MousePos()
		vp.mouse = imgui.Vec2{X: m.X - vp.pos.X, Y: m.Y - vp.pos.Y}
	}
	a.dwRebuild()
	W, H := s.Paper()
	if dm.fit || dm.zoom == 0 {
		dm.fitTo(vp, W, H)
		dm.fit = false
	}
	dm.mouse = dm.toSheet(vp, vp.mouse)
	a.drawingInput(s)

	dl := imgui.WindowDrawList()
	end := imgui.Vec2{X: vp.pos.X + vp.size.X, Y: vp.pos.Y + vp.size.Y}
	dl.PushClipRect(vp.pos, end)
	dl.AddRectFilled(vp.pos, end, rgba(0.23, 0.25, 0.28, 1))
	p0, p1 := dm.toScreen(vp, geom.V2(0, H)), dm.toScreen(vp, geom.V2(W, 0))
	dl.AddRectFilled(imgui.Vec2{X: p0.X + 4, Y: p0.Y + 4}, imgui.Vec2{X: p1.X + 4, Y: p1.Y + 4}, rgba(0, 0, 0, 0.35))
	dl.AddRectFilled(p0, p1, rgba(1, 1, 1, 1))

	ink := rgba(0.08, 0.08, 0.1, 1)
	sel := rgba(0.1, 0.45, 0.95, 1)
	hot := rgba(0.95, 0.55, 0.1, 1)
	colour := func(tag int) uint32 {
		if tag != 0 && tag == dm.selID {
			return sel
		}
		return ink
	}
	a.paintPage(dl, dm.res.Page, colour)

	// the selected view's outline
	if dm.selKind == "view" {
		if b := dm.res.Views[dm.selID]; b != nil && b.Err == "" {
			lo := dm.toScreen(vp, b.Lo.Sub(geom.V2(4, 4)))
			hi := dm.toScreen(vp, b.Hi.Add(geom.V2(4, 4)))
			dl.AddRectV(imgui.Vec2{X: lo.X, Y: hi.Y}, imgui.Vec2{X: hi.X, Y: lo.Y}, rgba(0.1, 0.45, 0.95, 0.8), 3, 1.5, 0)
		}
	}
	// views that couldn't be drawn
	for _, v := range s.Views {
		if b := dm.res.Views[v.ID]; b != nil && b.Err != "" {
			p := dm.toScreen(vp, geom.V2(v.X, v.Y))
			dl.AddTextVec2(p, rgba(0.8, 0.2, 0.1, 1), "view: "+b.Err)
		}
	}
	// a dimension tool: what it would pick, and the dimension as it would be
	if strings.HasPrefix(dm.tool, "dim:") {
		if dm.hoverOK {
			if b := dm.res.Views[dm.hoverV]; b != nil {
				p := dm.toScreen(vp, b.SheetOf(dm.hover.P))
				r := float32(4)
				if dm.hover.At != "" {
					r = 6
				}
				dl.AddCircleFilled(p, r, hot)
			}
		}
		for _, an := range dm.picks {
			if b := dm.res.Views[dm.pview]; b != nil {
				dl.AddCircleFilled(dm.toScreen(vp, b.SheetOf(an.P)), 5, sel)
			}
		}
		if d := a.dimPreview(); d != nil {
			pg := &drawing.Page{}
			if err := d.Draw(pg, dm.res.Views[d.View], s.Places); err == nil {
				a.paintPage(dl, pg, func(int) uint32 { return hot })
			}
		}
	}
	dl.PopClipRect()
	if dm.tool != "" && vp.hovered {
		imgui.SetTooltip(a.drawToolHint())
	}
}

// paintPage draws a page's lines, arrowheads and text.
func (a *App) paintPage(dl *imgui.DrawList, pg *drawing.Page, colour func(tag int) uint32) {
	vp := &a.vp
	dm := a.dw
	for _, p := range pg.Paths {
		if len(p.Pts) < 2 {
			continue
		}
		th := float32(math.Max(1, p.Pen.Width()*dm.zoom))
		col := colour(p.Tag)
		for _, run := range drawing.Dashed(p.Pts, p.Closed, p.Pen.Dash(), 1) {
			if len(run) < 2 {
				continue
			}
			pts := make([]imgui.Vec2, len(run))
			for i, q := range run {
				pts[i] = dm.toScreen(vp, q)
			}
			dl.AddPolylineV(&pts[0], int32(len(pts)), col, th, 0)
		}
	}
	for _, f := range pg.Fills {
		if len(f.Pts) < 3 {
			continue
		}
		pts := make([]imgui.Vec2, len(f.Pts))
		for i, q := range f.Pts {
			pts[i] = dm.toScreen(vp, q)
		}
		dl.AddConvexPolyFilled(&pts[0], int32(len(pts)), colour(f.Tag))
	}
	for _, t := range pg.Texts {
		a.sheetText(dl, t, colour(t.Tag))
	}
}

// sheetText draws text at any angle and size: each glyph's quad from the
// font atlas placed along the text's baseline (ImGui draws text only
// level), stretched to the width the page laid it out at.
func (a *App) sheetText(dl *imgui.DrawList, t drawing.Text, col uint32) {
	vp := &a.vp
	dm := a.dw
	if t.Size*dm.zoom < 2 {
		return
	}
	o, d := t.Origin()
	up := d.Perp()
	font := imgui.CurrentFont()
	const F = 48
	baked := font.FontBaked(F)
	if baked == nil {
		return
	}
	asc := float64(baked.Ascent())
	// the font's own width of it, to stretch it to the page's
	adv := 0.0
	for _, ch := range t.S {
		if g := baked.FindGlyph(imgui.Wchar(ch)); g != nil {
			adv += float64(g.AdvanceX())
		}
	}
	if adv == 0 {
		return
	}
	k := t.Size / F // mm per baked pixel: the font size is its em
	sx := drawing.TextWidth(t.S, t.Size, t.Bold) / (adv * k)
	tex := font.OwnerAtlas().TexRef()
	x := 0.0
	for _, ch := range t.S {
		g := baked.FindGlyph(imgui.Wchar(ch))
		if g == nil {
			continue
		}
		if g.Visible() != 0 {
			at := func(gx, gy float32) imgui.Vec2 {
				p := o.Add(d.Scale((x + float64(gx)) * k * sx)).Add(up.Scale((asc - float64(gy)) * k))
				return dm.toScreen(vp, p)
			}
			dl.AddImageQuadV(tex, at(g.X0(), g.Y0()), at(g.X1(), g.Y0()), at(g.X1(), g.Y1()), at(g.X0(), g.Y1()),
				imgui.Vec2{X: g.U0(), Y: g.V0()}, imgui.Vec2{X: g.U1(), Y: g.V0()}, imgui.Vec2{X: g.U1(), Y: g.V1()}, imgui.Vec2{X: g.U0(), Y: g.V1()}, col)
		}
		x += float64(g.AdvanceX())
	}
}

// dimPreview is the dimension a click would place now, if any.
func (a *App) dimPreview() *drawing.Dim {
	dm := a.dw
	kind := strings.TrimPrefix(dm.tool, "dim:")
	if len(dm.picks) < minPicks(kind) {
		return nil
	}
	s := a.dwSheet()
	v := s.View(dm.pview)
	b := dm.res.Views[dm.pview]
	if v == nil || b == nil || b.Err != "" {
		return nil
	}
	d := &drawing.Dim{View: v.ID, Kind: kind, Anchors: dm.picks, Place: dm.mouse.Sub(geom.V2(v.X, v.Y))}
	if _, err := d.Value(b); err != nil {
		return nil
	}
	return d
}

func minPicks(kind string) int {
	if kind == "angle" {
		return 2
	}
	return 1
}

func maxPicks(kind string) int {
	if kind == "r" || kind == "d" {
		return 1
	}
	return 2
}

func (a *App) drawToolHint() string {
	dm := a.dw
	switch {
	case dm.tool == "note":
		return "Click where the note goes"
	case strings.HasPrefix(dm.tool, "dim:"):
		kind := strings.TrimPrefix(dm.tool, "dim:")
		n := len(dm.picks)
		switch {
		case n >= maxPicks(kind):
			return "Click where the dimension goes"
		case n >= minPicks(kind):
			return "Pick another edge or point, or click where the dimension goes"
		case kind == "r" || kind == "d":
			return "Pick a circle seen end on"
		case kind == "angle":
			return "Pick two straight edges"
		}
		return "Pick an edge, a corner or a hole's centre"
	}
	return ""
}

// drawingInput: wheel zooms about the cursor, middle or right drag pans,
// left picks, drags, or does what the tool does.
func (a *App) drawingInput(s *drawing.Sheet) {
	vp := &a.vp
	dm := a.dw
	io := imgui.CurrentIO()
	if vp.hovered {
		if w := io.MouseWheel(); w != 0 {
			at := dm.mouse
			dm.zoom *= math.Pow(1.18, float64(w))
			dm.zoom = math.Max(0.2, math.Min(dm.zoom, 80))
			// keep the point under the cursor
			dm.off = geom.V2(float64(vp.mouse.X)-at.X*dm.zoom, float64(vp.mouse.Y)+at.Y*dm.zoom)
		}
		if imgui.IsMouseClickedBool(imgui.MouseButtonMiddle) || imgui.IsMouseClickedBool(imgui.MouseButtonRight) {
			if imgui.IsMouseClickedBool(imgui.MouseButtonRight) && dm.tool != "" {
				dm.tool, dm.picks = "", nil
			} else {
				dm.drag = "pan"
			}
		}
	}
	if dm.drag == "pan" {
		d := io.MouseDelta()
		dm.off = dm.off.Add(geom.V2(float64(d.X), float64(d.Y)))
		if !imgui.IsMouseDown(imgui.MouseButtonMiddle) && !imgui.IsMouseDown(imgui.MouseButtonRight) {
			dm.drag = ""
		}
		return
	}
	// what a dimension tool would pick under the cursor
	dm.hoverOK = false
	tol := 7 / dm.zoom
	if strings.HasPrefix(dm.tool, "dim:") && vp.hovered {
		for _, v := range s.Views {
			if len(dm.picks) > 0 && v.ID != dm.pview {
				continue
			}
			b := dm.res.Views[v.ID]
			if b == nil || b.Err != "" {
				continue
			}
			if an, ok := b.Pick(dm.mouse, tol); ok {
				dm.hover, dm.hoverV, dm.hoverOK = an, v.ID, true
				break
			}
		}
	}
	if vp.hovered && imgui.IsMouseClickedBool(imgui.MouseButtonLeft) {
		a.drawingClick(s)
	}
	if dm.drag != "" && dm.drag != "pan" {
		delta := dm.mouse.Sub(dm.dragFrom)
		if imgui.IsMouseDown(imgui.MouseButtonLeft) {
			if delta.Len()*dm.zoom > 3 {
				dm.dragMoved = true
			}
			if dm.dragMoved {
				a.dragTo(s, delta)
			}
		} else {
			if dm.dragMoved {
				a.doc.Commit()
			}
			dm.drag = ""
		}
	}
	if vp.hovered && dm.tool == "" && imgui.IsMouseDoubleClicked(imgui.MouseButtonLeft) && dm.selKind == "view" {
		// a double click on a view frames it
		if b := dm.res.Views[dm.selID]; b != nil && b.Err == "" {
			w, h := b.Hi.X-b.Lo.X+30, b.Hi.Y-b.Lo.Y+30
			dm.fitTo(vp, w, h)
			c := b.Lo.Add(b.Hi).Scale(0.5)
			dm.off = geom.V2(float64(vp.size.X)/2-c.X*dm.zoom, float64(vp.size.Y)/2+c.Y*dm.zoom)
		}
	}
}

func (a *App) drawingClick(s *drawing.Sheet) {
	dm := a.dw
	switch {
	case dm.tool == "note":
		a.addNote(dm.mouse, "NOTE")
		dm.tool = ""
		return
	case strings.HasPrefix(dm.tool, "dim:"):
		kind := strings.TrimPrefix(dm.tool, "dim:")
		if dm.hoverOK && len(dm.picks) < maxPicks(kind) {
			if len(dm.picks) == 0 {
				dm.pview = dm.hoverV
			}
			dm.picks = append(dm.picks, dm.hover)
			return
		}
		if d := a.dimPreview(); d != nil {
			s.AddDim(d)
			a.doc.Commit()
			dm.picks = nil
			dm.selKind, dm.selID = "dim", d.ID
			return
		}
		if len(dm.picks) >= minPicks(kind) {
			a.notify("That can't be measured: pick again")
			dm.picks = nil
		}
		return
	}
	// select: a dimension or note under the cursor first, then a view
	kind, id := a.hitTest(s)
	dm.selKind, dm.selID = kind, id
	dm.drag, dm.dragID, dm.dragFrom, dm.dragMoved = kind, id, dm.mouse, false
	dm.dragStart = nil
	switch kind {
	case "view":
		for _, v := range a.viewFamily(s, id) {
			dm.dragStart = append(dm.dragStart, geom.V2(v.X, v.Y))
		}
	case "dim":
		for _, d := range s.Dims {
			if d.ID == id {
				dm.dragStart = []geom.Vec2{d.Place}
			}
		}
	case "note":
		for _, n := range s.Notes {
			if n.ID == id {
				dm.dragStart = []geom.Vec2{geom.V2(n.X, n.Y)}
			}
		}
	default:
		dm.drag = ""
	}
}

// viewFamily is a view and every view seen from it, at any remove.
func (a *App) viewFamily(s *drawing.Sheet, id int) []*drawing.View {
	in := map[int]bool{id: true}
	for changed := true; changed; {
		changed = false
		for _, v := range s.Views {
			if !in[v.ID] && in[v.Parent] && v.Parent != 0 {
				in[v.ID], changed = true, true
			}
		}
	}
	var out []*drawing.View
	for _, v := range s.Views {
		if in[v.ID] {
			out = append(out, v)
		}
	}
	return out
}

func (a *App) dragTo(s *drawing.Sheet, delta geom.Vec2) {
	dm := a.dw
	switch dm.drag {
	case "view":
		fam := a.viewFamily(s, dm.dragID)
		for i, v := range fam {
			if i < len(dm.dragStart) {
				v.X, v.Y = dm.dragStart[i].X+delta.X, dm.dragStart[i].Y+delta.Y
			}
		}
		s.Align()
	case "dim":
		for _, d := range s.Dims {
			if d.ID == dm.dragID && len(dm.dragStart) > 0 {
				d.Place = dm.dragStart[0].Add(delta)
			}
		}
	case "note":
		for _, n := range s.Notes {
			if n.ID == dm.dragID && len(dm.dragStart) > 0 {
				n.X, n.Y = dm.dragStart[0].X+delta.X, dm.dragStart[0].Y+delta.Y
			}
		}
	}
}

// hitTest is what is under the cursor: a dimension or note (by its lines
// and text), else the smallest view whose box holds it.
func (a *App) hitTest(s *drawing.Sheet) (string, int) {
	dm := a.dw
	p := dm.mouse
	tol := 5 / dm.zoom
	isDim := map[int]bool{}
	for _, d := range s.Dims {
		isDim[d.ID] = true
	}
	isNote := map[int]bool{}
	for _, n := range s.Notes {
		isNote[n.ID] = true
	}
	best, bd := 0, math.Inf(1)
	consider := func(tag int, d float64) {
		if (isDim[tag] || isNote[tag]) && d < bd && d <= tol {
			best, bd = tag, d
		}
	}
	pg := dm.res.Page
	for _, pa := range pg.Paths {
		for i := 0; i+1 < len(pa.Pts); i++ {
			consider(pa.Tag, segDist(p, pa.Pts[i], pa.Pts[i+1]))
		}
	}
	for _, f := range pg.Fills {
		if len(f.Pts) > 0 {
			consider(f.Tag, p.Dist(f.Pts[0]))
		}
	}
	for _, t := range pg.Texts {
		o, d := t.Origin()
		w := drawing.TextWidth(t.S, t.Size, t.Bold)
		q := p.Sub(o)
		along, up := q.Dot(d), q.Dot(d.Perp())
		if along >= -1 && along <= w+1 && up >= -1 && up <= t.Size+1 {
			consider(t.Tag, 0)
		}
	}
	if best != 0 {
		if isDim[best] {
			return "dim", best
		}
		return "note", best
	}
	area := math.Inf(1)
	id := 0
	for _, v := range s.Views {
		b := dm.res.Views[v.ID]
		if b == nil || b.Err != "" {
			continue
		}
		if p.X >= b.Lo.X-4 && p.X <= b.Hi.X+4 && p.Y >= b.Lo.Y-4 && p.Y <= b.Hi.Y+4 {
			if ar := (b.Hi.X - b.Lo.X) * (b.Hi.Y - b.Lo.Y); ar < area {
				area, id = ar, v.ID
			}
		}
	}
	if id != 0 {
		return "view", id
	}
	return "", 0
}

func segDist(p, a, b geom.Vec2) float64 {
	ab := b.Sub(a)
	t := 0.0
	if l2 := ab.Dot(ab); l2 > 0 {
		t = math.Max(0, math.Min(1, p.Sub(a).Dot(ab)/l2))
	}
	return p.Dist(a.Add(ab.Scale(t)))
}

// drawingDelete takes the selection off the sheet.
func (a *App) drawingDelete() {
	s := a.dwSheet()
	dm := a.dw
	if s == nil || dm.selID == 0 {
		return
	}
	switch dm.selKind {
	case "view":
		s.RemoveView(dm.selID)
	case "dim":
		s.RemoveDim(dm.selID)
	case "note":
		for i, n := range s.Notes {
			if n.ID == dm.selID {
				s.Notes = append(s.Notes[:i], s.Notes[i+1:]...)
				break
			}
		}
	}
	dm.selKind, dm.selID = "", 0
	a.doc.Commit()
}

// drawingEscape drops the tool, then the selection.
func (a *App) drawingEscape() {
	dm := a.dw
	if dm.tool != "" {
		dm.tool, dm.picks = "", nil
		return
	}
	dm.selKind, dm.selID = "", 0
}

// --- ribbon and panels ---

func (a *App) drawingRibbon(r *ribbon) {
	dm := a.dw
	r.group()
	r.cmd("drawing.close", ui.FaCheck, "Finish", false)
	r.cmd("edit.undo", 0xf0e2, "Undo", false)
	r.cmd("edit.redo", 0xf01e, "Redo", false)
	r.end("Drawing")

	r.group()
	var sizes [][2]string
	for _, z := range drawing.Sizes {
		sizes = append(sizes, [2]string{"drawing.sheet." + strings.ReplaceAll(strings.ToLower(z.Name), " ", ""), z.Name})
	}
	r.menu("drawing.sheet", ui.FaFile, "New sheet", "A new sheet with the design's standard views", sizes)
	r.cmd("view.fit", ui.FaExpand, "Fit", false)
	r.end("Sheet")

	r.group()
	var bases [][2]string
	for _, d := range drawing.Dirs {
		bases = append(bases, [2]string{"drawing.base." + d, strings.ToUpper(dirLabel(d)[:1]) + dirLabel(d)[1:]})
	}
	r.menu("drawing.base", ui.FaCube, "Base view", "A view of the design from a standard direction", bases)
	var projs, secs [][2]string
	for _, s := range []string{"right", "left", "above", "below"} {
		projs = append(projs, [2]string{"drawing.proj." + s, "Seen from " + map[string]string{"right": "the right", "left": "the left", "above": "above", "below": "below"}[s]})
		secs = append(secs, [2]string{"drawing.section." + s, "Placed " + sideLabel(s)})
	}
	r.menu("drawing.proj", icProject, "Projected", "A view seen from a side of the selected view, lined up with it", projs)
	r.menu("drawing.section", icSection, "Section", "The selected view cut through its middle, hatched, placed beside it", secs)
	r.end("Views")

	r.group()
	for _, t := range []struct {
		kind string
		icon rune
		name string
	}{{"h", ui.FaArrowsH, "Horizontal"}, {"v", ui.FaArrowsV, "Vertical"}, {"a", ui.FaRuler, "Aligned"},
		{"r", ui.FaCircle, "Radius"}, {"d", ui.FaDotCircle, "Diameter"}, {"angle", icAngle, "Angle"}} {
		r.cmd("drawing.dim."+t.kind, t.icon, t.name, dm.tool == "dim:"+t.kind)
	}
	r.cmd("drawing.note", icNote, "Note", dm.tool == "note")
	r.end("Annotate")

	r.group()
	r.cmd("drawing.export", ui.FaDownload, "Export", false)
	r.end("Export")
}

// drawingBrowser lists the sheets and the views of the one shown.
func (a *App) drawingBrowser() {
	dm := a.dw
	imgui.SeparatorText(ui.Icon(icDrawing, "Sheets"))
	for _, s := range a.doc.D.Drawings {
		if imgui.SelectableBoolV(fmt.Sprintf("%s  (%s)##sheet%d", s.Name, s.Size, s.ID), s.ID == dm.sheet, 0, imgui.Vec2{}) && s.ID != dm.sheet {
			a.dw = &drawMode{sheet: s.ID, fit: true}
			return
		}
		a.markItem("item:" + s.Name)
	}
	s := a.dwSheet()
	if s == nil {
		return
	}
	imgui.SeparatorText("Views")
	for _, v := range s.Views {
		if imgui.SelectableBoolV(viewName(s, v)+fmt.Sprintf("##v%d", v.ID), dm.selKind == "view" && dm.selID == v.ID, 0, imgui.Vec2{}) {
			dm.selKind, dm.selID = "view", v.ID
		}
	}
	if len(s.Dims)+len(s.Notes) > 0 {
		imgui.SeparatorText("Annotations")
	}
	for i, d := range s.Dims {
		label := drawing.DimKinds[d.Kind] + " " + strconv.Itoa(i+1)
		if b := dm.res.Views[d.View]; b != nil && b.Err == "" {
			if v, err := d.Value(b); err == nil {
				label += "  " + drawing.FormatValue(v, s.Places)
			}
		}
		if e := dm.res.Dims[d.ID]; e != "" {
			label += "  (" + e + ")"
		}
		if imgui.SelectableBoolV(label+fmt.Sprintf("##d%d", d.ID), dm.selKind == "dim" && dm.selID == d.ID, 0, imgui.Vec2{}) {
			dm.selKind, dm.selID = "dim", d.ID
		}
	}
	for _, n := range s.Notes {
		first, _, _ := strings.Cut(n.Text, "\n")
		if imgui.SelectableBoolV("Note: "+first+fmt.Sprintf("##n%d", n.ID), dm.selKind == "note" && dm.selID == n.ID, 0, imgui.Vec2{}) {
			dm.selKind, dm.selID = "note", n.ID
		}
	}
}

func viewName(s *drawing.Sheet, v *drawing.View) string {
	switch v.Kind {
	case "section":
		return "Section " + v.Label + "-" + v.Label
	case "proj":
		fr, err := s.FrameOf(v)
		if err == nil {
			for _, d := range drawing.Dirs {
				if b, _ := drawing.BaseFrame(d); b.D.Dist(fr.D) < 1e-9 && b.U.Dist(fr.U) < 1e-9 {
					return strings.ToUpper(d[:1]) + d[1:] + " (projected)"
				}
			}
		}
		return "Projected " + v.Side
	}
	l := dirLabel(v.Dir)
	return strings.ToUpper(l[:1]) + l[1:]
}

// combo is a combo box of strings; true when one was picked.
func combo(label string, cur *string, items []string, names func(string) string) bool {
	changed := false
	if names == nil {
		names = func(s string) string { return s }
	}
	if imgui.BeginCombo(label, names(*cur)) {
		for _, it := range items {
			if imgui.SelectableBoolV(names(it), it == *cur, 0, imgui.Vec2{}) && it != *cur {
				*cur, changed = it, true
			}
		}
		imgui.EndCombo()
	}
	return changed
}

// drawingProperties edits the sheet and what is selected on it.
func (a *App) drawingProperties() {
	dm := a.dw
	s := a.dwSheet()
	if s == nil {
		return
	}
	commit := false
	edited := func(b bool) {
		if b {
			commit = true
		}
	}
	text := func(label string, v *string) {
		before := *v
		imgui.InputTextWithHint(label, "", v, 0, nil)
		if imgui.IsItemDeactivatedAfterEdit() && *v != before {
			commit = true
		}
	}
	switch dm.selKind {
	case "view":
		if v := s.View(dm.selID); v != nil {
			imgui.SeparatorText(viewName(s, v))
			if v.Kind == "base" {
				edited(combo("Direction##vdir", &v.Dir, drawing.Dirs, dirLabel))
			}
			sc := v.Scale
			if sc == "" {
				sc = "sheet"
			}
			if combo("Scale##vscale", &sc, append([]string{"sheet"}, drawing.Scales...), func(x string) string {
				if x == "sheet" {
					return "As the sheet (" + s.Scale + ")"
				}
				return x
			}) {
				v.Scale = map[bool]string{true: "", false: sc}[sc == "sheet"]
				commit = true
			}
			edited(imgui.Checkbox("Hidden lines##vh", &v.Hidden))
			edited(imgui.Checkbox("Tangent edges##vt", &v.Tangent))
			cl := !v.NoCentre
			if imgui.Checkbox("Centre lines##vc", &cl) {
				v.NoCentre = !cl
				commit = true
			}
			if v.Kind == "section" {
				bodies := a.doc.D.DrawingBodies()
				if fr, err := s.FrameOf(v); err == nil && len(bodies) > 0 {
					var box geom.Box
					for _, b := range bodies {
						box.Union(b.Body.Bounds())
					}
					lo, hi := math.Inf(1), math.Inf(-1)
					for i := 0; i < 8; i++ {
						p := geom.V3(pickF(i&1, box.Min.X, box.Max.X), pickF(i&2, box.Min.Y, box.Max.Y), pickF(i&4, box.Min.Z, box.Max.Z))
						lo, hi = math.Min(lo, p.Dot(fr.D)), math.Max(hi, p.Dot(fr.D))
					}
					at := float32(v.At)
					if imgui.SliderFloatV("Cut at##vat", &at, float32(lo), float32(hi), "%.2f mm", 0) {
						v.At = float64(at)
					}
					if imgui.IsItemDeactivatedAfterEdit() {
						commit = true
					}
				}
				text("Label##vlab", &v.Label)
			}
			if b := dm.res.Views[v.ID]; b != nil {
				if b.Err != "" {
					imgui.TextWrapped("Can't draw it: " + b.Err)
				} else if b.Proj.Warn != "" {
					imgui.TextWrapped(b.Proj.Warn)
				}
			}
			if imgui.Button(ui.Icon(ui.FaTrash, "Delete view")) {
				defer a.drawingDelete()
			}
		}
	case "dim":
		for _, d := range s.Dims {
			if d.ID != dm.selID {
				continue
			}
			imgui.SeparatorText(drawing.DimKinds[d.Kind] + " dimension")
			if b := dm.res.Views[d.View]; b != nil && b.Err == "" {
				if v, err := d.Value(b); err == nil {
					unit := " mm"
					if d.Kind == "angle" {
						unit = "°"
					}
					imgui.Text("Measures " + drawing.FormatValue(v, 4) + unit)
				}
			}
			if e := dm.res.Dims[d.ID]; e != "" {
				imgui.TextWrapped("Can't draw it: " + e)
			}
			text("Text##dtext", &d.Text)
			imgui.TextDisabled("Empty: the value. <> stands for it (\"2× <>\", \"Ø<> H7\").")
			if imgui.Button(ui.Icon(ui.FaTrash, "Delete dimension")) {
				defer a.drawingDelete()
			}
		}
	case "note":
		for _, n := range s.Notes {
			if n.ID != dm.selID {
				continue
			}
			imgui.SeparatorText("Note")
			before := n.Text
			imgui.InputTextMultiline("##ntext", &n.Text, imgui.Vec2{X: -1, Y: 80}, 0, nil)
			if imgui.IsItemDeactivatedAfterEdit() && n.Text != before {
				commit = true
			}
			size := float32(n.Size)
			if size == 0 {
				size = 3.5
			}
			if imgui.SliderFloatV("Size##nsize", &size, 2.5, 10, "%.1f mm", 0) {
				n.Size = float64(size)
			}
			if imgui.IsItemDeactivatedAfterEdit() {
				commit = true
			}
			if imgui.Button(ui.Icon(ui.FaTrash, "Delete note")) {
				defer a.drawingDelete()
			}
		}
	}

	imgui.SeparatorText(ui.Icon(icDrawing, "Sheet"))
	text("Name##sname", &s.Name)
	var sizes []string
	for _, z := range drawing.Sizes {
		sizes = append(sizes, z.Name)
	}
	if combo("Size##ssize", &s.Size, sizes, nil) {
		commit, dm.fit = true, true
	}
	if imgui.Checkbox("Portrait##sport", &s.Portrait) {
		commit, dm.fit = true, true
	}
	ang := map[bool]string{false: "third", true: "first"}[s.FirstAngle]
	if combo("Projection##sang", &ang, []string{"third", "first"}, func(x string) string { return map[string]string{"third": "Third angle", "first": "First angle"}[x] }) {
		s.FirstAngle = ang == "first"
		commit = true
	}
	edited(combo("Scale##sscale", &s.Scale, drawing.Scales, nil))
	pl := int32(s.Places)
	if imgui.SliderInt("Decimals##splaces", &pl, 0, 4) {
		s.Places = int(pl)
	}
	if imgui.IsItemDeactivatedAfterEdit() {
		commit = true
	}
	imgui.SeparatorText("Title block")
	t := &s.Title
	text("Title##tt", &t.Title)
	text("Drawing no.##tn", &t.Number)
	text("Material##tm", &t.Material)
	text("Finish##tf", &t.Finish)
	text("Drawn by##ta", &t.Author)
	text("Checked by##tc", &t.Checked)
	text("Date##td", &t.Date)
	text("Revision##tr", &t.Revision)
	text("Company##tco", &t.Company)
	imgui.Spacing()
	if imgui.Button(ui.Icon(ui.FaTrash, "Delete sheet")) {
		defer a.deleteSheet()
	}
	if commit {
		a.doc.Commit()
	}
}

func pickF(bit int, a, b float64) float64 {
	if bit != 0 {
		return b
	}
	return a
}

// drawingStatus is the status bar's text in a drawing.
func (a *App) drawingStatus() string {
	dm := a.dw
	s := a.dwSheet()
	if s == nil || dm.res == nil {
		return ""
	}
	out := fmt.Sprintf("Sheet %s · %s · scale %s", s.Name, s.Size, s.Scale)
	if a.vp.hovered && !math.IsNaN(dm.mouse.X) && math.Abs(dm.mouse.X) < 1e5 && math.Abs(dm.mouse.Y) < 1e5 {
		out += fmt.Sprintf(" · x %.1f  y %.1f mm", dm.mouse.X, dm.mouse.Y)
	}
	return out
}
