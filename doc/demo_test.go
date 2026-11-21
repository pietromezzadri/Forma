package doc

import (
	"os"
	"strings"
	"testing"
)

func designOf(t *testing.T, prims ...[2]any) []byte {
	t.Helper()
	d := New()
	for _, p := range prims {
		d.Insert(NewPrim(p[0].(string))).Inputs = p[1].(map[string]string)
	}
	d.Regenerate()
	mustOK(t, d.Features...)
	return d.Marshal()
}

// TestElectronicsDemo builds the M15 capture's assembly — an SG90 with an
// arm on its shaft and a TT motor with a turntable — with both joints, and
// writes it to $FORMA_WRITE_DEMO if that is set (captures/data/arm_and_wheel.forma),
// and beside it, wired to the sweep_drive sketch, as …_wired.forma.
func TestElectronicsDemo(t *testing.T) {
	servo := designOf(t,
		[2]any{KBox, map[string]string{"Width": "23", "Depth": "12", "Height": "22", "X": "0", "Y": "0", "Z": "0"}},
		[2]any{KBox, map[string]string{"Width": "32", "Depth": "12", "Height": "2.5", "X": "-4.5", "Y": "0", "Z": "15.5", "Operation": "Join"}},
		[2]any{KCylinder, map[string]string{"Radius": "5.75", "Height": "1.5", "X": "6", "Y": "6", "Z": "22", "Operation": "Join"}},
		[2]any{KCylinder, map[string]string{"Radius": "2.4", "Height": "4", "X": "6", "Y": "6", "Z": "23.5", "Operation": "Join"}},
	)
	arm := designOf(t,
		[2]any{KCylinder, map[string]string{"Radius": "3.5", "Height": "3", "X": "0", "Y": "0", "Z": "0"}},
		[2]any{KBox, map[string]string{"Width": "26", "Depth": "6", "Height": "2", "X": "2", "Y": "-3", "Z": "0.5", "Operation": "Join"}},
		[2]any{KCylinder, map[string]string{"Radius": "3", "Height": "2.4", "X": "30", "Y": "0", "Z": "0.3", "Operation": "Join"}},
		[2]any{KCylinder, map[string]string{"Radius": "1", "Height": "4", "X": "30", "Y": "0", "Z": "0", "Operation": "Cut"}},
	)
	motor := designOf(t,
		[2]any{KBox, map[string]string{"Width": "22", "Depth": "22", "Height": "18", "X": "0", "Y": "0", "Z": "0"}},
		[2]any{KCylinder, map[string]string{"Radius": "2.7", "Height": "6", "X": "11", "Y": "11", "Z": "18", "Operation": "Join"}},
	)
	disc := designOf(t,
		[2]any{KCylinder, map[string]string{"Radius": "22", "Height": "4", "X": "0", "Y": "0", "Z": "0"}},
		[2]any{KCylinder, map[string]string{"Radius": "5", "Height": "8", "X": "13", "Y": "0", "Z": "-2", "Operation": "Cut"}},
		[2]any{KCylinder, map[string]string{"Radius": "3", "Height": "8", "X": "-13", "Y": "0", "Z": "-2", "Operation": "Cut"}},
		[2]any{KBox, map[string]string{"Width": "4", "Depth": "18", "Height": "3", "X": "-2", "Y": "4", "Z": "3.5", "Operation": "Join"}},
	)
	d := New()
	sv := d.Insert(NewComponent("servo.forma", servo))
	sv.Inputs["Grounded"] = "1"
	am := d.Insert(NewComponent("arm.forma", arm))
	am.Inputs["Move X"] = "-40"
	mo := d.Insert(NewComponent("tt motor.forma", motor))
	mo.Inputs["Move X"], mo.Inputs["Move Y"], mo.Inputs["Grounded"] = "45", "-5", "1"
	ds := d.Insert(NewComponent("turntable.forma", disc))
	ds.Inputs["Move X"] = "120"
	d.Regenerate()
	mustOK(t, d.Features...)

	join := func(fixed, moving *Feature, fx, fy, fz, fr, mx, my, mz, mr float64) *Feature {
		fk, mk := BodyKey(fixed.ID, 0), BodyKey(moving.ID, 0)
		fe, fp := circleEdge(t, d, fk, fx, fy, fz, fr)
		me, mp := circleEdge(t, d, mk, mx, my, mz, mr)
		ja, err := d.JointRefFor(fk, nil, fe, nil, fp)
		if err != nil {
			t.Fatal(err)
		}
		jb, err := d.JointRefFor(mk, nil, me, nil, mp)
		if err != nil {
			t.Fatal(err)
		}
		j := d.Insert(NewJoint(ja, jb, "Revolute"))
		d.Regenerate()
		mustOK(t, j)
		return j
	}
	j1 := join(sv, am, 6, 6, 27.5, 2.4, -40, 0, 0, 3.5)
	j2 := join(mo, ds, 56, 6, 24, 2.7, 120, 0, 0, 22)
	j1.Name, j2.Name = "Arm", "Turntable"
	d.Regenerate()
	mustOK(t, d.Features...)
	if out := os.Getenv("FORMA_WRITE_DEMO"); out != "" {
		if err := os.WriteFile(out, d.Marshal(), 0o644); err != nil {
			t.Fatal(err)
		}
		// and wired: the sweep_drive sketch, the servo on the arm, the motor
		// behind a TB6612 on the turntable, a pot, a button and the LED
		c := d.Insert(NewCircuit())
		c.Data, c.Circuit.File = firmware(t, "sweep_drive"), "sweep_drive.hex"
		ci := c.Circuit
		add := func(model, name string, pins map[string]string) *CircuitPart {
			p := NewCircuitPart(ci, model, name)
			if pins != nil {
				p.Pins = pins
			}
			ci.Parts = append(ci.Parts, p)
			return &ci.Parts[len(ci.Parts)-1]
		}
		add("sg90", "arm servo", nil).Joint = j1.ID
		add("battery-4aa", "battery", nil)
		add("tt-motor", "motor", nil).Joint = j2.ID
		add("tb6612", "driver", map[string]string{"in1": "D7", "in2": "D8", "enable": "D6"})
		add("pot", "speed", nil)
		add("button", "reverse", map[string]string{"pin": "D2"})
		add("led", "led", map[string]string{"pin": "D13"})
		d.Regenerate()
		mustOK(t, d.Features...)
		if _, err := d.NewStudy(c); err != nil {
			t.Fatal(err)
		}
		wired := strings.TrimSuffix(out, ".forma") + "_wired.forma"
		if err := os.WriteFile(wired, d.Marshal(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
