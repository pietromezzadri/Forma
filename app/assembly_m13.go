package app

// M13 in the app: grounding a component, limits on a joint's motion, the
// bill of materials window (and its CSV), and the exploded view.

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	imgui "github.com/AllenDang/cimgui-go/imgui"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/ncruces/zenity"

	"forma/doc"
	"forma/geom"
	"forma/ui"
)

const (
	icGround  = 0xf08d // thumbtack
	icBOM     = 0xf0ce // table
	icExplode = 0xf0b2 // arrows out
)

type m13State struct {
	showBOM     bool
	bom         []doc.BOMRow
	bomGen      int
	bomDoc      *doc.Document
	showExplode bool
	explode     float32
	offsets     map[int]mgl32.Vec3 // component → drawn offset
	offsetsGen  int
	offsetsDoc  *doc.Document
	offsetsAt   float32
}

func (a *App) registerM13Commands(c *Commands) {
	c.add(&Command{ID: "assemble.ground", Label: "Ground / unground component", Group: "Assemble", Icon: icGround,
		Enabled: func() bool { return a.selComponent() != nil },
		Run: func(string) {
			if f := a.selComponent(); f != nil {
				a.setGrounded(f, !doc.Grounded(f))
			}
		}})
	c.add(&Command{ID: "assemble.bom", Label: "Bill of materials", Group: "Assemble", Icon: icBOM,
		Run: func(string) { a.m13.showBOM = !a.m13.showBOM }})
	c.add(&Command{ID: "assemble.bomcsv", Label: "Export bill of materials (CSV)…", Group: "Assemble", Icon: icBOM,
		Run: func(arg string) {
			if arg != "" {
				a.writeBOM(arg)
				return
			}
			a.pickFile(true, "Export the bill of materials", "Export", startIn(a.doc.Path, "bom.csv"), csvFilter, a.writeBOM)
		}})
	c.add(&Command{ID: "assemble.explode", Label: "Exploded view", Group: "Assemble", Icon: icExplode,
		Run: func(arg string) {
			if arg != "" {
				v, err := strconv.ParseFloat(arg, 64)
				if err == nil {
					a.m13.explode, a.m13.showExplode = float32(v), v > 0
				}
				return
			}
			a.m13.showExplode = !a.m13.showExplode
			if a.m13.showExplode && a.m13.explode == 0 {
				a.m13.explode = 0.3
			}
			if !a.m13.showExplode {
				a.m13.explode = 0
			}
		}})
}

var csvFilter = zenity.FileFilters{{Name: "Comma-separated values (*.csv)", Patterns: []string{"*.csv"}}}

func (a *App) setGrounded(f *doc.Feature, on bool) {
	f.Inputs["Grounded"] = map[bool]string{true: "1", false: "0"}[on]
	a.doc.Commit()
	if on {
		a.notify("%s is grounded: joints move the other part", f.Name)
	}
}

// groundedProperty is a component's Grounded checkbox.
func (a *App) groundedProperty(f *doc.Feature) {
	g := doc.Grounded(f)
	if imgui.Checkbox(ui.Icon(icGround, "Grounded")+"##ground", &g) {
		a.setGrounded(f, g)
	}
	if g {
		imgui.TextDisabled("Stays put: joints move the other end, the gizmo is off")
	}
}

// limitProperties sets a joint's limits on input (Angle or Distance).
func (a *App) limitProperties(f *doc.Feature, input string) {
	on := f.Inputs[input+" min"] != "" || f.Inputs[input+" max"] != ""
	if imgui.Checkbox("Limits##lim"+input, &on) {
		if on {
			lo, hi := "-90", "90"
			if input == "Distance" {
				lo, hi = "0", "20"
			}
			f.Inputs[input+" min"], f.Inputs[input+" max"] = lo, hi
		} else {
			delete(f.Inputs, input+" min")
			delete(f.Inputs, input+" max")
		}
		a.doc.Commit()
	}
	if on {
		unit := " mm"
		if input == "Angle" {
			unit = "°"
		}
		imgui.TextDisabled("From")
		a.exprField(f, input+" min", unit)
		imgui.TextDisabled("To")
		a.exprField(f, input+" max", unit)
	}
	if f.Warn != "" {
		imgui.TextColored(colWarn, f.Warn)
	}
}

// motionRange is the slider's range for a joint's motion: its limits, or
// a turn / the design's size.
func (a *App) motionRange(f *doc.Feature, input string) (float64, float64) {
	lo, hi := -180.0, 180.0
	if input == "Distance" {
		r := a.doc.Bounds().Diagonal()
		lo, hi = -r, r
	}
	if l, h, ok := a.doc.D.Limits(f, input); ok {
		if !math.IsInf(l, 0) {
			lo = l
		}
		if !math.IsInf(h, 0) {
			hi = h
		}
	}
	return lo, hi
}

