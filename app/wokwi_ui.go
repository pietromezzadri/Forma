package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ncruces/zenity"

	"forma/doc"
)

var wokwiFilter = zenity.FileFilters{{Name: "Wokwi diagram (diagram.json)", Patterns: []string{"*.json"}}}

// elecWokwi brings a Wokwi project in as a new circuit: its diagram.json
// (path, or the folder holding it) wired as it was, its sketch as the
// circuit's code, compiled if arduino-cli is there. What it left out is
// shown on the wiring sheet.
func (a *App) elecWokwi(path string) {
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		path = filepath.Join(path, "diagram.json")
	}
	diagram, err := os.ReadFile(path)
	if err != nil {
		a.notify("Wokwi: %v", err)
		return
	}
	dir := filepath.Dir(path)
	sketch := ""
	inos, _ := filepath.Glob(filepath.Join(dir, "*.ino"))
	for _, p := range inos {
		if strings.EqualFold(filepath.Base(p), "sketch.ino") || sketch == "" {
			if b, err := os.ReadFile(p); err == nil {
				sketch = string(b)
			}
		}
	}
	c, notes, err := doc.ReadWokwi(diagram, sketch)
	if err != nil {
		a.notify("Wokwi: %v", err)
		return
	}
	if sketch == "" {
		notes = append(notes, "no sketch (.ino) beside the diagram: load firmware or write the code")
	}
	a.elecReset()
	f := doc.NewCircuit()
	f.Circuit = c
	if name := filepath.Base(dir); name != "." && name != string(filepath.Separator) {
		f.Name = name
	}
	f = a.doc.D.Insert(f)
	a.doc.Commit()
	a.elec.circuit, a.elec.open = f.ID, true
	a.elec.sheet.open, a.elec.sheet.fit = true, true
	a.elec.sheet.notes = notes
	a.elec.sheet.notesFor = f.ID
	a.notify("Read %s: %d parts, %d wires", filepath.Base(path), len(c.Parts), len(c.Wires))
	if sketch != "" {
		if arduinoCLI() != "" {
			a.codeCompile(a.capture != nil)
			a.code.open = false // the sheet, not the code, is what to look at
		} else {
			a.notify("The sketch came in as the circuit's code; install arduino-cli to compile it")
		}
	}
}
