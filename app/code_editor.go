package app

// The sketch's code (Arduino C++) written in Forma: a code editor (ImGui
// Color Text Edit: highlighting, undo, find, line numbers) whose text is
// kept with the circuit, compiled with arduino-cli into the firmware the
// motion study runs, its errors marked on their lines. Open in VS Code
// writes the code to an .ino and opens it there; saving it there brings
// it back.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	cte "github.com/AllenDang/cimgui-go/ImGuiColorTextEdit"
	imgui "github.com/AllenDang/cimgui-go/imgui"

	"forma/arduinocli"
	"forma/doc"
	"forma/ui"
)

type codeEditor struct {
	open      bool
	ed        *cte.TextEditor
	circuit   int    // the circuit whose code it shows
	undo      uint64 // the editor's undo index when its text was last kept
	file      string // an .ino shared with an outside editor
	fileTime  time.Time
	checked   time.Time
	msgs      []compileMsg
	status    string
	compiling bool
	// arduino-cli being installed: what it is doing, how far (-1: can't
	// tell), and how it ended
	setup      bool
	setupStage string
	setupFrac  float64
	setupErr   string
}

// sketchExamples are code to start from, by name.
var sketchExamples = map[string]string{
	"Blink": `// Blink the LED on pin 13 (wire an LED part to D13)
void setup() {
  pinMode(13, OUTPUT);
}

void loop() {
  digitalWrite(13, HIGH);
  delay(500);
  digitalWrite(13, LOW);
  delay(500);
}
`,
	"Servo sweep": `// Sweep a servo on pin 9 between 20 and 160 degrees
#include <Servo.h>

Servo arm;

void setup() {
  arm.attach(9);
}

void loop() {
  for (int a = 20; a <= 160; a++) {
    arm.write(a);
    delay(15);
  }
  for (int a = 160; a >= 20; a--) {
    arm.write(a);
    delay(15);
  }
}
`,
	"Button and LED": `// The LED on pin 13 lights while the button on pin 2 is pressed
void setup() {
  pinMode(2, INPUT_PULLUP);
  pinMode(13, OUTPUT);
}

void loop() {
  digitalWrite(13, digitalRead(2) == LOW ? HIGH : LOW);
}
`,
	"Motor speed from a pot": `// A gear motor on a TB6612 (PWM on 6, direction on 7 and 8) turning
// as fast as the pot on A0 says
void setup() {
  pinMode(6, OUTPUT);
  pinMode(7, OUTPUT);
  pinMode(8, OUTPUT);
}

void loop() {
  int speed = analogRead(A0) / 4;
  digitalWrite(7, HIGH);
  digitalWrite(8, LOW);
  analogWrite(6, speed);
}
`,
}

func (a *App) registerCodeCommands(c *Commands) {
	c.add(&Command{ID: "electronics.code", Label: "Electronics: write the sketch's code", Group: "Assemble", Icon: icCode,
		Run: func(string) { a.codeOpen() }})
	c.add(&Command{ID: "electronics.example", Label: "Electronics: start the code from an example", Group: "Assemble",
		Run: func(arg string) { a.codeExample(arg) }})
	c.add(&Command{ID: "electronics.compile", Label: "Electronics: compile the code and load it", Group: "Assemble",
		Run: func(arg string) { a.codeCompile(arg == "now" || a.capture != nil) }})
	c.add(&Command{ID: "electronics.installcli", Label: "Electronics: install arduino-cli and the Uno's compiler", Group: "Assemble",
		Run: func(string) { a.installArduinoCLI() }})
	c.add(&Command{ID: "electronics.vscode", Label: "Electronics: open the code in VS Code", Group: "Assemble",
		Enabled: func() bool { return vsCode() != "" }, Run: func(string) { a.codeInVSCode() }})
}

const icCode = 0xf121 // code

// codeCircuit is the circuit the editor shows (the window's), made if need be.
func (a *App) codeCircuit() *doc.Feature {
	f := a.elecCircuit()
	if f == nil {
		a.elecAdd()
		f = a.elecCircuit()
	}
	return f
}

