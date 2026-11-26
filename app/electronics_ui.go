package app

// Electronics in the app (M15): a circuit feature — an Arduino Uno, the
// sketch it runs and library parts wired to its pins — edited in its own
// window, and a motion study that runs the sketch on the emulated board
// and lets its servos and motors drive the assembly's joints, with the
// pots and buttons to play with, the LEDs lit and the serial monitor.

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	imgui "github.com/AllenDang/cimgui-go/imgui"
	"github.com/ncruces/zenity"

	"forma/avr"
	"forma/doc"
	"forma/elec"
	"forma/geom"
	"forma/ui"
)

const icCircuit = 0xf2db // microchip

var firmwareFilter = zenity.FileFilters{
	{Name: "Arduino firmware and sketches (*.hex, *.ino)", Patterns: []string{"*.hex", "*.ino"}},
	{Name: "Intel HEX (*.hex)", Patterns: []string{"*.hex"}},
	{Name: "Arduino sketch (*.ino)", Patterns: []string{"*.ino"}},
}

type elecState struct {
	open      bool
	circuit   int // the feature shown
	study     *doc.Study
	playing   bool
	speed     float32
	last      time.Time
	compiling string // the sketch being compiled
	addModel  string
	noModel   bool // Add part leaves the 3D model out
	sheet     wireSheet
}

func (a *App) registerElecCommands(c *Commands) {
	c.add(&Command{ID: "electronics.open", Label: "Electronics", Group: "Assemble", Icon: icCircuit,
		Run: func(string) {
			a.elec.open = !a.elec.open
			if a.elec.open && a.elecCircuit() == nil {
				a.elecAdd()
			}
		}})
	c.add(&Command{ID: "electronics.add", Label: "Electronics: add a circuit (an Arduino Uno)", Group: "Assemble",
		Run: func(string) { a.elecAdd() }})
	c.add(&Command{ID: "electronics.firmware", Label: "Electronics: load firmware (.hex) or a sketch (.ino)", Group: "Assemble",
		Run: func(arg string) {
			if arg != "" {
				a.elecLoadFirmware(arg)
				return
			}
			a.pickFile(false, "Load firmware", "Load", startIn(a.doc.Path, ""), firmwareFilter, a.elecLoadFirmware)
		}})
	c.add(&Command{ID: "electronics.part", Label: "Electronics: add a part (model[=name])", Group: "Assemble",
		Run: func(arg string) {
			model, name, _ := strings.Cut(arg, "=")
			a.elecAddPart(model, name)
		}})
	c.add(&Command{ID: "electronics.wire", Label: "Electronics: wire a part (part.role=pin)", Group: "Assemble",
		Run: func(arg string) { a.elecCommand(arg, "wire") }})
	c.add(&Command{ID: "electronics.sheet", Label: "Electronics: the wiring sheet", Group: "Assemble", Icon: icWiring,
		Run: func(arg string) {
			sh := &a.elec.sheet
			sh.open = arg == "1" || arg == "" && !sh.open
			sh.fit = true
			if sh.open && a.elecCircuit() == nil {
				a.elecAdd()
			}
		}})
	c.add(&Command{ID: "electronics.wokwi", Label: "Electronics: open a Wokwi project (its diagram.json and sketch)", Group: "Assemble", Icon: icWiring,
		Run: func(arg string) {
			if arg != "" {
				a.elecWokwi(arg)
				return
			}
			a.pickFile(false, "Open a Wokwi project's diagram.json", "Open", startIn(a.doc.Path, ""), wokwiFilter, a.elecWokwi)
		}})
	c.add(&Command{ID: "electronics.connect", Label: "Electronics: wire two terminals (part.term=part.term[=colour])", Group: "Assemble",
		Run: func(arg string) { a.elecConnect(arg, true) }})
	c.add(&Command{ID: "electronics.disconnect", Label: "Electronics: take the wire between two terminals off (part.term=part.term)", Group: "Assemble",
		Run: func(arg string) { a.elecConnect(arg, false) }})
	c.add(&Command{ID: "electronics.place", Label: "Electronics: put a part on the wiring sheet (part=x,y)", Group: "Assemble",
		Run: func(arg string) { a.elecPlace(arg) }})
	c.add(&Command{ID: "electronics.drive", Label: "Electronics: drive a joint (part=joint[,ratio[,zero]])", Group: "Assemble",
		Run: func(arg string) { a.elecCommand(arg, "drive") }})
	c.add(&Command{ID: "electronics.set", Label: "Electronics: set a part's figure (part.figure=value)", Group: "Assemble",
		Run: func(arg string) { a.elecCommand(arg, "set") }})
	c.add(&Command{ID: "electronics.model", Label: "Electronics: insert a part's 3D model as a component", Group: "Assemble", Icon: icComponent,
		Run: func(arg string) {
			// "id" or "id@part": the model, linked to that part
			id, part, _ := strings.Cut(arg, "@")
			m := a.elecInsertModel(id)
			if f := a.elecCircuit(); m != nil && f != nil && part != "" {
				for i := range f.Circuit.Parts {
					if f.Circuit.Parts[i].Name == part {
						f.Circuit.Parts[i].Model3D = m.ID
					}
				}
				a.doc.Commit()
			}
		}})
	c.add(&Command{ID: "electronics.press", Label: "Electronics: press or let go of a button (name=1|0)", Group: "Assemble",
		Run: func(arg string) {
			name, v, _ := strings.Cut(arg, "=")
			if st := a.elec.study; st != nil {
				switch q := st.Parts[name].(type) {
				case *elec.Button:
					q.Pressed = v != "0"
				case *elec.Switch:
					q.On = v != "0"
				}
			}
		}})
	c.add(&Command{ID: "electronics.run", Label: "Electronics: run the motion study", Group: "Assemble", Icon: icPlay,
		Run: func(string) { a.elecPlay(true) }})
	c.add(&Command{ID: "electronics.pause", Label: "Electronics: pause the motion study", Group: "Assemble",
		Run: func(string) { a.elecPlay(false) }})
	c.add(&Command{ID: "electronics.reset", Label: "Electronics: reset the motion study", Group: "Assemble",
		Run: func(string) { a.elecReset() }})
}

