package app

// Static stress analysis in the app (M14): pick a body's faces to hold
// fixed and faces to load, a material and an element size, run the study
// on a goroutine, and see the result painted on the deformed part — von
// Mises stress or displacement — with a legend, the maximum stress and
// displacement and the safety factor.

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	imgui "github.com/AllenDang/cimgui-go/imgui"

	"forma/brep"
	"forma/fea"
	"forma/geom"
	"forma/render"
	"forma/ui"
)

const icStress = 0xf0e7 // bolt

type simLoad struct {
	Faces    []int
	Force    [3]string // N
	Pressure string    // MPa
	ByPress  bool
}

type simState struct {
	open     bool
	body     int // the BodyItem studied
	part     *brep.Body
	fixed    []int
	loads    []simLoad
	material int
	size     string // mm, "" picks one

	mu      sync.Mutex
	running bool
	stage   string
	frac    float64
	result  *fea.Result
	err     error
	newRes  bool

	show   int     // 0 von Mises, 1 displacement
	deform float32 // × the automatic scale
	plot   *render.ColorMesh
	built  struct {
		res    *fea.Result
		show   int
		deform float32
	}
}

func (a *App) registerSimCommands(c *Commands) {
	c.add(&Command{ID: "simulate.open", Label: "Stress analysis", Group: "Inspect", Icon: icStress,
		Run: func(string) { a.sim.open = !a.sim.open }})
	c.add(&Command{ID: "simulate.fix", Label: "Stress: hold the selected faces fixed", Group: "Inspect",
		Run: func(string) { a.simAddFixed() }})
	c.add(&Command{ID: "simulate.load", Label: "Stress: load the selected faces", Group: "Inspect",
		Run: func(arg string) { a.simAddLoad(arg) }})
	c.add(&Command{ID: "simulate.run", Label: "Stress: run the study", Group: "Inspect",
		Run: func(arg string) { a.simRun(arg == "now") }})
	c.add(&Command{ID: "simulate.show", Label: "Stress: show stress / displacement", Group: "Inspect",
		Run: func(arg string) {
			a.sim.show = 0
			if arg == "displacement" {
				a.sim.show = 1
			}
		}})
	c.add(&Command{ID: "simulate.clear", Label: "Stress: clear the study", Group: "Inspect",
		Run: func(string) { a.simClear() }})
}

// simFaces are the selected faces, all on one body (its ID, 0 if none).
func (a *App) simFaces() (int, []int) {
	body := 0
	var faces []int
	for _, r := range a.sel {
		if r.Kind != KindFace {
			continue
		}
		if body != 0 && r.Item != body {
			a.notify("Pick faces of one body")
			return 0, nil
		}
		body = r.Item
		faces = append(faces, r.Local)
	}
	if body == 0 {
		a.notify("Select faces of the part first")
	}
	return body, faces
}

// simBody switches the study to body (keeping what was set if it is the
// same one).
func (a *App) simBody(body int) bool {
	b := a.doc.Body(body)
	if b == nil {
		return false
	}
	if a.sim.body != body {
		a.simClear()
		a.sim.body = body
	}
	a.sim.part = b.Body
	a.sim.open = true
	return true
}

func (a *App) simAddFixed() {
	body, faces := a.simFaces()
	if body == 0 || !a.simBody(body) {
		return
	}
	a.sim.fixed = mergeIDs(a.sim.fixed, faces)
	a.sel = a.sel[:0]
}

// simAddLoad loads the selected faces: arg "fx,fy,fz" (N) or "p=<MPa>".
func (a *App) simAddLoad(arg string) {
	body, faces := a.simFaces()
	if body == 0 || !a.simBody(body) {
		return
	}
	l := simLoad{Faces: faces, Force: [3]string{"0", "0", "-100"}, Pressure: "1"}
	if strings.HasPrefix(arg, "p=") {
		l.ByPress, l.Pressure = true, strings.TrimPrefix(arg, "p=")
	} else if parts := strings.Split(arg, ","); len(parts) == 3 {
		copy(l.Force[:], parts)
	}
	a.sim.loads = append(a.sim.loads, l)
	a.sel = a.sel[:0]
}

func mergeIDs(a, b []int) []int {
	for _, x := range b {
		found := false
		for _, y := range a {
			found = found || x == y
		}
		if !found {
			a = append(a, x)
		}
	}
	return a
}