func (a *App) codeOpen() {
	ce := &a.code
	f := a.codeCircuit()
	if ce.ed == nil {
		ce.ed = cte.NewTextEditor()
		ce.ed.SetLanguage(cte.LanguageCpp())
		ce.ed.SetPalette(cte.TextEditorGetDarkPalette())
		ce.ed.SetTabSize(2)
		ce.ed.SetInsertSpacesOnTabs(true)
		ce.ed.SetAutoIndentEnabled(true)
		ce.ed.SetShowLineNumbersEnabled(true)
		ce.ed.SetShowWhitespacesEnabled(false)
	}
	if ce.circuit != f.ID {
		src := f.Circuit.Source
		if src == "" {
			src = sketchExamples["Blink"]
			f.Circuit.Source = src
		}
		ce.ed.SetText(src)
		ce.undo = ce.ed.UndoIndex()
		ce.circuit, ce.file, ce.msgs, ce.status = f.ID, "", nil, ""
	}
	ce.open = true
}

func (a *App) codeExample(name string) {
	src, ok := sketchExamples[name]
	if !ok {
		a.notify("No example %q", name)
		return
	}
	a.codeOpen()
	a.code.ed.SetText(src)
	a.codeKeep()
}

// codeKeep puts the editor's text into the circuit (and the shared file).
func (a *App) codeKeep() {
	ce := &a.code
	f := a.doc.D.Feature(ce.circuit)
	if f == nil || f.Kind != doc.KCircuit || ce.ed == nil {
		return
	}
	ce.undo = ce.ed.UndoIndex()
	src := ce.ed.Text()
	if src == f.Circuit.Source {
		return
	}
	f.Circuit.Source = src
	if ce.file != "" {
		if err := os.WriteFile(ce.file, []byte(src), 0o644); err == nil {
			if st, err := os.Stat(ce.file); err == nil {
				ce.fileTime = st.ModTime()
			}
		}
	}
}

// codeCompile compiles the code and, if it builds, loads it as the
// circuit's firmware.
func (a *App) codeCompile(now bool) {
	ce := &a.code
	if ce.compiling {
		return
	}
	a.codeOpen()
	a.codeKeep()
	f := a.doc.D.Feature(ce.circuit)
	if f == nil {
		return
	}
	id, name, src := f.ID, sketchName(f.Name), f.Circuit.Source
	ce.compiling, ce.status = true, "Compiling…"
	done := func(hex string, msgs []compileMsg, err error) {
		ce.compiling, ce.msgs = false, msgs
		ce.ed.ClearMarkers()
		for _, m := range msgs {
			col := imgui.ColorConvertFloat4ToU32(imgui.Vec4{X: 0.95, Y: 0.75, Z: 0.2, W: 1})
			if m.Error {
				col = imgui.ColorConvertFloat4ToU32(imgui.Vec4{X: 0.95, Y: 0.3, Z: 0.25, W: 1})
			}
			ce.ed.AddMarker(uint64(max(0, m.Line-1)), col, col&0x40ffffff, m.Text, m.Text)
		}
		if err != nil {
			ce.status = "Didn't compile: " + err.Error()
			a.notify("%s", ce.status)
			return
		}
		f := a.doc.D.Feature(id)
		if f == nil || f.Kind != doc.KCircuit {
			return
		}
		f.Data, f.Circuit.File = hex, name+".ino"
		a.elecEdit()
		size := 0
		if p, err := doc.Firmware(f); err == nil {
			size = 2 * len(trimProg(p))
		}
		ce.status = fmt.Sprintf("Compiled and loaded: %d bytes of program", size)
		a.notify("%s", ce.status)
	}
	build := func() {
		hex, msgs, err := compileCode(name, src)
		a.post(func() { done(hex, msgs, err) })
	}
	if now {
		hex, msgs, err := compileCode(name, src)
		done(hex, msgs, err)
		return
	}
	go build()
}