// elecCircuit is the circuit feature the window shows (the last one picked,
// else the design's first).
func (a *App) elecCircuit() *doc.Feature {
	if f := a.doc.D.Feature(a.elec.circuit); f != nil && f.Kind == doc.KCircuit {
		return f
	}
	for _, f := range a.doc.D.Features {
		if f.Kind == doc.KCircuit {
			a.elec.circuit = f.ID
			return f
		}
	}
	return nil
}

func (a *App) elecAdd() {
	a.elecReset()
	f := a.doc.D.Insert(doc.NewCircuit())
	a.doc.Commit()
	a.elec.circuit, a.elec.open = f.ID, true
	a.browserSel = bodyKey(f.ID, 0)
}

// elecEdit is called after the circuit changes: the study starts over.
func (a *App) elecEdit() {
	a.elecReset()
	a.doc.Commit()
}

func (a *App) elecLoadFirmware(path string) {
	f := a.elecCircuit()
	if f == nil {
		a.elecAdd()
		f = a.elecCircuit()
	}
	id := f.ID
	set := func(hex string) {
		f := a.doc.D.Feature(id)
		if f == nil || f.Kind != doc.KCircuit {
			return
		}
		f.Data, f.Circuit.File = hex, filepath.Base(path)
		a.elecEdit()
		if p, err := doc.Firmware(f); err == nil {
			a.notify("Loaded %s: %d bytes of program", filepath.Base(path), 2*len(trimProg(p)))
		}
	}
	if strings.EqualFold(filepath.Ext(path), ".ino") {
		if a.elec.compiling != "" {
			a.notify("Still compiling %s", filepath.Base(a.elec.compiling))
			return
		}
		a.elec.compiling = path
		build := func() {
			hex, err := compileSketch(path)
			a.post(func() {
				a.elec.compiling = ""
				if err != nil {
					a.notify("Compiling %s failed: %v", filepath.Base(path), err)
					return
				}
				set(hex)
			})
		}
		if a.capture != nil {
			build() // a capture waits for it
		} else {
			go build()
		}
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		a.notify("Load failed: %v", err)
		return
	}
	if _, err := avr.ParseHex(strings.NewReader(string(b))); err != nil {
		a.notify("%s is not Intel HEX: %v", filepath.Base(path), err)
		return
	}
	set(string(b))
}

// trimProg drops the erased flash past a program.
func trimProg(p []uint16) []uint16 {
	n := len(p)
	for n > 0 && p[n-1] == 0xFFFF {
		n--
	}
	return p[:n]
}

// elecAddPart adds a part of a model and returns its name ("" if none was
// added). Adding can bring more than the one part (an LED its resistor), so
// the part is known by its name, not by being last.
func (a *App) elecAddPart(model, name string) string {
	f := a.elecCircuit()
	if f == nil {
		a.elecAdd()
		f = a.elecCircuit()
	}
	if elec.Library[model] == nil {
		a.notify("No part %q in the library", model)
		return ""
	}
	p := f.Circuit.AddPart(model, name)
	a.elec.open = true
	a.elecEdit()
	if p == nil {
		return ""
	}
	return p.Name
}

// elecInsertModel puts a library part's 3D model (Arduino Uno, SG90, TT
// motor…) in the design as a component, clear of what is there.
func (a *App) elecInsertModel(id string) *doc.Feature {
	if id == "" {
		id = "uno"
	}
	f, err := doc.NewPartComponent(id)
	if err != nil {
		a.notify("%v", err)
		return nil
	}
	first := true
	for _, g := range a.doc.D.Features {
		first = first && g.Kind != doc.KComponent
	}
	if first {
		f.Inputs["Grounded"] = "1"
	}
	a.placeNew(f)
	if a.doc.D.Feature(f.ID) == nil {
		return nil
	}
	a.notify("Inserted the %s model as a component", doc.PartModels[id].Label)
	return f
}

