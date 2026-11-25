package ops

import (
	"testing"
	"time"

	"forma/brep"
	"forma/geom"
)

// A threaded rod joined to a shank under a head (a bolt) and cut from a
// block drilled under its crests (a tapped hole): valid solids, quickly.
func TestThreadedRod(t *testing.T) {
	F := geom.FrameZ(geom.V3(0, 0, 0), geom.ZAxis, geom.XAxis)
	// a bolt: the rod from the tip (-16) to -4, a plain shank -4..0, a head
	t0 := time.Now()
	rod, err := ThreadedRod(ThreadSpec{F: F, D: 5, P: 0.8, Z0: -16, Z1: -4})
	if err != nil {
		t.Fatal(err)
	}
	if err := brep.Check(rod); err != nil {
		t.Fatalf("rod: %v", err)
	}
	t1 := time.Now()
	shank, _ := Cylinder(geom.FrameZ(geom.V3(0, 0, -4), geom.ZAxis, geom.XAxis), 2.5, 4)
	head, _ := Cylinder(F, 4.25, 5)
	sh, err := brep.Boolean(shank, head, brep.Union)
	if err != nil {
		t.Fatal(err)
	}
	bolt, err := brep.Boolean(sh, rod, brep.Union)
	if err != nil {
		t.Fatalf("bolt: %v", err)
	}
	if err := brep.Check(bolt); err != nil {
		t.Fatalf("bolt: %v", err)
	}
	t.Logf("bolt: rod %d faces in %v, joined in %v, %d faces, volume %.2f (rod %.2f)", len(rod.Faces()), t1.Sub(t0), time.Since(t1), len(bolt.Faces()), brep.Mass(bolt, 2e-4).Volume, brep.Mass(rod, 2e-4).Volume)

	// a tapped hole through a block: drilled under the crest, the rod cut
	s := ThreadSpec{F: geom.FrameZ(geom.V3(0, 0, 0), geom.ZAxis, geom.XAxis), D: 5, P: 0.8, Z0: -18, Z1: 2, Internal: true}
	t0 = time.Now()
	blk, _ := Box(geom.V3(-6, -6, -16), geom.V3(6, 6, 0))
	drill, _ := Cylinder(geom.FrameZ(geom.V3(0, 0, -17), geom.ZAxis, geom.XAxis), s.Crest()-0.01, 18)
	b, err := brep.Boolean(blk, drill, brep.Subtract)
	if err != nil {
		t.Fatal(err)
	}
	hrod, err := ThreadedRod(s)
	if err != nil {
		t.Fatal(err)
	}
	t1 = time.Now()
	nut, err := brep.Boolean(b, hrod, brep.Subtract)
	if err != nil {
		t.Fatalf("tapped: %v", err)
	}
	if err := brep.Check(nut); err != nil {
		t.Fatalf("tapped: %v", err)
	}
	t.Logf("tapped: rod %d faces in %v, cut in %v, %d faces, volume %.2f", len(hrod.Faces()), t1.Sub(t0), time.Since(t1), len(nut.Faces()), brep.Mass(nut, 2e-4).Volume)
}