// vsCode finds Visual Studio Code's command.
func vsCode() string {
	if p, err := exec.LookPath("code"); err == nil {
		return p
	}
	for _, p := range []string{
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Microsoft VS Code", "bin", "code.cmd"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft VS Code", "bin", "code.cmd"),
		"/usr/local/bin/code", "/usr/bin/code",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// codeInVSCode writes the code to an .ino (in Documents/Forma/sketches,
// a folder of its own name, as arduino-cli and the Arduino extension want)
// and opens it in VS Code; saving it there comes back here.
func (a *App) codeInVSCode() {
	ce := &a.code
	a.codeOpen()
	f := a.doc.D.Feature(ce.circuit)
	if f == nil {
		return
	}
	if ce.file == "" {
		name := sketchName(f.Name)
		dir := defaultPath(filepath.Join("sketches", name))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			a.notify("Can't write the sketch: %v", err)
			return
		}
		ce.file = filepath.Join(dir, name+".ino")
	}
	if err := os.WriteFile(ce.file, []byte(f.Circuit.Source), 0o644); err != nil {
		a.notify("Can't write the sketch: %v", err)
		return
	}
	if st, err := os.Stat(ce.file); err == nil {
		ce.fileTime = st.ModTime()
	}
	code := vsCode()
	if code == "" {
		a.notify("VS Code isn't installed: the sketch is at %s", ce.file)
		return
	}
	if a.capture != nil {
		a.notify("Would open %s in VS Code", ce.file)
		return
	}
	cmd := exec.Command(code, filepath.Dir(ce.file), ce.file)
	hideConsole(cmd)
	if err := cmd.Start(); err != nil {
		a.notify("VS Code: %v", err)
		return
	}
	go cmd.Wait()
	a.notify("Opened %s in VS Code: saving it there updates the sketch here", filepath.Base(ce.file))
}

// codeWatch brings back what an outside editor saved.
func (a *App) codeWatch() {
	ce := &a.code
	if ce.file == "" || time.Since(ce.checked) < 500*time.Millisecond {
		return
	}
	ce.checked = time.Now()
	st, err := os.Stat(ce.file)
	if err != nil || !st.ModTime().After(ce.fileTime) {
		return
	}
	ce.fileTime = st.ModTime()
	b, err := os.ReadFile(ce.file)
	if err != nil || ce.ed == nil || string(b) == ce.ed.Text() {
		return
	}
	ce.ed.SetText(string(b))
	a.codeKeep()
	a.notify("%s changed outside: the code here follows", filepath.Base(ce.file))
}

// installArduinoCLI downloads arduino-cli if it is missing and installs
// the Uno's core and the Servo library through it, on a goroutine.
func (a *App) installArduinoCLI() {
	ce := &a.code
	if ce.setup {
		return
	}
	ce.setup, ce.setupStage, ce.setupFrac, ce.setupErr = true, "starting", -1, ""
	go func() {
		exe, err := arduinocli.Installer{}.Setup(context.Background(), func(stage string, frac float64) {
			a.post(func() { ce.setupStage, ce.setupFrac = stage, frac })
		})
		a.post(func() {
			ce.setup = false
			if err != nil {
				ce.setupErr = err.Error()
				a.notify("Setting up arduino-cli failed: %v", err)
				return
			}
			a.notify("arduino-cli is ready (%s): sketches can be compiled", exe)
		})
	}()
}

// cliBanner offers to install arduino-cli where it is missing (or the
// Uno's compiler, when a compile said it isn't there), and shows the
// install as it goes.
func (a *App) cliBanner() {
	ce := &a.code
	if ce.setup {
		frac := float32(ce.setupFrac)
		label := ce.setupStage
		if frac < 0 {
			frac = float32(-1 * imgui.Time())
		}
		imgui.ProgressBarV(frac, imgui.Vec2{X: -1, Y: 0}, label)
		return
	}
	missing := arduinoCLI() == ""
	noCore := strings.Contains(ce.status, "arduino:avr") || strings.Contains(ce.status, "platform not installed") || strings.Contains(ce.status, "Servo.h")
	if !missing && !noCore {
		return
	}
	msg := "arduino-cli, which compiles sketches, isn't installed."
	btn := "Install arduino-cli"
	tip := "Downloads arduino-cli from arduino.cc (about 20 MB), then the Uno's compiler and the Servo library through it (about 150 MB)"
	if !missing {
		msg = "The Uno's compiler (the arduino:avr core) or a library is missing."
		btn = "Install the Uno's compiler"
		tip = "Runs arduino-cli core install arduino:avr and lib install Servo"
	}
	imgui.PushStyleColorVec4(imgui.ColText, imgui.Vec4{X: 0.95, Y: 0.75, Z: 0.3, W: 1})
	imgui.TextWrapped(msg)
	imgui.PopStyleColor()
	if imgui.SmallButton(btn) {
		a.installArduinoCLI()
	}
	a.markItem("btn:electronics.installcli")
	imgui.SetItemTooltip(tip)
	if ce.setupErr != "" {
		imgui.TextColored(colErr, ce.setupErr)
	}
}