// elecCommand is the wire / drive / set commands' "part.what=value".
func (a *App) elecCommand(arg, what string) {
	f := a.elecCircuit()
	if f == nil {
		a.notify("No circuit in the design")
		return
	}
	lhs, val, ok := strings.Cut(arg, "=")
	if !ok {
		a.notify("Expected part.%s=value", what)
		return
	}
	name, key := lhs, ""
	if what != "drive" {
		i := strings.LastIndex(lhs, ".")
		if i < 0 {
			a.notify("Expected part.name=value")
			return
		}
		name, key = lhs[:i], lhs[i+1:]
	}
	var cp *doc.CircuitPart
	for i := range f.Circuit.Parts {
		if f.Circuit.Parts[i].Name == name {
			cp = &f.Circuit.Parts[i]
		}
	}
	if cp == nil {
		a.notify("No part %q in the circuit", name)
		return
	}
	switch what {
	case "wire":
		// a role to a board pin, a driver's supply or motor, or a terminal
		// to another (part.terminal or a board pin; empty: unwired)
		m := elec.Library[cp.Model]
		switch {
		case key == "supply":
			f.Circuit.SetSupply(cp.Name, val)
		case key == "motor":
			f.Circuit.SetMotor(cp.Name, val)
		case m != nil && m.Role[key] != "":
			f.Circuit.SetBoardPin(cp.Name, m.Role[key], strings.ToUpper(val))
		case val == "":
			f.Circuit.SetBoardPin(cp.Name, key, "")
		case !strings.Contains(val, "."):
			f.Circuit.Connect(cp.Name+"."+key, elec.Board+"."+strings.ToUpper(val))
		default:
			f.Circuit.Connect(cp.Name+"."+key, val)
		}
	case "drive":
		parts := strings.Split(val, ",")
		cp.Joint = 0
		for _, j := range a.doc.D.Features {
			if j.Kind == doc.KJoint && j.Name == strings.TrimSpace(parts[0]) {
				cp.Joint = j.ID
			}
		}
		if cp.Joint == 0 && parts[0] != "" {
			a.notify("No joint %q", parts[0])
		}
		if len(parts) > 1 {
			cp.Ratio, _ = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		}
		if len(parts) > 2 {
			cp.Zero, _ = strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
		}
	case "set":
		v, err := strconv.ParseFloat(val, 64)
		if err != nil {
			if b, err2 := strconv.ParseBool(val); err2 == nil && b {
				v = 1
			} else if err2 != nil {
				a.notify("%s: not a number", val)
				return
			}
		}
		// a pot's or button's setting changes the running study, not the design
		if st := a.elec.study; st != nil && a.elecRuntime(st.Parts[name], key, v) {
			return
		}
		if cp.Params == nil {
			cp.Params = map[string]float64{}
		}
		cp.Params[key] = v
	}
	a.elecEdit()
}

// elecRuntime sets a control of a running part (a pot's Value, a button's
// Pressed).
func (a *App) elecRuntime(p elec.Part, key string, v float64) bool {
	switch q := p.(type) {
	case *elec.Pot:
		if key == "Value" {
			q.Value = v
			return true
		}
	case *elec.Button:
		if key == "Pressed" {
			q.Pressed = v != 0
			return true
		}
	case *elec.Switch:
		if key == "On" {
			q.On = v != 0
			return true
		}
	}
	return false
}

// elecStudy is the running study of the window's circuit, started if need be.
func (a *App) elecStudy() *doc.Study {
	f := a.elecCircuit()
	if st := a.elec.study; st != nil && (f == nil || st.F != f || a.doc.D.Feature(f.ID) != st.F) {
		a.elec.study, a.elec.playing = nil, false // the design was replaced (undo, open)
	}
	if a.elec.study == nil && f != nil && f.Err == "" {
		st, err := a.doc.D.NewStudy(f)
		if err != nil {
			a.notify("Motion study: %v", err)
			return nil
		}
		a.elec.study = st
	}
	return a.elec.study
}

func (a *App) elecPlay(on bool) {
	if on && a.elecStudy() == nil {
		return
	}
	if on && !a.elec.playing {
		a.stopAnimation()
	}
	a.elec.playing, a.elec.last = on, time.Now()
}

// elecReset ends the study, putting the joints back.
func (a *App) elecReset() {
	st := a.elec.study
	a.elec.study, a.elec.playing = nil, false
	if st != nil && a.doc.D.Feature(st.F.ID) == st.F {
		st.Restore(a.doc.D)
		a.doc.Regenerate()
	}
}

// elecTick runs the study for the frame's time and poses the joints.
func (a *App) elecTick() {
	if !a.elec.playing {
		return
	}
	st := a.elecStudy()
	if st == nil {
		a.elec.playing = false
		return
	}
	now := time.Now()
	dt := math.Min(now.Sub(a.elec.last).Seconds(), 0.05)
	if a.capture != nil {
		dt = 1.0 / 30
	}
	a.elec.last = now
	if a.elec.speed == 0 {
		a.elec.speed = 1
	}
	st.Run(dt * float64(a.elec.speed))
	if st.Pose(a.doc.D) {
		a.doc.Regenerate()
	}
}

