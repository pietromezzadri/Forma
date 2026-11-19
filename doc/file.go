package doc

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Encode writes the design as JSON: parameters and the feature history
// (results are rebuilt on load).
func (d *Document) Encode(w io.Writer) error {
	e := json.NewEncoder(w)
	e.SetIndent("", " ")
	d.Version = Version
	return e.Encode(d)
}

// Marshal is the design as JSON bytes.
func (d *Document) Marshal() []byte {
	d.Version = Version
	b, _ := json.Marshal(d)
	return b
}

// Decode reads a design and regenerates it.
func Decode(r io.Reader) (*Document, error) {
	var d Document
	if err := json.NewDecoder(r).Decode(&d); err != nil {
		return nil, err
	}
	return finish(&d)
}

// Unmarshal reads a design from JSON bytes and regenerates it.
func Unmarshal(b []byte) (*Document, error) {
	var d Document
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	return finish(&d)
}

func finish(d *Document) (*Document, error) {
	if d.Version > Version {
		return nil, fmt.Errorf("made by a newer Forma (format %d)", d.Version)
	}
	for _, f := range d.Features {
		if f.Inputs == nil {
			f.Inputs = map[string]string{}
		}
		if f.ID > d.NextID {
			d.NextID = f.ID
		}
	}
	d.Regenerate()
	return d, nil
}

// Save writes the design to a file (through a temporary file, so a failed
// write never leaves half a design).
func (d *Document) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := d.Encode(f); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Open reads a design file.
func Open(path string) (*Document, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Decode(f)
}

// History keeps snapshots of the design for undo and redo.
type History struct {
	snaps [][]byte
	pos   int // index of the current state
	max   int
}

func NewHistory(d *Document) *History {
	return &History{snaps: [][]byte{d.Marshal()}, max: 200}
}

// Push records the design after a change (dropping what was undone). A
// snapshot equal to the current one is not pushed.
func (h *History) Push(d *Document) {
	b := d.Marshal()
	if string(b) == string(h.snaps[h.pos]) {
		return
	}
	h.snaps = append(h.snaps[:h.pos+1], b)
	if len(h.snaps) > h.max {
		h.snaps = h.snaps[len(h.snaps)-h.max:]
	}
	h.pos = len(h.snaps) - 1
}

func (h *History) CanUndo() bool { return h.pos > 0 }
func (h *History) CanRedo() bool { return h.pos < len(h.snaps)-1 }

// Undo returns the design as it was before the last change.
func (h *History) Undo() (*Document, error) {
	if !h.CanUndo() {
		return nil, fmt.Errorf("nothing to undo")
	}
	h.pos--
	return Unmarshal(h.snaps[h.pos])
}

// Redo returns the design as it was after the change last undone.
func (h *History) Redo() (*Document, error) {
	if !h.CanRedo() {
		return nil, fmt.Errorf("nothing to redo")
	}
	h.pos++
	return Unmarshal(h.snaps[h.pos])
}

// Changed reports whether the design differs from the current snapshot.
func (h *History) Changed(d *Document) bool { return string(d.Marshal()) != string(h.snaps[h.pos]) }
