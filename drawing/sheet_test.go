package drawing

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/xml"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"forma/brep"
	"forma/geom"
	"forma/ops"
)

// bracket is the plate with its hole and a Ø20 boss 15 tall on it.
func bracket(t *testing.T) *brep.Body {
	t.Helper()
	p := plate(t)
	boss, err := ops.Cylinder(geom.FrameZ(geom.V3(45, 20, 10), geom.ZAxis, geom.XAxis), 10, 15)
	if err != nil {
		t.Fatal(err)
	}
	out, err := brep.Boolean(p, boss, brep.Union)
	if err != nil {
		t.Fatal(err)
	}
	out.Name = "Bracket"
	return out
}

func TestSideFrames(t *testing.T) {
	f, _ := BaseFrame("front")
	for _, c := range []struct {
		side  string
		first bool
		want  string
	}{
		{"above", false, "top"}, {"right", false, "right"}, {"left", false, "left"}, {"below", false, "bottom"},
		{"below", true, "top"}, {"right", true, "left"}, {"above", true, "bottom"},
	} {
		got, err := SideFrame(f, c.side, c.first)
		if err != nil {
			t.Fatal(err)
		}
		want, _ := BaseFrame(c.want)
		if got.D.Dist(want.D) > 1e-12 || got.U.Dist(want.U) > 1e-12 || got.R.Dist(want.R) > 1e-12 {
			t.Errorf("%s (first angle %v): %+v, want the %s view %+v", c.side, c.first, got, c.want, want)
		}
	}
}

func TestScales(t *testing.T) {
	for s, want := range map[string]float64{"1:2": 0.5, "2:1": 2, "1:1": 1, "0.25": 0.25} {
		if got, err := ParseScale(s); err != nil || got != want {
			t.Errorf("%s: %v %v", s, got, err)
		}
	}
	if _, err := ParseScale("x:2"); err == nil {
		t.Error("x:2 read")
	}
	if FormatValue(12.5, 2) != "12.5" || FormatValue(40, 2) != "40" || FormatValue(3.14159, 2) != "3.14" {
		t.Error(FormatValue(12.5, 2), FormatValue(40, 2), FormatValue(3.14159, 2))
	}
}

func standardSheet(t *testing.T) (*Sheet, []Body) {
	bodies := []Body{{Key: 1, Name: "Bracket", Body: bracket(t), Gen: 1}}
	s := NewSheet("Bracket", "A4")
	s.Standard(bodies)
	return s, bodies
}

func TestStandardLayout(t *testing.T) {
	s, bodies := standardSheet(t)
	if len(s.Views) != 4 {
		t.Fatalf("%d views", len(s.Views))
	}
	if s.Scale != "1:1" {
		t.Errorf("scale %s, want 1:1 for a 60 mm part on A4", s.Scale)
	}
	res := s.Build(bodies, nil)
	ax0, ay0, ax1, ay1 := s.Area()
	for _, v := range s.Views {
		b := res.Views[v.ID]
		if b.Err != "" {
			t.Fatalf("view %d: %s", v.ID, b.Err)
		}
		if b.Lo.X < ax0 || b.Lo.Y < ay0 || b.Hi.X > ax1 || b.Hi.Y > ay1 {
			t.Errorf("view %s/%s outside the area: %v %v", v.Dir, v.Side, b.Lo, b.Hi)
		}
	}
	// the top view lines up with the front over it, the right beside it
	front, top, right := res.Views[s.Views[0].ID], res.Views[s.Views[1].ID], res.Views[s.Views[2].ID]
	if math.Abs(front.Lo.X-top.Lo.X) > 1e-9 || math.Abs(front.Hi.X-top.Hi.X) > 1e-9 || top.Lo.Y <= front.Hi.Y {
		t.Errorf("top %v %v not above the front %v %v", top.Lo, top.Hi, front.Lo, front.Hi)
	}
	if math.Abs(front.Lo.Y-right.Lo.Y) > 1e-9 || math.Abs(front.Hi.Y-right.Hi.Y) > 1e-9 || right.Lo.X <= front.Hi.X {
		t.Errorf("right %v %v not beside the front %v %v", right.Lo, right.Hi, front.Lo, front.Hi)
	}
	// moving the front takes the others with it
	s.Views[0].X += 10
	s.Views[0].Y += 5
	s.Align()
	if s.Views[1].X != s.Views[0].X || s.Views[2].Y != s.Views[0].Y {
		t.Error("projected views didn't follow")
	}
	// a big part goes down a scale
	big := []Body{{Key: 1, Body: scaled(t, bodies[0].Body, 5)}}
	s2 := NewSheet("Big", "A4")
	s2.Standard(big)
	if s2.Scale != "1:5" {
		t.Errorf("a 300 mm part on A4 at %s, want 1:5", s2.Scale)
	}
}

