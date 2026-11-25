package doc

import (
	"math"
	"strings"
	"testing"
	"time"

	"forma/brep"
	"forma/drawing"
	"forma/geom"
)

// Modelled threads: a socket head screw, a nut and a tapped hole come out
// as valid solids with the thread's material gone (between the plain
// shank or bore and the thread's minor diameter, about half of it), and a
// drawing shows them by their outlines and thin lines, not their facets.
func TestModelledThreads(t *testing.T) {
	t0 := time.Now()
	// the screw: 2d + 12 = 22 of its 30 threaded, a plain shank above
	plain, err := ScrewDesign(ScrewSpec{Std: "ISO 4762", Size: "M5", Length: 30})
	if err != nil {
		t.Fatal(err)
	}
	mod, err := ScrewDesign(ScrewSpec{Std: "ISO 4762", Size: "M5", Length: 30, Modelled: true})
	if err != nil {
		t.Fatal(err)
	}
	sb := mod.Bodies[0].Body
	if err := brep.Check(sb); err != nil {
		t.Fatalf("screw: %v", err)
	}
	// the thread takes about the ring between d and d3 over 22 mm, the
	// part of it the teeth leave: the profile's area a pitch is 0.3907 P²
	// out of the ring's (d − d3)/2 · P = 0.6134 P²
	gone := volume(t, plain.Bodies[0].Body) - volume(t, sb)
	ring := math.Pi * (5*5 - math.Pow(5-1.22687*0.8, 2)) / 4 * 22
	t.Logf("screw: %d faces, %.2f mm³ gone of a %.2f ring (%.0f%%) in %v", len(sb.Faces()), gone, ring, 100*gone/ring, time.Since(t0))
	if gone < 0.3*ring || gone > 0.6*ring {
		t.Fatalf("the thread took %.2f mm³ of a %.2f ring", gone, ring)
	}
	if bx := sb.Bounds(); math.Abs(bx.Min.Z+30) > 0.02 {
		t.Fatalf("the tip at %.4f", bx.Min.Z)
	}

	// the nut: bored under the crests, the thread cut through
	t0 = time.Now()
	nut, err := FastenerDesign(PartSpec{Kind: "nut", Std: "ISO 4032", Size: "M5", Modelled: true})
	if err != nil {
		t.Fatal(err)
	}
	nb := nut.Bodies[0].Body
	if err := brep.Check(nb); err != nil {
		t.Fatalf("nut: %v", err)
	}
	pn, _ := FastenerDesign(PartSpec{Kind: "nut", Std: "ISO 4032", Size: "M5"})
	// against one bored at the nominal size: more material (the teeth)
	if more := volume(t, nb) - volume(t, pn.Bodies[0].Body); more <= 0 {
		t.Fatalf("the modelled nut has %.2f mm³ less than a plain one", -more)
	}
	t.Logf("nut: %d faces in %v", len(nb.Faces()), time.Since(t0))

	// a tapped hole through a 10 mm plate, modelled
	t0 = time.Now()
	d := New()
	bx := d.Insert(NewPrim(KBox))
	bx.Inputs["Width"], bx.Inputs["Depth"], bx.Inputs["Height"] = "30", "20", "10"
	sk := d.Insert(NewSketch(PlaneRef{Std: "xy"}))
	sk.Inputs = map[string]string{"Offset": "10"}
	p := sk.Sketch.AddPoint(geom.V2(10, 10)).ID
	h := d.Insert(NewHole(sk.ID, []int{p}))
	h.Inputs["Extent"], h.Inputs["Screw"], h.Inputs["Fit"], h.Inputs["Thread"] = "Through all", "M5", "Tapped", "Modelled"
	d.Regenerate()
	if h.Err != "" {
		t.Fatal(h.Err)
	}
	pb := d.Bodies[0].Body
	if err := brep.Check(pb); err != nil {
		t.Fatalf("tapped plate: %v", err)
	}
	hole := 6000 - volume(t, pb)
	// between the minor and the major diameter's cylinders
	lo, hi := math.Pi*math.Pow(5-1.0825*0.8, 2)/4*10, math.Pi*25/4*10
	t.Logf("tapped plate: %d faces, a %.2f mm³ hole (%.2f..%.2f) in %v", len(pb.Faces()), hole, lo, hi, time.Since(t0))
	if hole < lo || hole > hi {
		t.Fatalf("the tapped hole is %.2f mm³, not between %.2f and %.2f", hole, lo, hi)
	}

	// drawn: no facet edges; outlines and thin lines; a callout on its
	// outline circle seen from above
	bodies := d.DrawingBodies()
	if len(bodies[0].Threads) != 1 || math.Abs(bodies[0].Threads[0].A.Dist(bodies[0].Threads[0].B)-10) > 0.05 {
		t.Fatalf("the plate's thread: %+v", bodies[0].Threads)
	}
	sh := drawing.NewSheet("t", "A4")
	top := sh.AddView(&drawing.View{Kind: "base", Dir: "top", X: 100, Y: 140})
	sec := sh.AddView(&drawing.View{Kind: "section", Parent: top.ID, Side: "below", X: 100, Y: 60, Label: "A"})
	fr, _ := sh.FrameOf(sec)
	sec.At = geom.V3(10, 10, 5).Dot(fr.D)
	res := sh.Build(bodies, new(drawing.Cache))
	for _, v := range []*drawing.View{top, sec} {
		b := res.Views[v.ID]
		if b.Err != "" {
			t.Fatal(b.Err)
		}
		facets, outlines, thin := 0, 0, 0
		for _, c := range b.Proj.Curves {
			switch {
			case c.Edge != nil && (drawing.ThreadFacet(c.Edge.HE[0].Loop.Face.Name) || drawing.ThreadFacet(c.Edge.HE[1].Loop.Face.Name)):
				facets++
			case c.Thread && !c.Hidden:
				thin++
			case c.Name != "" && !c.Hidden:
				outlines++
			}
		}
		t.Logf("%s view: %d facet edges, %d outlines, %d thin lines", v.Kind, facets, outlines, thin)
		if facets != 0 || outlines == 0 || thin == 0 {
			t.Fatalf("%s view: %d facet edges, %d outlines, %d thin lines", v.Kind, facets, outlines, thin)
		}
	}
	b := res.Views[top.ID]
	an, ok := b.Pick(b.SheetOf(geom.V3(10+2.5-0.541266*0.8, 10, 10)), 1)
	if !ok {
		t.Fatal("the outline wasn't picked")
	}
	sh.AddDim(&drawing.Dim{View: top.ID, Kind: "hole", Anchors: []drawing.Anchor{an}, Place: geom.V2(30, 30)})
	res = sh.Build(bodies, new(drawing.Cache))
	if len(res.Dims) != 0 {
		t.Fatalf("callout: %v", res.Dims)
	}
	found := false
	for _, tx := range res.Page.Texts {
		found = found || strings.Contains(tx.S, "M5×0.8 THRU")
	}
	if !found {
		t.Fatal("no M5×0.8 callout")
	}
}