var unoPins = func() []string {
	p := []string{""}
	for i := 0; i <= 13; i++ {
		p = append(p, fmt.Sprintf("D%d", i))
	}
	for i := 0; i <= 5; i++ {
		p = append(p, fmt.Sprintf("A%d", i))
	}
	return p
}()

// circuitProperties is the circuit's summary in the Properties panel.
func (a *App) circuitProperties(f *doc.Feature) bool {
	if f.Kind != doc.KCircuit {
		return false
	}
	if f.Warn != "" {
		imgui.TextWrapped(f.Warn)
	}
	c := f.Circuit
	if c != nil {
		imgui.Text("Arduino Uno · " + map[bool]string{true: c.File, false: "no firmware"}[c.File != ""])
		for _, p := range c.Parts {
			imgui.BulletText(a.partLine(c, p))
		}
	}
	if imgui.Button(ui.Icon(icCircuit, "Open Electronics")) {
		a.elec.circuit, a.elec.open = f.ID, true
	}
	return true
}

func (a *App) partLine(c *doc.Circuit, p doc.CircuitPart) string {
	s := p.Name
	var pins []string
	if m := elec.Library[p.Model]; m != nil {
		for _, r := range m.Pins {
			if v := c.BoardPin(p.Name, m.Role[r]); v != "" {
				pins = append(pins, v)
			}
		}
	}
	if len(pins) > 0 {
		s += " on " + strings.Join(pins, ", ")
	}
	if j := a.doc.D.Feature(p.Joint); j != nil && p.Joint != 0 {
		s += " → " + j.Name
	}
	return s
}

// elecWindow is the Electronics window.
func (a *App) elecWindow() {
	if !a.elec.open {
		return
	}
	// beside the viewport's right edge, over the Properties panel
	vp := imgui.MainViewport()
	imgui.SetNextWindowPosV(imgui.Vec2{X: vp.Pos().X + vp.Size().X - 420, Y: vp.Pos().Y + 104}, imgui.CondAppearing, imgui.Vec2{})
	imgui.SetNextWindowSizeV(imgui.Vec2{X: 410, Y: min(640, vp.Size().Y-140)}, imgui.CondAppearing)
	open := true
	if imgui.BeginV(ui.Icon(icCircuit, "Electronics")+"###electronics", &open, imgui.WindowFlagsNoSavedSettings) {
		a.elecBody()
	}
	imgui.End()
	if !open {
		a.elec.open = false
	}
}