func scaled(t *testing.T, b *brep.Body, k float64) *brep.Body {
	out, err := brep.Scale(b, geom.Vec3{}, k)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// anchorAt picks what is under a model point in a view.
func anchorAt(t *testing.T, b *Built, p geom.Vec3) Anchor {
	t.Helper()
	a, ok := b.Pick(b.SheetOf(p), 0.8)
	if !ok {
		t.Fatalf("nothing at %v", p)
	}
	return a
}

func TestDimensions(t *testing.T) {
	s, bodies := standardSheet(t)
	res := s.Build(bodies, nil)
	front, top := res.Views[s.Views[0].ID], res.Views[s.Views[1].ID]
	// the plate's bottom edge in front, 60 long
	w := s.AddDim(&Dim{View: front.View.ID, Kind: "h", Anchors: []Anchor{anchorAt(t, front, geom.V3(30, 0, 0))}, Place: geom.V2(0, -25)})
	// the boss's height from the plate's top: its top edge's end to the plate's top-left corner
	hgt := s.AddDim(&Dim{View: front.View.ID, Kind: "v", Anchors: []Anchor{anchorAt(t, front, geom.V3(0, 0, 10)), anchorAt(t, front, geom.V3(40, 0, 25))}, Place: geom.V2(-40, 0)})
	// the hole and the boss seen from above
	hole := s.AddDim(&Dim{View: top.View.ID, Kind: "d", Anchors: []Anchor{anchorAt(t, top, geom.V3(28, 20, 10))}, Place: geom.V2(-20, 25)})
	boss := s.AddDim(&Dim{View: top.View.ID, Kind: "r", Anchors: []Anchor{anchorAt(t, top, geom.V3(55, 20, 25))}, Place: geom.V2(30, 22)})
	// hole centre to boss centre
	ctr := s.AddDim(&Dim{View: top.View.ID, Kind: "h", Anchors: []Anchor{anchorAt(t, top, geom.V3(20, 20, 10)), anchorAt(t, top, geom.V3(45, 20, 25))}, Place: geom.V2(0, -28)})
	// the corner's right angle
	ang := s.AddDim(&Dim{View: front.View.ID, Kind: "angle", Anchors: []Anchor{anchorAt(t, front, geom.V3(30, 0, 0)), anchorAt(t, front, geom.V3(60, 0, 5))}, Place: geom.V2(22, -2)})
	res = s.Build(bodies, nil)
	for d, want := range map[*Dim]float64{w: 60, hgt: 15, hole: 16, boss: 10, ctr: 25, ang: 90} {
		if e := res.Dims[d.ID]; e != "" {
			t.Fatalf("%s dim: %s", d.Kind, e)
		}
		got, err := d.Value(res.Views[d.View])
		if err != nil || math.Abs(got-want) > 1e-6 {
			t.Errorf("%s dim: %v (%v), want %v", d.Kind, got, err, want)
		}
	}
	if hole.Anchors[0].At != "" || ctr.Anchors[0].At != "centre" {
		t.Errorf("anchors: hole %+v, centre %+v", hole.Anchors[0], ctr.Anchors[0])
	}
	// the dimensions follow an edit: the boss moved 5 along and made Ø24
	p := plate(t)
	boss2, _ := ops.Cylinder(geom.FrameZ(geom.V3(50, 20, 10), geom.ZAxis, geom.XAxis), 12, 15)
	b2, err := brep.Boolean(p, boss2, brep.Union)
	if err != nil {
		t.Fatal(err)
	}
	res = s.Build([]Body{{Key: 1, Body: b2, Gen: 2}}, nil)
	if v, err := ctr.Value(res.Views[ctr.View]); err != nil || math.Abs(v-30) > 1e-6 {
		t.Errorf("centres after the edit: %v %v, want 30", v, err)
	}
	if v, err := boss.Value(res.Views[boss.View]); err != nil || math.Abs(v-12) > 1e-6 {
		t.Errorf("boss radius after the edit: %v %v, want 12", v, err)
	}
	writeAll(t, s, res.Page, "bracket")
}

func TestSectionSheet(t *testing.T) {
	bodies := []Body{{Key: 1, Name: "Bracket", Body: bracket(t), Gen: 1}}
	s := NewSheet("Section", "A3")
	s.Scale = "2:1"
	front := s.AddView(&View{Kind: "base", Dir: "top", X: 120, Y: 160, Tangent: true})
	fr, _ := s.FrameOf(front)
	sf, _ := SideFrame(fr, "below", s.FirstAngle)
	sec := s.AddView(&View{Kind: "section", Parent: front.ID, Side: "below", Y: 70, Label: s.NextLabel(), At: SectionAt(bodies, sf)})
	res := s.Build(bodies, nil)
	b := res.Views[sec.ID]
	if b.Err != "" || b.Proj.Warn != "" {
		t.Fatal(b.Err, b.Proj.Warn)
	}
	hatch := 0
	for _, p := range res.Page.Paths {
		if p.Pen == PenHatch {
			hatch++
		}
	}
	if len(b.Proj.Regions) == 0 || hatch < 20 {
		t.Fatalf("%d cut faces, %d hatch lines", len(b.Proj.Regions), hatch)
	}
	found := false
	for _, tx := range res.Page.Texts {
		if tx.S == "SECTION A-A" {
			found = true
		}
	}
	if !found {
		t.Error("no SECTION A-A label")
	}
	writeAll(t, s, res.Page, "section")
}

// writeAll writes the page in every format, checks each reads back, and
// keeps them when FORMA_DRAWING_OUT names a folder.
func writeAll(t *testing.T, s *Sheet, pg *Page, name string) {
	t.Helper()
	var svg, pdf, dxf bytes.Buffer
	if err := pg.WriteSVG(&svg); err != nil {
		t.Fatal(err)
	}
	if err := pg.WritePDF(&pdf); err != nil {
		t.Fatal(err)
	}
	if err := pg.WriteDXF(&dxf); err != nil {
		t.Fatal(err)
	}
	checkSVG(t, svg.Bytes(), pg)
	checkPDF(t, pdf.Bytes(), pg)
	checkDXF(t, dxf.Bytes(), pg)
	if dir := os.Getenv("FORMA_DRAWING_OUT"); dir != "" {
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, name+".svg"), svg.Bytes(), 0o644)
		os.WriteFile(filepath.Join(dir, name+".pdf"), pdf.Bytes(), 0o644)
		os.WriteFile(filepath.Join(dir, name+".dxf"), dxf.Bytes(), 0o644)
	}
}

