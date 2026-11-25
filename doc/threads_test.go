package doc

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"

	"forma/drawing"
	"forma/geom"
)

// A block with a tapped M5 hole through it and two counterbored M5
// clearance holes, and a socket head M5×30 beside it: the threads the
// drawing gets, the callouts, and what each view shows of them.
func TestThreadsAndCallouts(t *testing.T) {
	d := New()
	bx := d.Insert(NewPrim(KBox))
	bx.Inputs["Width"], bx.Inputs["Depth"], bx.Inputs["Height"] = "40", "30", "20"
	sk := d.Insert(NewSketch(PlaneRef{Std: "xy"}))
	sk.Inputs = map[string]string{"Offset": "20"}
	p0 := sk.Sketch.AddPoint(geom.V2(8, 15)).ID
	p1 := sk.Sketch.AddPoint(geom.V2(20, 15)).ID
	p2 := sk.Sketch.AddPoint(geom.V2(32, 15)).ID
	tap := d.Insert(NewHole(sk.ID, []int{p0}))
	tap.Inputs["Extent"], tap.Inputs["Screw"], tap.Inputs["Fit"] = "Through all", "M5", "Tapped"
	cb := d.Insert(NewHole(sk.ID, []int{p1, p2}))
	cb.Inputs["Type"], cb.Inputs["Extent"], cb.Inputs["Screw"], cb.Inputs["Fit"] = "Counterbore", "Through all", "M5", "Normal"
	s, err := NewScrew(ScrewSpec{"ISO 4762", "M5", 30})
	if err != nil {
		t.Fatal(err)
	}
	s.Inputs["Move X"] = "60"
	d.Insert(s)
	d.Regenerate()
	for _, f := range d.Features {
		if f.Err != "" {
			t.Fatalf("%s: %s", f.Name, f.Err)
		}
	}
	bodies := d.DrawingBodies()
	if len(bodies) != 2 {
		t.Fatalf("%d bodies", len(bodies))
	}
	block, screw := bodies[0], bodies[1]

	// the tapped hole's thread: M5's nominal radius, right through
	if len(block.Threads) != 1 {
		t.Fatalf("the block has %d threads, want 1: %+v", len(block.Threads), block.Threads)
	}
	th := block.Threads[0]
	if math.Abs(th.R-2.5) > 1e-9 || math.Abs(th.A.Dist(th.B)-20) > 1e-6 || th.External {
		t.Fatalf("the tapped hole's thread: %+v", th)
	}
	// the screw's: the minor diameter, 2d + 12 = 22 up from the tip, a
	// line across where it stops
	if len(screw.Threads) != 1 {
		t.Fatalf("the screw has %d threads", len(screw.Threads))
	}
	st := screw.Threads[0]
	if !st.External || math.Abs(st.R-(5-1.22687*0.8)/2) > 1e-9 || math.Abs(st.A.Dist(st.B)-22) > 1e-6 || st.Limit != 2.5 || math.Abs(st.B.Z+30) > 1e-6 {
		t.Fatalf("the screw's thread: %+v", st)
	}

	// callouts, by any face of the hole
	callFor := func(step *Feature) []string {
		for _, f := range block.Body.Faces() {
			if strings.HasPrefix(f.Name, "f"+strconv.Itoa(step.ID)+"/hole:") {
				return d.HoleCallout(f.Name)
			}
		}
		return nil
	}
	if got, want := callFor(tap), []string{"Ø4.2 THRU", "M5×0.8 THRU"}; !slices.Equal(got, want) {
		t.Fatalf("tapped callout %q, want %q", got, want)
	}
	if got, want := callFor(cb), []string{"2× Ø5.5 THRU", "{cbore} Ø10 {depth}5.4"}; !slices.Equal(got, want) {
		t.Fatalf("counterbore callout %q, want %q", got, want)
	}
	if d.HoleCallout(block.Body.Faces()[0].Name) != nil && !strings.Contains(block.Body.Faces()[0].Name, "/hole:") {
		t.Fatal("a box face has a callout")
	}

	// the views
	sh := drawing.NewSheet("t", "A3")
	top := sh.AddView(&drawing.View{Kind: "base", Dir: "top", X: 120, Y: 200})
	front := sh.AddView(&drawing.View{Kind: "base", Dir: "front", X: 120, Y: 110})
	sec := sh.AddView(&drawing.View{Kind: "section", Parent: top.ID, Side: "right", X: 300, Y: 200, Label: "A"})
	fr, err := sh.FrameOf(sec)
	if err != nil {
		t.Fatal(err)
	}
	sec.At = geom.V3(0, 15, 0).Dot(fr.D)
	if math.Abs(math.Abs(fr.D.Y)-1) > 1e-9 {
		// a section right of the top view looks along X: cut through the
		// holes' line instead along Y
		sec.Side = "below"
		sec.Y, sec.X = 60, 120
		fr, _ = sh.FrameOf(sec)
		sec.At = geom.V3(0, 15, 0).Dot(fr.D)
	}
	res := sh.Build(bodies, new(drawing.Cache))
	threads := func(v *drawing.View, key int) (vis, hid, limit int) {
		b := res.Views[v.ID]
		if b.Err != "" {
			t.Fatalf("view %s: %s", v.Kind, b.Err)
		}
		for _, c := range b.Proj.Curves {
			if c.Body != key {
				continue
			}
			switch {
			case c.Thread && c.Hidden:
				hid++
			case c.Thread:
				vis++
			case c.Edge == nil && c.Face == nil:
				limit++
			}
		}
		return
	}
	// from above: the tapped hole's three quarters of a circle at its top;
	// the screw's tip is under its head
	if v, _, _ := threads(top, block.Key); v != 1 {
		t.Fatalf("top view: %d thread arcs on the block, want 1", v)
	}
	if v, _, _ := threads(top, screw.Key); v != 0 {
		t.Fatalf("top view: %d thread arcs on the screw, want 0", v)
	}
	// from the front: the hole's thread is inside the block (hidden lines
	// off), the screw's two lines and its thread's end show
	if v, h, _ := threads(front, block.Key); v != 0 || h != 0 {
		t.Fatalf("front view: the block's thread shows (%d, %d)", v, h)
	}
	if v, _, l := threads(front, screw.Key); v != 2 || l != 1 {
		t.Fatalf("front view: the screw's thread %d lines and %d ends, want 2 and 1", v, l)
	}
	// cut through the holes: the tapped hole's two lines
	if v, _, _ := threads(sec, block.Key); v != 2 {
		t.Fatalf("section: %d thread lines on the block, want 2", v)
	}

	// hole callouts on the top view, picked by their rims
	b := res.Views[top.ID]
	for _, p := range []geom.Vec3{{X: 10.1, Y: 15, Z: 20}, {X: 37, Y: 15, Z: 20}} {
		an, ok := b.Pick(b.SheetOf(p), 1)
		if !ok {
			t.Fatalf("nothing picked at %v", p)
		}
		sh.AddDim(&drawing.Dim{View: top.ID, Kind: "hole", Anchors: []drawing.Anchor{an}, Place: geom.V2(40, 30)})
	}
	res = sh.Build(bodies, new(drawing.Cache))
	if len(res.Dims) != 0 {
		t.Fatalf("callouts not drawn: %v", res.Dims)
	}
	var texts []string
	for _, tx := range res.Page.Texts {
		texts = append(texts, tx.S)
	}
	for _, want := range []string{"Ø4.2 THRU", "M5×0.8 THRU", "2× Ø5.5 THRU", " Ø10 ", "5.4"} {
		if !slices.Contains(texts, want) {
			t.Fatalf("no %q on the sheet: %q", want, texts)
		}
	}
	// and a plain box edge can't take one
	an, ok := b.Pick(b.SheetOf(geom.V3(0, 0, 20)), 1)
	if ok {
		if _, err := (&drawing.Dim{View: top.ID, Kind: "hole", Anchors: []drawing.Anchor{an}}).Value(b); err == nil {
			t.Fatal("a box corner took a hole callout")
		}
	}
}