func (a *App) elecBody() {
	f := a.elecCircuit()
	if f == nil {
		imgui.TextWrapped("A circuit is an Arduino Uno running a sketch, with servos, motors, buttons and pots wired to it. Its servos and motors can drive the joints of an assembly.")
		if imgui.Button("Add a circuit") {
			a.elecAdd()
		}
		imgui.SameLine()
		if imgui.Button("Open a Wokwi project…") {
			a.cmds.Run("electronics.wokwi", "")
		}
		imgui.SetItemTooltip("A circuit made on wokwi.com: its diagram.json, wired as it was, and its sketch")
		return
	}
	c := f.Circuit
	// several circuits: pick one
	var circuits []string
	for _, g := range a.doc.D.Features {
		if g.Kind == doc.KCircuit {
			circuits = append(circuits, g.Name)
		}
	}
	if len(circuits) > 1 {
		if n, ok := a.combo("Circuit##circ", f.Name, circuits, nil); ok {
			for _, g := range a.doc.D.Features {
				if g.Kind == doc.KCircuit && g.Name == n {
					a.elecReset()
					a.elec.circuit = g.ID
				}
			}
			return
		}
	}
	if f.Err != "" {
		imgui.PushStyleColorVec4(imgui.ColText, colErr)
		imgui.TextWrapped(f.Err)
		imgui.PopStyleColor()
	}

	imgui.SeparatorText("Board")
	imgui.Text("Arduino Uno (ATmega328P, 16 MHz)")
	imgui.SameLine()
	if imgui.SmallButton("3D model##uno") {
		a.elecInsertModel("uno")
	}
	imgui.SetItemTooltip("Insert the board's outline, holes and connectors as a component")
	fw := "no firmware"
	if p, err := doc.Firmware(f); err == nil {
		fw = fmt.Sprintf("%s · %d bytes", c.File, 2*len(trimProg(p)))
	}
	if a.elec.compiling != "" {
		fw = "compiling " + filepath.Base(a.elec.compiling) + "…"
	}
	imgui.TextDisabled(fw)
	if imgui.Button("Load firmware…") {
		a.cmds.Run("electronics.firmware", "")
	}
	imgui.SetItemTooltip("A compiled sketch (.hex), or a sketch (.ino) compiled with arduino-cli if it is installed")
	imgui.SameLine()
	if imgui.Button(ui.Icon(icCode, "Write code…")) {
		a.codeOpen()
	}
	a.markItem("btn:electronics.code")
	imgui.SetItemTooltip("Write the sketch here (or in VS Code), compile it and load it")
	a.cliBanner()

	a.elecStudyUI(f)

	imgui.SeparatorText("Parts")
	if imgui.Button(ui.Icon(icWiring, "Wiring sheet")) {
		a.cmds.Run("electronics.sheet", "1")
	}
	a.markItem("btn:electronics.sheet")
	imgui.SetItemTooltip("Draw the wires from pin to pin, part to part, and see the checks")
	imgui.SameLine()
	if imgui.Button("Wokwi…") {
		a.cmds.Run("electronics.wokwi", "")
	}
	imgui.SetItemTooltip("Open a Wokwi project as a new circuit: its diagram.json, wired as it was, and its sketch")
	if n := len(c.Issues()); n > 0 {
		imgui.SameLine()
		imgui.TextColored(colWarn, fmt.Sprintf("%d to look at", n))
	}
	if a.wiringTable(f) {
		return
	}
	changed := false
	for i := range c.Parts {
		imgui.PushIDInt(int32(i))
		del, ch := a.partEditor(f, &c.Parts[i])
		changed = changed || ch
		imgui.PopID()
		if del {
			c.RemovePart(c.Parts[i].Name)
			a.elecEdit()
			return
		}
	}
	if changed {
		a.elecReset()
		a.doc.Regenerate()
	}
	ids := elec.Models()
	if a.elec.addModel == "" {
		a.elec.addModel = "sg90"
	}
	imgui.SetNextItemWidth(200)
	if imgui.BeginCombo("##addpart", elec.Library[a.elec.addModel].Label) {
		for _, id := range ids {
			m := elec.Library[id]
			if imgui.SelectableBoolV(m.Label+"##"+id, id == a.elec.addModel, 0, imgui.Vec2{}) {
				a.elec.addModel = id
			}
			imgui.SetItemTooltip(m.About)
		}
		imgui.EndCombo()
	}
	imgui.SameLine()
	if imgui.Button("Add part") {
		name := a.elecAddPart(a.elec.addModel, "")
		if name != "" && !a.elec.noModel && doc.PartModels[a.elec.addModel] != nil {
			if m := a.elecInsertModel(a.elec.addModel); m != nil {
				if f := a.elecCircuit(); f != nil {
					for i := range f.Circuit.Parts {
						if f.Circuit.Parts[i].Name == name {
							f.Circuit.Parts[i].Model3D = m.ID
						}
					}
					a.doc.Commit()
				}
			}
		}
	}
	model := !a.elec.noModel
	if imgui.Checkbox("with its 3D model##addmodel", &model) {
		a.elec.noModel = !model
	}
	imgui.SetItemTooltip("Put the part's 3D model in the design as a component too (each part's entry can insert it later)")
}

// partEditor edits one part: wiring, what it drives, its figures. It says
// whether to delete it and whether it changed.
// partWires lists a part's terminals, each with the terminals it is wired
// to (× takes a wire off) and a menu wiring it to any other terminal of the
// circuit: another part's as well as the board's. It says whether the
// wiring changed.
func (a *App) partWires(c *doc.Circuit, p *doc.CircuitPart, m *elec.Model) (edited bool) {
	n := 0
	for _, w := range c.Wires {
		pa, _, _ := elec.Split(w.A)
		pb, _, _ := elec.Split(w.B)
		if pa == p.Name || pb == p.Name {
			n++
		}
	}
	open := imgui.TreeNodeExStrV(fmt.Sprintf("Wires (%d)###wires", n), 0)
	a.markItem("wires:" + p.Name)
	if !open {
		imgui.SetItemTooltip("Wire this part's terminals to other parts or to the board (or drag between terminals on the wiring sheet)")
		return false
	}
	defer imgui.TreePop()
	var all []string // every other terminal: the other parts', then the board's
	for _, q := range c.Parts {
		if q.Name != p.Name {
			for _, t := range elec.Terms(q.Model) {
				all = append(all, q.Name+"."+t)
			}
		}
	}
	for _, t := range boardOrder {
		all = append(all, elec.Board+"."+t)
	}
	for _, t := range m.Terms {
		ref := p.Name + "." + t
		imgui.PushIDStr(t)
		imgui.AlignTextToFramePadding()
		imgui.Text(t)
		wired := map[string]bool{}
		for _, w := range c.Wires {
			if !w.Has(ref) {
				continue
			}
			o := w.Other(ref)
			wired[o] = true
			imgui.SameLine()
			if imgui.SmallButton(o + " ×##" + o) {
				c.Disconnect(ref, o)
				edited = true
			}
			imgui.SetItemTooltip("Take the wire " + ref + " – " + o + " off")
		}
		imgui.SameLine()
		imgui.SetNextItemWidth(130)
		if imgui.BeginComboV("##to", "wire to…", imgui.ComboFlagsHeightLarge) {
			for _, o := range all {
				if wired[o] {
					continue
				}
				if imgui.SelectableBool(o) {
					c.Connect(ref, o)
					edited = true
				}
				a.markItem("wireto:" + ref + ">" + o)
			}
			imgui.EndCombo()
		}
		a.markItem("wireto:" + ref)
		imgui.PopID()
		if edited {
			return true
		}
	}
	return false
}