// bomWindow shows the bill of materials.
func (a *App) bomWindow() {
	if !a.m13.showBOM {
		return
	}
	if a.m13.bom == nil || a.m13.bomGen != a.doc.D.Gen() || a.m13.bomDoc != a.doc.D {
		a.m13.bom, a.m13.bomGen, a.m13.bomDoc = a.doc.D.BOM(), a.doc.D.Gen(), a.doc.D
	}
	rho, _ := a.doc.D.Eval(a.density)
	imgui.SetNextWindowSizeV(imgui.Vec2{X: 620, Y: 0}, imgui.CondAppearing)
	open := true
	if imgui.BeginV(ui.Icon(icBOM, "Bill of materials")+"###bom", &open, imgui.WindowFlagsNoSavedSettings|imgui.WindowFlagsAlwaysAutoResize) {
		if len(a.m13.bom) == 0 {
			imgui.Text("No parts yet.")
		} else if imgui.BeginTableV("##bomt", 7, imgui.TableFlagsRowBg|imgui.TableFlagsBorders|imgui.TableFlagsSizingFixedFit, imgui.Vec2{}, 0) {
			for _, h := range []string{"#", "Part", "File", "Qty", "Volume mm³", "Mass g", "Total g"} {
				imgui.TableSetupColumn(h)
			}
			imgui.TableHeadersRow()
			total := 0.0
			for _, r := range a.m13.bom {
				imgui.TableNextRow()
				imgui.TableNextColumn()
				imgui.Text(strconv.Itoa(r.Item))
				imgui.TableNextColumn()
				name := r.Name
				if r.Ground {
					name += "  " + string(rune(icGround))
				}
				imgui.Text(name)
				imgui.TableNextColumn()
				file := r.File
				if r.Comp == 0 {
					file = "(this design)"
				}
				imgui.TextDisabled(file)
				imgui.TableNextColumn()
				imgui.Text(strconv.Itoa(r.Qty))
				imgui.TableNextColumn()
				imgui.Text(fmtSig(r.Volume))
				m := r.Volume * rho / 1000
				total += m * float64(r.Qty)
				imgui.TableNextColumn()
				imgui.Text(fmtSig(m))
				imgui.TableNextColumn()
				imgui.Text(fmtSig(m * float64(r.Qty)))
			}
			imgui.EndTable()
			imgui.Text(fmt.Sprintf("%d parts, %s g in all at %s g/cm³", len(a.m13.bom), fmtSig(total), a.density))
		}
		imgui.SetNextItemWidth(90)
		imgui.InputTextWithHint("Density g/cm³##bomdens", "7.85", &a.density, imgui.InputTextFlagsAutoSelectAll, nil)
		if imgui.Button(ui.Icon(icBOM, "Export CSV…")) {
			a.cmds.Run("assemble.bomcsv", "")
		}
		imgui.SameLine()
		if imgui.Button("Close") {
			open = false
		}
	}
	imgui.End()
	if !open {
		a.m13.showBOM = false
	}
}

func (a *App) writeBOM(path string) {
	if !strings.HasSuffix(strings.ToLower(path), ".csv") {
		path += ".csv"
	}
	rho, _ := a.doc.D.Eval(a.density)
	f, err := os.Create(path)
	if err != nil {
		a.notify("Export failed: %v", err)
		return
	}
	err = doc.WriteBOM(f, a.doc.D.BOM(), rho)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		a.notify("Export failed: %v", err)
		return
	}
	a.notify("Wrote the bill of materials to %s", filepath.Base(path))
}

// explodeWindow is the exploded view's slider.
func (a *App) explodeWindow() {
	if !a.m13.showExplode {
		return
	}
	imgui.SetNextWindowSizeV(imgui.Vec2{X: 300, Y: 0}, imgui.CondAppearing)
	open := true
	if imgui.BeginV(ui.Icon(icExplode, "Exploded view")+"###explode", &open, imgui.WindowFlagsNoSavedSettings|imgui.WindowFlagsAlwaysAutoResize) {
		imgui.SetNextItemWidth(200)
		imgui.SliderFloatV("Spread##exp", &a.m13.explode, 0, 1, "%.2f", 0)
		imgui.TextDisabled("Parts come apart along their joints;")
		imgui.TextDisabled("grounded ones stay. Only the view moves.")
		if imgui.Button("Assemble") {
			open = false
		}
	}
	imgui.End()
	if !open {
		a.m13.showExplode, a.m13.explode = false, 0
	}
}

// explodeOffset is where a body is drawn from where it is: its part's
// exploded offset (group: doc.ExplodeGroup).
func (a *App) explodeOffset(comp int) mgl32.Vec3 {
	if a.m13.explode == 0 {
		return mgl32.Vec3{}
	}
	if a.m13.offsets == nil || a.m13.offsetsGen != a.doc.D.Gen() || a.m13.offsetsDoc != a.doc.D || a.m13.offsetsAt != a.m13.explode {
		a.m13.offsets = map[int]mgl32.Vec3{}
		for c, v := range a.doc.D.Explode(float64(a.m13.explode)) {
			a.m13.offsets[c] = mgl32.Vec3{float32(v.X), float32(v.Y), float32(v.Z)}
		}
		a.m13.offsetsGen, a.m13.offsetsDoc, a.m13.offsetsAt = a.doc.D.Gen(), a.doc.D, a.m13.explode
	}
	return a.m13.offsets[comp]
}

// bodyIcon is a body's icon in the browser: a pin for a grounded
// component's.
func bodyIcon(f *doc.Feature) rune {
	if doc.Grounded(f) {
		return icGround
	}
	return kindIcon(f.Kind)
}

// fitBounds is what Fit frames: the design, its components where the
// exploded view draws them.
func (a *App) fitBounds() geom.Box {
	b := a.doc.Bounds()
	if a.m13.explode == 0 {
		return b
	}
	for _, it := range a.doc.Bodies {
		if !it.Visible {
			continue
		}
		o := a.explodeOffset(doc.ExplodeGroup(it.Comp, it.ID))
		bb := it.Body.Bounds()
		d := geom.V3(float64(o[0]), float64(o[1]), float64(o[2]))
		b.Add(bb.Min.Add(d))
		b.Add(bb.Max.Add(d))
	}
	return b
}
