package sketch

import (
	"encoding/json"

	"forma/geom"
)

type sketchJSON struct {
	Name   string        `json:"name"`
	Plane  geom.Frame    `json:"plane"`
	Params []float64     `json:"params"`
	Ents   []*Entity     `json:"ents"`
	Cons   []*Constraint `json:"cons"`
	Next   int           `json:"next"`
}

// MarshalJSON saves the sketch's geometry and constraints (not the solve
// status).
func (s *Sketch) MarshalJSON() ([]byte, error) {
	return json.Marshal(sketchJSON{s.Name, s.Plane, s.Params, s.Ents, s.Cons, s.nextID})
}

func (s *Sketch) UnmarshalJSON(b []byte) error {
	var j sketchJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	*s = Sketch{Name: j.Name, Plane: j.Plane, Params: j.Params, Ents: j.Ents, Cons: j.Cons, nextID: j.Next}
	if s.Params == nil {
		s.Params = []float64{}
	}
	return nil
}

// Clone is a deep copy (through JSON).
func (s *Sketch) Clone() *Sketch {
	b, _ := s.MarshalJSON()
	c := &Sketch{}
	_ = c.UnmarshalJSON(b)
	c.Status = s.Status
	return c
}