func (a *App) partEditor(f *doc.Feature, p *doc.CircuitPart) (del, changed bool) {
	m := elec.Library[p.Model]
	open := imgui.TreeNodeExStrV(a.partLine(f.Circuit, *p)+"###part", 0)
	a.markItem("part:" + p.Name)
	if m != nil {
		imgui.SetItemTooltip(m.Label + ": " + m.About)
	}
	if !open {
		return false, false
	}
	defer imgui.TreePop()
	if m == nil {
		imgui.TextColored(colErr, "not in the library")
		return imgui.SmallButton("Remove"), false
	}
	defer func() {
		if doc.PartModels[p.Model] != nil {
			if imgui.SmallButton("Insert 3D model") {
				if m := a.elecInsertModel(p.Model); m != nil {
					p.Model3D = m.ID
					a.doc.Commit()
				}
			}
			imgui.SameLine()
		}
		if imgui.SmallButton("Remove " + m.Label) {
			del = true
		}
	}()
	name := p.Name
	imgui.SetNextItemWidth(160)
	imgui.InputTextWithHint("Name##n", "name", &name, 0, nil)
	if name != p.Name && name != "" {
		f.Circuit.RenamePart(p.Name, name)
		changed = true
	}
	if imgui.IsItemDeactivatedAfterEdit() {
		a.doc.Commit()
	}
	for _, role := range m.Pins {
		opts := unoPins
		if m.Analog {
			opts = []string{"", "A0", "A1", "A2", "A3", "A4", "A5"}
		}
		cur := f.Circuit.BoardPin(p.Name, m.Role[role])
		if v, ok := a.combo(strings.ToUpper(role[:1])+role[1:]+"##pin"+role, cur, opts, nil); ok {
			f.Circuit.SetBoardPin(p.Name, m.Role[role], v)
			a.elecEdit()
		}
	}
	if a.partWires(f.Circuit, p, m) {
		a.elecEdit()
		return false, false
	}
	others := func(kind string) []string {
		out := []string{""}
		for _, q := range f.Circuit.Parts {
			if k := elec.Library[q.Model]; k != nil && k.Kind == kind {
				out = append(out, q.Name)
			}
		}
		return out
	}
	for _, need := range m.Needs {
		switch need {
		case "supply":
			if v, ok := a.combo("Supply##sup", f.Circuit.Linked(p.Name, "VM", "battery", "+"), others("battery"), nil); ok {
				f.Circuit.SetSupply(p.Name, v)
				a.elecEdit()
			}
		case "motor":
			cur := f.Circuit.Linked(p.Name, "OUT1", "motor", "1") + f.Circuit.Linked(p.Name, "OUT1", "motor", "2")
			if v, ok := a.combo("Motor##mot", cur, others("motor"), nil); ok {
				f.Circuit.SetMotor(p.Name, v)
				a.elecEdit()
			}
		}
	}
	if m.Drives {
		joints := []string{"(none)"}
		byName := map[string]int{}
		for _, j := range a.doc.D.Features {
			if j.Kind == doc.KJoint && doc.MotionInput(j) != "" {
				joints = append(joints, j.Name)
				byName[j.Name] = j.ID
			}
		}
		cur := "(none)"
		if j := a.doc.D.Feature(p.Joint); j != nil && p.Joint != 0 {
			cur = j.Name
		}
		if v, ok := a.combo("Drives##joint", cur, joints, nil); ok {
			p.Joint = byName[v]
			a.elecEdit()
		}
		if p.Joint != 0 {
			r := p.Ratio
			if r == 0 {
				r = 1
			}
			unit := "° / °"
			if j := a.doc.D.Feature(p.Joint); j != nil && doc.MotionInput(j) == "Distance" {
				unit = "mm / °"
			}
			if a.numField("Ratio##ratio", &r, unit) {
				p.Ratio, changed = r, true
			}
			if a.numField("Zero at##zero", &p.Zero, "°") {
				changed = true
			}
		}
	}
	if len(m.Params) > 0 && imgui.TreeNodeExStrV("Figures##fig", 0) {
		for _, q := range m.Params {
			v := q.Default
			if x, ok := p.Params[q.Name]; ok {
				v = x
			}
			if a.numField(q.Name+"##"+q.Name, &v, q.Unit) {
				if p.Params == nil {
					p.Params = map[string]float64{}
				}
				p.Params[q.Name], changed = v, true
			}
		}
		imgui.TreePop()
	}
	return false, changed
}

// numField edits a number, committing when done; it says whether it changed.
func (a *App) numField(label string, v *float64, unit string) bool {
	imgui.SetNextItemWidth(110)
	ch := imgui.InputDoubleV(label, v, 0, 0, "%g", imgui.InputTextFlagsAutoSelectAll)
	if imgui.IsItemDeactivatedAfterEdit() {
		a.elecEdit()
	}
	if unit != "" {
		imgui.SameLine()
		imgui.TextDisabled(unit)
	}
	return ch
}