func (a *App) simClear() {
	s := &a.sim
	s.mu.Lock()
	busy := s.running
	s.mu.Unlock()
	if busy {
		return
	}
	s.fixed, s.loads, s.result, s.err = nil, nil, nil, nil
	s.body, s.part = 0, nil
	s.plot.Free()
	s.plot = nil
}

// simStudy is the study as set up, its numbers worked out.
func (a *App) simStudy() (fea.Study, error) {
	s := &a.sim
	b := a.doc.Body(s.body)
	if b == nil {
		return fea.Study{}, fmt.Errorf("pick the part's faces first")
	}
	st := fea.Study{Body: b.Body, Material: fea.Materials[s.material], Fixed: s.fixed}
	if s.size != "" {
		v, err := a.doc.D.Eval(s.size)
		if err != nil {
			return st, fmt.Errorf("element size: %v", err)
		}
		st.Size = v
	}
	for i, l := range s.loads {
		ld := fea.Load{Faces: l.Faces}
		if l.ByPress {
			p, err := a.doc.D.Eval(l.Pressure)
			if err != nil {
				return st, fmt.Errorf("load %d: %v", i+1, err)
			}
			ld.Pressure = p
		} else {
			var v [3]float64
			for k := range v {
				x, err := a.doc.D.Eval(l.Force[k])
				if err != nil {
					return st, fmt.Errorf("load %d: %v", i+1, err)
				}
				v[k] = x
			}
			ld.Force = geom.V3(v[0], v[1], v[2])
		}
		st.Loads = append(st.Loads, ld)
	}
	if len(st.Loads) == 0 {
		return st, fmt.Errorf("add a load")
	}
	return st, nil
}