func checkSVG(t *testing.T, b []byte, pg *Page) {
	t.Helper()
	d := xml.NewDecoder(bytes.NewReader(b))
	counts := map[string]int{}
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("SVG: %v", err)
		}
		if se, ok := tok.(xml.StartElement); ok {
			counts[se.Name.Local]++
		}
	}
	if counts["path"]+counts["circle"] < len(pg.Paths)-5 || counts["text"] != len(pg.Texts) || counts["polygon"] != len(pg.Fills) {
		t.Errorf("SVG has %v for %d paths, %d texts, %d fills", counts, len(pg.Paths), len(pg.Texts), len(pg.Fills))
	}
}

func checkPDF(t *testing.T, b []byte, pg *Page) {
	t.Helper()
	if !bytes.HasPrefix(b, []byte("%PDF-1.4")) || !bytes.HasSuffix(b, []byte("%%EOF\n")) {
		t.Fatal("PDF header or trailer")
	}
	// every xref offset points at its object
	m := regexp.MustCompile(`startxref\n(\d+)`).FindSubmatch(b)
	if m == nil {
		t.Fatal("no startxref")
	}
	x, _ := strconv.Atoi(string(m[1]))
	if !bytes.HasPrefix(b[x:], []byte("xref\n")) {
		t.Fatal("startxref doesn't point at the xref")
	}
	sc := bufio.NewScanner(bytes.NewReader(b[x:]))
	sc.Scan()
	sc.Scan()
	n, _ := strconv.Atoi(strings.Fields(sc.Text())[1])
	sc.Scan() // the free entry
	for i := 1; i < n; i++ {
		sc.Scan()
		off, _ := strconv.Atoi(strings.Fields(sc.Text())[0])
		if want := strconv.Itoa(i) + " 0 obj"; !bytes.HasPrefix(b[off:], []byte(want)) {
			t.Fatalf("xref %d points at %q", i, b[off:off+10])
		}
	}
	// the content stream inflates and has every text
	i := bytes.Index(b, []byte("stream\n")) + len("stream\n")
	j := bytes.Index(b, []byte("\nendstream"))
	r, err := zlib.NewReader(bytes.NewReader(b[i:j]))
	if err != nil {
		t.Fatal(err)
	}
	c, _ := io.ReadAll(r)
	if got := bytes.Count(c, []byte(" Tj ET")); got != len(pg.Texts) {
		t.Errorf("PDF has %d texts, want %d", got, len(pg.Texts))
	}
	if got := bytes.Count(c, []byte(" S\n")); got < len(pg.Paths)-5 {
		t.Errorf("PDF strokes %d of %d paths", got, len(pg.Paths))
	}
}

