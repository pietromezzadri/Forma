package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	imgui "github.com/AllenDang/cimgui-go/imgui"
	"github.com/go-gl/mathgl/mgl32"

	"forma/brep"
	"forma/doc"
	fio "forma/io"
	"forma/render"
	"forma/ui"
)

// Exchange: export the visible bodies as STEP, STL, OBJ or 3MF (by the file's
// extension); import STEP solids as bodies and STL meshes as references.

func (a *App) registerExchangeCommands(c *Commands) {
	c.add(&Command{ID: "file.export", Label: "Export… (STEP, STL, OBJ, 3MF)", Group: "File", Icon: ui.FaDownload, Shortcut: "Ctrl+E",
		Run: func(arg string) {
			if arg != "" {
				a.exportFile(arg)
				return
			}
			a.pickFile(true, "Export", "Export", a.exportStart(), exportFilter, func(p string) {
				a.exportFile(withExt(p, ".step", ".step", ".stp", ".stl", ".obj", ".3mf"))
			})
		}})
	c.add(&Command{ID: "file.import", Label: "Import… (STEP, STL, 3MF)", Group: "File", Icon: icImport, Shortcut: "Ctrl+I",
		Run: func(arg string) {
			if arg != "" {
				a.importFile(arg)
				return
			}
			a.pickFile(false, "Import a STEP, STL or 3MF file", "Import", startIn(a.importPath, ""), importFilter, a.importFile)
		}})
}

// registerSeparateCommands: a 3MF's (or STL's) parts written as STL files
// of their own, for printing one by one; the design is left alone.
func (a *App) registerSeparateCommands(c *Commands) {
	for _, loose := range []bool{false, true} {
		id, label := "file.separate", "Separate a 3MF into part files… (to print them one by one)"
		if loose {
			id, label = "file.separate.loose", "Separate a 3MF or STL into its loose pieces…"
		}
		c.add(&Command{ID: id, Label: label, Group: "File", Icon: icSplit,
			Run: func(arg string) {
				if arg != "" {
					a.separateFile(arg, loose)
					return
				}
				a.pickFile(false, "Separate a 3MF or STL into its parts", "Separate", startIn(a.importPath, ""), separateFilter,
					func(p string) { a.separateFile(p, loose) })
			}})
	}
}

// separateFile writes path's parts (and, with loose, each part's loose
// pieces) as STL files into "<file> parts" beside it, on a goroutine.
func (a *App) separateFile(path string, loose bool) {
	if a.separating {
		a.notify("Still separating the last file: wait for it to finish")
		return
	}
	a.importPath = path
	dir := filepath.Join(filepath.Dir(path), strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+" parts")
	a.separating = true
	a.notify("Separating %s…", filepath.Base(path))
	go func() {
		res, err := fio.SeparateParts(path, dir, loose, func(s string) {
			a.post(func() { a.toast.show("Separating: " + s) })
		})
		a.post(func() {
			a.separating = false
			if err != nil {
				a.notify("Separating %s failed: %v", filepath.Base(path), err)
				return
			}
			tris := 0
			for _, r := range res {
				tris += r.Triangles
			}
			a.notify("Separated %s into %d %s (%d triangles) in %s", filepath.Base(path), len(res), plural(len(res), "part", "parts"), tris, dir)
		})
	}()
}

const icImport = 0xf093 // upload

func (a *App) exportFile(path string) {
	a.exportPath = path
	if err := a.writeExport(path); err != nil {
		a.notify("Export failed: %v", err)
		return
	}
	a.notify("Exported %s", filepath.Base(path))
}