// elecStudyUI is the motion study: play, the clock, each part as it is,
// the controls and the serial monitor.
func (a *App) elecStudyUI(f *doc.Feature) {
	imgui.SeparatorText("Motion study")
	st := a.elec.study
	if st != nil && st.F != f {
		st = nil
	}
	ready := f.Err == "" && f.Data != ""
	imgui.BeginDisabledV(!ready)
	label := ui.Icon(icPlay, "Run")
	if a.elec.playing {
		label = ui.Icon(icPause, "Pause")
	}
	if imgui.Button(label + "##run") {
		a.elecPlay(!a.elec.playing)
	}
	imgui.EndDisabled()
	imgui.SameLine()
	imgui.BeginDisabledV(st == nil)
	if imgui.Button("Reset##reset") {
		a.elecReset()
	}
	imgui.EndDisabled()
	imgui.SameLine()
	if a.elec.speed == 0 {
		a.elec.speed = 1
	}
	imgui.SetNextItemWidth(110)
	imgui.SliderFloatV("##speed", &a.elec.speed, 0.1, 2, "%.1f× speed", 0)
	if st == nil {
		if !ready {
			imgui.TextDisabled("Load firmware to run it.")
		} else {
			imgui.TextDisabled("Runs the sketch on the emulated board; the parts drive their joints.")
		}
		return
	}
	imgui.Text(fmt.Sprintf("t = %.2f s", st.Sim.T))
	names := make([]string, 0, len(st.Parts))
	for n := range st.Parts {
		names = append(names, n)
	}
	sort.SliceStable(names, func(i, j int) bool { return partOrder(f, names[i]) < partOrder(f, names[j]) })
	if imgui.BeginTableV("##parts", 2, imgui.TableFlagsSizingStretchProp, imgui.Vec2{}, 0) {
		for _, n := range names {
			p := st.Parts[n]
			imgui.TableNextRow()
			imgui.TableNextColumn()
			imgui.TextUnformatted(n)
			imgui.TableNextColumn()
			imgui.PushIDStr(n)
			switch q := p.(type) {
			case *elec.Pot:
				v := float32(q.Value)
				imgui.SetNextItemWidth(-1)
				if imgui.SliderFloatV("##pot", &v, 0, 1, "%.2f of its turn", 0) {
					q.Value = float64(v)
				}
			case *elec.Button:
				imgui.Checkbox("pressed", &q.Pressed)
			case *elec.Switch:
				imgui.Checkbox("on", &q.On)
			case *elec.LED:
				b := float32(ledLook(q.Brightness))
				imgui.ColorButtonV("##led", imgui.Vec4{X: 0.25 + 0.75*b, Y: 0.12 + 0.2*b, Z: 0.1, W: 1}, imgui.ColorEditFlagsNoTooltip, imgui.Vec2{X: 14, Y: 14})
				imgui.SameLine()
				imgui.TextUnformatted(stateOf(p))
			default:
				imgui.TextUnformatted(stateOf(p))
			}
			imgui.PopID()
		}
		imgui.EndTable()
	}
	if len(st.Sim.Serial) > 0 {
		imgui.SeparatorText("Serial")
		lines := strings.Split(strings.TrimRight(strings.ReplaceAll(string(st.Sim.Serial), "\r", ""), "\n"), "\n")
		if len(lines) > 200 {
			lines = lines[len(lines)-200:]
		}
		if imgui.BeginChildStrV("##serial", imgui.Vec2{X: 0, Y: 110}, imgui.ChildFlagsBorders, 0) {
			for _, l := range lines {
				imgui.TextUnformatted(l)
			}
			if a.elec.playing {
				imgui.SetScrollHereYV(1)
			}
		}
		imgui.EndChild()
	}
}

func partOrder(f *doc.Feature, name string) int {
	for i, p := range f.Circuit.Parts {
		if p.Name == name {
			return i
		}
	}
	return 1 << 20
}

func stateOf(p elec.Part) string {
	if s, ok := p.(interface{ State() string }); ok {
		return s.State()
	}
	return ""
}