func checkDXF(t *testing.T, b []byte, pg *Page) {
	t.Helper()
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines)%2 != 0 {
		t.Fatal("DXF: odd number of lines")
	}
	ents := map[string]int{}
	inEnt := false
	for i := 0; i < len(lines); i += 2 {
		code, err := strconv.Atoi(strings.TrimSpace(lines[i]))
		if err != nil {
			t.Fatalf("DXF line %d: group code %q", i+1, lines[i])
		}
		v := lines[i+1]
		if code == 2 && v == "ENTITIES" {
			inEnt = true
		}
		if code == 0 && inEnt {
			ents[v]++
		}
		if code >= 10 && code < 60 {
			if _, err := strconv.ParseFloat(v, 64); err != nil {
				t.Fatalf("DXF line %d: %q isn't a number", i+2, v)
			}
		}
	}
	if lines[len(lines)-1] != "EOF" {
		t.Fatal("DXF doesn't end in EOF")
	}
	if ents["POLYLINE"]+ents["CIRCLE"]+ents["ARC"] < len(pg.Paths)-5 || ents["TEXT"] != len(pg.Texts) || ents["SOLID"] != len(pg.Fills) || ents["POLYLINE"] != ents["SEQEND"] {
		t.Errorf("DXF has %v for %d paths, %d texts, %d fills", ents, len(pg.Paths), len(pg.Texts), len(pg.Fills))
	}
}