// writeExport writes the visible bodies in the format the extension names.
func (a *App) writeExport(path string) error {
	var bodies []*brep.Body
	for _, b := range a.doc.Bodies {
		if b.Visible && b.Body != nil {
			bodies = append(bodies, b.Body)
		}
	}
	if len(bodies) == 0 {
		return fmt.Errorf("nothing to export")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	fine := func(b *brep.Body) *brep.Mesh {
		opt := brep.DefaultMeshOptions(b)
		opt.ChordTol = min(opt.ChordTol, 0.01)
		return brep.Tessellate(b, opt)
	}
	name := "design"
	if a.doc.Path != "" {
		name = strings.TrimSuffix(filepath.Base(a.doc.Path), ".forma")
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".step", ".stp":
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		visible := map[int]bool{}
		for _, b := range a.doc.Bodies {
			visible[b.ID] = b.Visible
		}
		asm := a.doc.D.StepAssembly(name, func(r *doc.BodyRef) bool { return visible[r.Key] })
		if err := fio.WriteSTEPAssembly(f, asm, fio.StepOptions{Name: name}); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	case ".stl":
		var meshes []*brep.Mesh
		for _, b := range bodies {
			meshes = append(meshes, fine(b))
		}
		return fio.SaveSTL(path, name, meshes...)
	case ".obj":
		var items []fio.OBJItem
		for _, b := range bodies {
			items = append(items, fio.OBJItem{Name: b.Name, Mesh: fine(b)})
		}
		return fio.SaveOBJ(path, items)
	case ".3mf":
		// each visible body a part in its step's colour, the parts one object
		// (a slicer keeps them where they stand to each other)
		var items []fio.ThreeMFItem
		for _, b := range a.doc.Bodies {
			if !b.Visible || b.Body == nil {
				continue
			}
			col := [3]float64{float64(colBody[0]), float64(colBody[1]), float64(colBody[2])}
			if c, ok := doc.ParseColor(doc.ColorOf(b.F, b.K)); ok {
				col = c
			}
			items = append(items, fio.ThreeMFItem{Name: b.Name, Mesh: fine(b.Body), Color: col})
		}
		return fio.Save3MF(path, name, "Forma "+Version, items, true)
	}
	return fmt.Errorf("%q: name the file .step, .stl, .obj or .3mf", filepath.Ext(path))
}

func (a *App) importFile(path string) {
	a.importPath = path
	data, err := os.ReadFile(path)
	if err != nil {
		a.notify("Import failed: %v", err)
		return
	}
	var f *doc.Feature
	switch strings.ToLower(filepath.Ext(path)) {
	case ".step", ".stp":
		f = doc.NewImport(path, string(data))
	case ".3mf":
		if _, _, err := doc.Check3MF(data); err != nil {
			a.notify("Import: %v", err)
			return
		}
		f = doc.NewImport3MF(path, data)
	case ".stl":
		f = doc.NewMesh(path, data)
	default:
		a.notify("Import: %q isn't a STEP (.step, .stp), STL or 3MF file", filepath.Ext(path))
		return
	}
	a.doc.D.Insert(f)
	a.doc.Commit()
	if f.Err != "" {
		a.notify("Import failed: %s", f.Err)
		a.doc.Remove(f.ID)
		return
	}
	switch {
	case f.Warn != "":
		a.notify("Imported %s, but %s", filepath.Base(path), f.Warn)
	case f.Kind == doc.KMesh:
		a.notify("Imported %s as a reference mesh (%d triangles)", filepath.Base(path), len(f.RefMesh.Tris))
	default:
		a.notify("Imported %s: %d %s", filepath.Base(path), len(f.Bodies), plural(len(f.Bodies), "body", "bodies"))
	}
	a.cam.Fit(a.doc.Bounds(), a.aspect(), false)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// MeshItem is a reference mesh in the view.
type MeshItem struct {
	ID      int
	Name    string
	F       *doc.Feature
	Mesh    *brep.Mesh
	Visible bool
	gen     int
	gpu     *render.TriMesh
	mass    *brep.MassProps
}

func (m *MeshItem) upload() *render.TriMesh {
	if m.gpu == nil && m.Mesh != nil {
		ids := make([]uint32, len(m.Mesh.Pos))
		m.gpu = render.NewTriMesh(m.Mesh.Pos, m.Mesh.Nrm, ids, m.Mesh.Tris)
	}
	return m.gpu
}

func (m *MeshItem) free() {
	if m.gpu != nil {
		m.gpu.Delete()
		m.gpu = nil
	}
}

// syncMeshes keeps an item per active mesh feature.
func (d *Document) syncMeshes() {
	var out []*MeshItem
	seen := map[int]bool{}
	for _, f := range d.D.Features {
		if !f.Active || f.Kind != doc.KMesh || f.RefMesh == nil {
			continue
		}
		id := bodyKey(f.ID, 0)
		it := d.meshes[id]
		if it == nil || it.gen != f.Gen {
			if it != nil {
				it.free()
			}
			it = &MeshItem{ID: id, F: f, Mesh: f.RefMesh, gen: f.Gen}
			d.meshes[id] = it
		}
		it.F, it.Name, it.Visible = f, f.Name, !f.Hidden
		out = append(out, it)
		seen[id] = true
	}
	for id, it := range d.meshes {
		if !seen[id] {
			it.free()
			delete(d.meshes, id)
		}
	}
	d.Meshes = out
}

var colMesh = mgl32.Vec4{0.62, 0.7, 0.78, 1}

func (a *App) drawMeshes(v render.View, inSketch bool) {
	for _, m := range a.doc.Meshes {
		if !m.Visible {
			continue
		}
		g := m.upload()
		if g == nil {
			continue
		}
		col := colMesh
		if inSketch {
			col[3] = 0.25
		}
		if a.browserSel == m.ID {
			col = mgl32.Vec4{0.95, 0.68, 0.4, 1}
		}
		a.r.DrawFaces(v, g, render.Paint{Color: col}, false)
	}
}

// importProperties shows what an import or mesh feature holds.
func (a *App) importProperties(f *doc.Feature) bool {
	switch f.Kind {
	case doc.KImport:
		imgui.TextDisabled("From " + f.Inputs["File"])
		if f.Warn != "" {
			imgui.PushStyleColorVec4(imgui.ColText, colWarn)
			imgui.TextWrapped(f.Warn)
			imgui.PopStyleColor()
		}
		imgui.Text(fmt.Sprintf("%d %s", len(f.Bodies), plural(len(f.Bodies), "solid", "solids")))
		a.opCombo(f)
		return true
	case doc.KMesh:
		imgui.TextDisabled("From " + f.Inputs["File"])
		imgui.TextWrapped("A reference mesh: shown, measured, not joined or cut.")
		it := a.doc.meshes[bodyKey(f.ID, 0)]
		if it == nil || it.Mesh == nil {
			return true
		}
		b := doc.MeshBounds(it.Mesh)
		s := b.Max.Sub(b.Min)
		imgui.Text(fmt.Sprintf("%d triangles", len(it.Mesh.Tris)))
		imgui.Text(fmt.Sprintf("Size %s × %s × %s mm", fmtSig(s.X), fmtSig(s.Y), fmtSig(s.Z)))
		if it.mass == nil {
			mp := brep.MeshMass(it.Mesh)
			it.mass = &mp
		}
		if err := it.Mesh.Watertight(1e-4 * b.Diagonal()); err == nil {
			imgui.Text(fmt.Sprintf("Volume %s mm³", fmtSig(it.mass.Volume)))
		} else {
			imgui.TextDisabled("Open (not watertight): no volume")
		}
		imgui.Text(fmt.Sprintf("Area %s mm²", fmtSig(it.mass.Area)))
		return true
	}
	return false
}

// meshBrowser lists the reference meshes.
func (a *App) meshBrowser(eye func(*doc.Feature)) {
	if len(a.doc.Meshes) == 0 {
		return
	}
	if imgui.TreeNodeExStrV(ui.Icon(icMesh, "Meshes")+"###meshes", imgui.TreeNodeFlagsDefaultOpen|imgui.TreeNodeFlagsSpanAvailWidth) {
		for _, m := range a.doc.Meshes {
			eye(m.F)
			if imgui.SelectableBoolV(fmt.Sprintf("%s##m%d", m.Name, m.ID), a.browserSel == m.ID, 0, imgui.Vec2{}) {
				a.browserSel = m.ID
			}
			a.markItem("item:" + m.Name)
			a.featureMenu(m.F)
		}
		imgui.TreePop()
	}
}

const icMesh = 0xf1b3 // cubes

// exportStart is where Export opens: the last export, else the design's
// folder and name as a .step.
func (a *App) exportStart() string {
	if a.exportPath != "" {
		return a.exportPath
	}
	if a.doc.Path != "" {
		return strings.TrimSuffix(a.doc.Path, filepath.Ext(a.doc.Path)) + ".step"
	}
	return defaultPath("design.step")
}