// codeWindow is the code editor's window.
func (a *App) codeWindow() {
	ce := &a.code
	a.codeWatch()
	if !ce.open || ce.ed == nil {
		return
	}
	f := a.doc.D.Feature(ce.circuit)
	if f == nil || f.Kind != doc.KCircuit {
		ce.open, ce.circuit = false, 0
		return
	}
	imgui.SetNextWindowSizeV(imgui.Vec2{X: 640, Y: 560}, imgui.CondFirstUseEver)
	open := true
	if imgui.BeginV(ui.Icon(icCode, "Sketch code · "+f.Name)+"###code", &open, imgui.WindowFlagsNoSavedSettings) {
		busy := ce.compiling
		imgui.BeginDisabledV(busy)
		if imgui.Button(ui.Icon(ui.FaCheck, "Compile and load")) {
			a.codeCompile(false)
		}
		a.markItem("btn:code.compile")
		imgui.EndDisabled()
		imgui.SetItemTooltip("Build the code with arduino-cli for the Uno and load it into the circuit")
		imgui.SameLine()
		names := make([]string, 0, len(sketchExamples))
		for n := range sketchExamples {
			names = append(names, n)
		}
		sort.Strings(names)
		imgui.SetNextItemWidth(160)
		if imgui.BeginCombo("##examples", "Start from an example") {
			for _, n := range names {
				if imgui.SelectableBool(n) {
					a.codeExample(n)
				}
			}
			imgui.EndCombo()
		}
		imgui.SameLine()
		if imgui.Button("Open .ino…") {
			a.pickFile(false, "Open a sketch", "Open", startIn("", ""), firmwareFilter[2:], func(p string) {
				if b, err := os.ReadFile(p); err == nil {
					ce.ed.SetText(string(b))
					a.codeKeep()
				}
			})
		}
		imgui.SameLine()
		if imgui.Button("Save .ino…") {
			a.codeKeep()
			a.pickFile(true, "Save the sketch", "Save", defaultPath(sketchName(f.Name)+".ino"), firmwareFilter[2:], func(p string) {
				if err := os.WriteFile(p, []byte(f.Circuit.Source), 0o644); err != nil {
					a.notify("Save failed: %v", err)
				}
			})
		}
		imgui.SameLine()
		imgui.BeginDisabledV(vsCode() == "")
		if imgui.Button("Open in VS Code") {
			a.codeInVSCode()
		}
		imgui.EndDisabled()
		if vsCode() == "" {
			if imgui.IsItemHoveredV(imgui.HoveredFlagsAllowWhenDisabled) {
				imgui.SetTooltip("VS Code isn't installed")
			}
		} else if ce.file != "" {
			imgui.SetItemTooltip("Shared with " + ce.file + ": saving it there updates the code here")
		}
		a.cliBanner()
		if ce.status != "" {
			col := imgui.Vec4{X: 0.6, Y: 0.85, Z: 0.6, W: 1}
			if strings.HasPrefix(ce.status, "Didn't") {
				col = colErr
			}
			imgui.TextColored(col, ce.status)
		}
		avail := imgui.ContentRegionAvail()
		h := avail.Y
		if len(ce.msgs) > 0 {
			h -= float32(min(len(ce.msgs), 5))*imgui.TextLineHeightWithSpacing() + 12
		}
		if ui.Mono != nil {
			imgui.PushFont(ui.Mono, 0)
		}
		ce.ed.RenderV("##editor", imgui.Vec2{X: avail.X, Y: h}, imgui.ChildFlagsBorders, 0)
		if ui.Mono != nil {
			imgui.PopFont()
		}
		if ce.ed.UndoIndex() != ce.undo {
			a.codeKeep()
		}
		// the compiler's messages: a click goes to the line
		if len(ce.msgs) > 0 && imgui.BeginChildStrV("##msgs", imgui.Vec2{}, 0, 0) {
			for i, m := range ce.msgs {
				col := imgui.Vec4{X: 0.95, Y: 0.75, Z: 0.2, W: 1}
				if m.Error {
					col = colErr
				}
				imgui.PushStyleColorVec4(imgui.ColText, col)
				if imgui.SelectableBool(fmt.Sprintf("line %d:%d  %s##m%d", m.Line, m.Col, m.Text, i)) {
					ce.ed.SetCursor(*cte.NewDocPossizet(uint64(max(0, m.Line-1)), uint64(max(0, m.Col-1))))
					ce.ed.SetFocus()
				}
				imgui.PopStyleColor()
			}
			imgui.EndChild()
		}
	}
	imgui.End()
	if !open {
		a.codeKeep()
		a.doc.Commit()
		ce.open = false
	}
}