// wiringTable shows every part's pins and where each goes on the board,
// all at once (each part's entry below has the rest of it). It says
// whether it changed the circuit.
func (a *App) wiringTable(f *doc.Feature) bool {
	c := f.Circuit
	rows := 0
	for _, p := range c.Parts {
		if m := elec.Library[p.Model]; m != nil {
			rows += len(m.Pins)
		}
	}
	if rows == 0 {
		return false
	}
	imgui.TextDisabled("Wiring: each part's pin to a pin of the Uno")
	changed := false
	if imgui.BeginTableV("##wiring", 3, imgui.TableFlagsRowBg|imgui.TableFlagsBordersInnerV|imgui.TableFlagsSizingStretchProp, imgui.Vec2{}, 0) {
		imgui.TableSetupColumn("Part")
		imgui.TableSetupColumn("Pin")
		imgui.TableSetupColumn("Board pin")
		imgui.TableHeadersRow()
		for i := range c.Parts {
			p := &c.Parts[i]
			m := elec.Library[p.Model]
			if m == nil {
				continue
			}
			for k, role := range m.Pins {
				imgui.TableNextRow()
				imgui.TableNextColumn()
				if k == 0 {
					imgui.TextUnformatted(p.Name)
				}
				imgui.TableNextColumn()
				imgui.TextDisabled(role)
				imgui.TableNextColumn()
				opts := unoPins
				if m.Analog {
					opts = []string{"", "A0", "A1", "A2", "A3", "A4", "A5"}
				}
				// pins another part has taken are marked
				taken := map[string]string{}
				for _, q := range c.Parts {
					qm := elec.Library[q.Model]
					if q.Name == p.Name || qm == nil {
						continue
					}
					for _, t := range qm.Terms {
						if v := c.BoardPin(q.Name, t); v != "" {
							taken[v] = "on " + q.Name
						}
					}
				}
				// a terminal wired to other parts, not to a pin (an LED's
				// anode through its resistor), is drawn on the wiring sheet
				ref := p.Name + "." + m.Role[role]
				if c.BoardPin(p.Name, m.Role[role]) == "" {
					if net := c.Net(ref); len(net) > 1 {
						var via []string
						for _, r := range net {
							if q, _, _ := elec.Split(r); q != p.Name && !slices.Contains(via, q) {
								via = append(via, q)
							}
						}
						imgui.TextDisabled("through " + strings.Join(via, ", "))
						continue
					}
				}
				imgui.PushIDStr(p.Name + "." + role)
				if v, ok := a.combo("##wire", c.BoardPin(p.Name, m.Role[role]), opts, taken); ok {
					c.SetBoardPin(p.Name, m.Role[role], v)
					changed = true
				}
				a.markItem("wire:" + p.Name + "." + role)
				imgui.PopID()
			}
		}
		imgui.EndTable()
	}
	if changed {
		a.elecEdit()
	}
	return changed
}

// elecConnect wires two terminals ("a=b", a colour after a second "=")
// or, with on false, takes the wire between them off.
func (a *App) elecConnect(arg string, on bool) {
	f := a.elecCircuit()
	parts := strings.Split(arg, "=")
	if f == nil || len(parts) < 2 {
		return
	}
	c := f.Circuit
	x, y := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	if !on {
		c.Disconnect(x, y)
		a.elecEdit()
		return
	}
	c.Connect(x, y)
	if len(parts) > 2 {
		for i := range c.Wires {
			if c.Wires[i].Has(x) && c.Wires[i].Other(x) == y {
				c.Wires[i].Colour = strings.TrimSpace(parts[2])
			}
		}
	}
	a.elecEdit()
}

// elecPlace puts a part (or the board, "uno") on the wiring sheet: "name=x,y".
func (a *App) elecPlace(arg string) {
	f := a.elecCircuit()
	name, xy, _ := strings.Cut(arg, "=")
	n := floats(xy)
	if f == nil || len(n) != 2 {
		return
	}
	p := geom.V2(n[0], n[1])
	if name == elec.Board {
		f.Circuit.BoardAt = &p
	}
	for i := range f.Circuit.Parts {
		if f.Circuit.Parts[i].Name == name {
			f.Circuit.Parts[i].At = &p
		}
	}
	a.doc.Commit()
}

// partOfModel is the running study's part whose 3D model is step fid.
func (a *App) partOfModel(fid int) elec.Part {
	st := a.elec.study
	f := a.elecCircuit()
	if st == nil || f == nil || fid == 0 {
		return nil
	}
	for _, p := range f.Circuit.Parts {
		if p.Model3D == fid {
			return st.Parts[p.Name]
		}
	}
	return nil
}

// pressPart: a press on a button's 3D model while the study runs holds the
// button down until the mouse is let go.
func (a *App) pressPart() bool {
	if a.sk != nil || a.hover.IsZero() {
		return false
	}
	b := a.doc.Body(a.hover.Item)
	if b == nil {
		return false
	}
	switch q := a.partOfModel(b.F.ID).(type) {
	case *elec.Button:
		q.Pressed = true
		a.vp.pressing = q
		return true
	case *elec.Switch:
		q.On = !q.On
		return true
	}
	return false
}

// ledGlow is how bright the LED whose model is step fid is lit (0 when it
// isn't one, or no study runs).
func (a *App) ledGlow(fid int) float64 {
	if q, ok := a.partOfModel(fid).(*elec.LED); ok {
		return ledLook(q.Brightness)
	}
	return 0
}

// ledLook is how lit an LED looks at a brightness (its current over the
// 20 mA it is fully lit at): the eye takes light on a curve, so a few
// milliamps, plainly lit on a real LED, look so here too rather than barely
// tinted.
func ledLook(b float64) float64 {
	if b <= 0.002 {
		return 0
	}
	return math.Pow(math.Min(1, b), 0.45)
}