// simRun runs the study on a goroutine (now: on this one, for captures).
func (a *App) simRun(now bool) {
	s := &a.sim
	st, err := a.simStudy()
	if err != nil {
		a.notify("Stress analysis: %v", err)
		return
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running, s.stage, s.frac, s.err = true, "Starting", 0, nil
	s.part = st.Body
	s.mu.Unlock()
	st.Progress = func(stage string, frac float64) {
		s.mu.Lock()
		s.stage, s.frac = stage, frac
		s.mu.Unlock()
	}
	work := func() {
		r, err := fea.Run(st)
		s.mu.Lock()
		s.running, s.result, s.err, s.newRes = false, r, err, true
		s.mu.Unlock()
	}
	if now {
		work()
	} else {
		go work()
	}
}

// simHides: the studied body isn't drawn while its result is (the plot
// stands in for it).
func (a *App) simHides(id int) bool {
	s := &a.sim
	return s.open && s.result != nil && s.body == id && a.simFresh()
}

// simFresh: the result is of the part as it stands.
func (a *App) simFresh() bool {
	b := a.doc.Body(a.sim.body)
	return b != nil && a.sim.result != nil && b.Body == a.sim.part
}

// simWindow is the study's window.
func (a *App) simWindow() {
	s := &a.sim
	s.mu.Lock()
	if s.newRes {
		s.newRes = false
		if s.err != nil {
			a.notify("Stress analysis: %v", s.err)
		} else if s.result != nil {
			a.notify("Solved: %d elements in %.1f s; %s", len(s.result.Mesh.Tets), s.result.Took.Seconds(), a.simSummary())
			if s.deform == 0 {
				s.deform = 1
			}
		}
	}
	running, stage, frac, res, runErr := s.running, s.stage, s.frac, s.result, s.err
	s.mu.Unlock()
	if !s.open {
		return
	}
	imgui.SetNextWindowSizeV(imgui.Vec2{X: 330, Y: 0}, imgui.CondAppearing)
	open := true
	if imgui.BeginV(ui.Icon(icStress, "Stress analysis")+"###stress", &open, imgui.WindowFlagsNoSavedSettings|imgui.WindowFlagsAlwaysAutoResize) {
		b := a.doc.Body(s.body)
		if b == nil {
			imgui.TextWrapped("Select faces of a part, then hold them fixed or load them.")
		} else {
			imgui.Text("Part: " + b.Name)
		}
		imgui.SeparatorText("Supports")
		imgui.Text(fmt.Sprintf("%d faces held fixed", len(s.fixed)))
		if imgui.Button("Hold selected faces##fix") {
			a.simAddFixed()
		}
		imgui.SeparatorText("Loads")
		for i := range s.loads {
			l := &s.loads[i]
			imgui.PushIDInt(int32(i))
			imgui.Text(fmt.Sprintf("Load %d · %d faces", i+1, len(l.Faces)))
			imgui.SameLine()
			if imgui.SmallButton("x") {
				s.loads = append(s.loads[:i], s.loads[i+1:]...)
				imgui.PopID()
				break
			}
			imgui.Checkbox("Pressure", &l.ByPress)
			if l.ByPress {
				imgui.SetNextItemWidth(110)
				imgui.InputTextWithHint("MPa##p", "1", &l.Pressure, 0, nil)
			} else {
				for k, ax := range []string{"Fx", "Fy", "Fz"} {
					if k > 0 {
						imgui.SameLine()
					}
					imgui.SetNextItemWidth(70)
					imgui.InputTextWithHint(ax+"##f"+ax, "0", &l.Force[k], 0, nil)
				}
				imgui.SameLine()
				imgui.TextDisabled("N")
			}
			imgui.PopID()
		}
		if imgui.Button("Load selected faces##load") {
			a.simAddLoad("")
		}
		imgui.SeparatorText("Material and mesh")
		names := make([]string, len(fea.Materials))
		for i, m := range fea.Materials {
			names[i] = m.Name
		}
		if cur, ok := a.combo("Material##mat", names[s.material], names, nil); ok {
			for i, n := range names {
				if n == cur {
					s.material = i
				}
			}
		}
		m := fea.Materials[s.material]
		imgui.TextDisabled(fmt.Sprintf("E %s MPa · Poisson %.2f · yield %s MPa", fmtSig(m.E), m.Nu, fmtSig(m.Yield)))
		imgui.SetNextItemWidth(110)
		imgui.InputTextWithHint("Element size mm##size", "auto", &s.size, 0, nil)
		imgui.Separator()
		if running {
			imgui.ProgressBarV(float32(frac), imgui.Vec2{X: -1, Y: 0}, stage)
		} else if imgui.Button(ui.Icon(icStress, "Run")) {
			a.simRun(false)
		}
		if runErr != nil && !running {
			imgui.TextColored(colWarn, runErr.Error())
		}
		if res != nil && !running {
			a.simResults(res)
		}
		imgui.SameLine()
		if imgui.Button("Close##simclose") {
			open = false
		}
	}
	imgui.End()
	if !open {
		s.open = false
	}
}

// simResults shows the numbers and the plot's settings and legend.
func (a *App) simResults(r *fea.Result) {
	s := &a.sim
	imgui.SeparatorText("Results")
	if !a.simFresh() {
		imgui.TextColored(colWarn, "The part has changed: run it again")
		return
	}
	imgui.Text(fmt.Sprintf("Max von Mises  %s MPa", fmtSig(r.MaxVM)))
	imgui.Text(fmt.Sprintf("Max displacement  %s mm", fmtSig(r.MaxU)))
	sf := fmt.Sprintf("Safety factor  %.2f", r.Safety)
	if r.Safety < 1 {
		imgui.TextColored(colWarn, sf+"  (yields)")
	} else {
		imgui.Text(sf)
	}
	imgui.TextDisabled(fmt.Sprintf("%d elements · %d nodes · %.1f s", len(r.Mesh.Tets), len(r.Mesh.Nodes), r.Took.Seconds()))
	if imgui.RadioButtonBool("Stress", s.show == 0) {
		s.show = 0
	}
	imgui.SameLine()
	if imgui.RadioButtonBool("Displacement", s.show == 1) {
		s.show = 1
	}
	imgui.SetNextItemWidth(160)
	imgui.SliderFloatV("Deformation##def", &s.deform, 0, 3, "×%.2f", 0)
	// the legend
	lo, hi, unit := 0.0, r.MaxVM, "MPa"
	if s.show == 1 {
		hi, unit = r.MaxU, "mm"
	}
	dl := imgui.WindowDrawList()
	p := imgui.CursorScreenPos()
	w, h := float32(280), float32(14)
	const n = 8
	for i := 0; i < n; i++ {
		x0 := p.X + w*float32(i)/n
		x1 := p.X + w*float32(i+1)/n
		c0, c1 := ramp(float64(i)/n), ramp(float64(i+1)/n)
		u0, u1 := u32(c0[0], c0[1], c0[2], 1), u32(c1[0], c1[1], c1[2], 1)
		dl.AddRectFilledMultiColor(imgui.Vec2{X: x0, Y: p.Y}, imgui.Vec2{X: x1, Y: p.Y + h}, u0, u1, u1, u0)
	}
	imgui.Dummy(imgui.Vec2{X: w, Y: h})
	imgui.Text(fmtSig(lo))
	imgui.SameLine()
	label := fmtSig(hi) + " " + unit
	imgui.SetCursorPosX(imgui.CursorPosX() + w - imgui.CalcTextSize(label).X - imgui.CalcTextSize(fmtSig(lo)).X - 8)
	imgui.Text(label)
}

// ramp is the plot's colour scale: blue, cyan, green, yellow, red.
func ramp(t float64) [3]float32 {
	t = math.Max(0, math.Min(1, t))
	stops := [][3]float32{{0.17, 0.30, 0.78}, {0.15, 0.70, 0.85}, {0.30, 0.78, 0.35}, {0.97, 0.85, 0.20}, {0.86, 0.20, 0.16}}
	f := t * float64(len(stops)-1)
	i := int(f)
	if i >= len(stops)-1 {
		return stops[len(stops)-1]
	}
	k := float32(f - float64(i))
	a, b := stops[i], stops[i+1]
	return [3]float32{a[0] + (b[0]-a[0])*k, a[1] + (b[1]-a[1])*k, a[2] + (b[2]-a[2])*k}
}

// drawSimulation draws the study in the viewport: the fixed and loaded
// faces picked out before it runs, the coloured result after.
func (a *App) drawSimulation(v render.View) {
	s := &a.sim
	if !s.open {
		return
	}
	if a.simFresh() && s.result != nil {
		a.simBuildPlot()
		if err := a.r.DrawColored(v, s.plot); err != nil {
			a.notify("%v", err)
			s.open = false
		}
		return
	}
	b := a.doc.Body(s.body)
	if b == nil || b.gpu == nil {
		return
	}
	for _, f := range s.fixed {
		a.r.DrawFaceRange(v, b.gpu.faces, uint32(f), render.Paint{Color: [4]float32{0.25, 0.45, 0.95, 1}})
	}
	for _, l := range s.loads {
		for _, f := range l.Faces {
			a.r.DrawFaceRange(v, b.gpu.faces, uint32(f), render.Paint{Color: [4]float32{0.95, 0.55, 0.15, 1}})
		}
	}
}

// simBuildPlot makes the coloured mesh of the result when it or how it is
// shown changes: the mesh's boundary triangles, each cut into four through
// its edge mids, moved by the displacement × the scale.
func (a *App) simBuildPlot() {
	s := &a.sim
	r := s.result
	if s.plot != nil && s.built.res == r && s.built.show == s.show && s.built.deform == s.deform {
		return
	}
	s.plot.Free()
	s.built.res, s.built.show, s.built.deform = r, s.show, s.deform
	m := r.Mesh
	// the automatic scale shows the largest displacement as 5 % of the part
	scale := 0.0
	if r.MaxU > 0 {
		bb := s.part.Bounds()
		scale = 0.05 * bb.Diagonal() / r.MaxU * float64(s.deform)
	}
	val := func(i int) float64 {
		if s.show == 1 {
			if r.MaxU == 0 {
				return 0
			}
			return r.U[i].Len() / r.MaxU
		}
		if r.MaxVM == 0 {
			return 0
		}
		return r.VonMises[i] / r.MaxVM
	}
	at := func(i int) geom.Vec3 { return m.Nodes[i].Add(r.U[i].Scale(scale)) }
	var pos, nrm []geom.Vec3
	var col [][3]float32
	var tris [][3]int32
	sub := [4][3]int{{0, 3, 5}, {3, 1, 4}, {5, 4, 2}, {3, 4, 5}}
	for _, bt := range m.Bound {
		for _, t := range sub {
			p0, p1, p2 := at(bt.N[t[0]]), at(bt.N[t[1]]), at(bt.N[t[2]])
			n := p1.Sub(p0).Cross(p2.Sub(p0)).Norm()
			base := int32(len(pos))
			for _, k := range t {
				pos = append(pos, at(bt.N[k]))
				nrm = append(nrm, n)
				col = append(col, ramp(val(bt.N[k])))
			}
			tris = append(tris, [3]int32{base, base + 1, base + 2})
		}
	}
	s.plot = render.NewColorMesh(pos, nrm, col, tris)
}

// simSummary is the result in a line (for the capture log).
func (a *App) simSummary() string {
	r := a.sim.result
	if r == nil {
		return "no result"
	}
	return "max von Mises " + strconv.FormatFloat(r.MaxVM, 'f', 2, 64) + " MPa, max u " +
		strconv.FormatFloat(r.MaxU, 'f', 4, 64) + " mm, safety " + strconv.FormatFloat(r.Safety, 'f', 2, 64)
}
