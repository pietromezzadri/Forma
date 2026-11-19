package expr

import (
	"math"
	"testing"
)

func TestEval(t *testing.T) {
	vars := map[string]float64{"w": 40, "wall_t": 2.5}
	for src, want := range map[string]float64{
		"20":             20,
		"20mm":           20,
		"2cm":            20,
		"1in":            25.4,
		"1 in + 2mm":     27.4,
		"w/2":            20,
		"(w - 2*wall_t)": 35,
		"-3 + 2^3":       5,
		"sqrt(16)":       4,
		"cos(60)":        0.5,
		"45deg":          45,
		"1.5e2":          150,
		"pi*2":           2 * math.Pi,
	} {
		got, err := Eval(src, vars)
		if err != nil {
			t.Errorf("%q: %v", src, err)
			continue
		}
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("%q = %g, want %g", src, got, want)
		}
	}
	for _, bad := range []string{"", "2 +", "(3", "foo", "1/0", "3 3"} {
		if _, err := Eval(bad, vars); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
