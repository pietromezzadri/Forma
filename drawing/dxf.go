package drawing

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"strings"
)

// WriteDXF writes the page as an AutoCAD R12 DXF in millimetres: a layer
// per pen with its line type (continuous, dashed, centre), circles and
// arcs as CIRCLE and ARC, the rest as polylines, arrowheads as SOLIDs and
// text as TEXT. R12 is the version every CAD and laser cutter reads.
func (pg *Page) WriteDXF(w io.Writer) error {
	b := bufio.NewWriter(w)
	g := func(code int, v string) { fmt.Fprintf(b, "%d\n%s\n", code, v) }
	n := func(code int, x float64) { g(code, trimNum(x, 6)) }
	g(0, "SECTION")
	g(2, "HEADER")
	g(9, "$ACADVER")
	g(1, "AC1009")
	g(9, "$INSUNITS")
	g(70, "4")
	g(9, "$MEASUREMENT")
	g(70, "1")
	g(9, "$EXTMIN")
	n(10, 0)
	n(20, 0)
	g(9, "$EXTMAX")
	n(10, pg.W)
	n(20, pg.H)
	g(9, "$LTSCALE")
	n(40, 1)
	g(0, "ENDSEC")

	g(0, "SECTION")
	g(2, "TABLES")
	g(0, "TABLE")
	g(2, "LTYPE")
	g(70, "3")
	ltype := func(name, desc string, d []float64) {
		g(0, "LTYPE")
		g(2, name)
		g(70, "0")
		g(3, desc)
		g(72, "65")
		g(73, fmt.Sprint(len(d)))
		tot := 0.0
		for _, x := range d {
			tot += x
		}
		n(40, tot)
		for i, x := range d {
			if i%2 == 1 {
				x = -x
			}
			n(49, x)
		}
	}
	ltype("CONTINUOUS", "Solid line", nil)
	ltype("DASHED", "Dashed __ __ __", PenHidden.Dash())
	ltype("CENTER", "Center ____ _ ____", PenCentre.Dash())
	g(0, "ENDTAB")
	g(0, "TABLE")
	g(2, "LAYER")
	g(70, fmt.Sprint(int(nPens)))
	colour := [...]int{7, 2, 3, 1, 8, 1, 1, 7, 7}
	for p := Pen(0); p < nPens; p++ {
		lt := "CONTINUOUS"
		switch p {
		case PenHidden:
			lt = "DASHED"
		case PenCentre, PenCutLine:
			lt = "CENTER"
		}
		g(0, "LAYER")
		g(2, p.Name())
		g(70, "0")
		g(62, fmt.Sprint(colour[p]))
		g(6, lt)
	}
	g(0, "ENDTAB")
	g(0, "ENDSEC")

	g(0, "SECTION")
	g(2, "ENTITIES")
	for _, p := range pg.Paths {
		if len(p.Pts) < 2 {
			continue
		}
		layer := p.Pen.Name()
		switch {
		case p.Arc != nil && p.Closed:
			g(0, "CIRCLE")
			g(8, layer)
			n(10, p.Arc.C.X)
			n(20, p.Arc.C.Y)
			n(40, p.Arc.R)
		case p.Arc != nil:
			a0 := p.Pts[0].Sub(p.Arc.C).Angle() * 180 / math.Pi
			a1 := p.Pts[len(p.Pts)-1].Sub(p.Arc.C).Angle() * 180 / math.Pi
			if !p.CCW {
				a0, a1 = a1, a0 // ARC runs counter-clockwise
			}
			g(0, "ARC")
			g(8, layer)
			n(10, p.Arc.C.X)
			n(20, p.Arc.C.Y)
			n(40, p.Arc.R)
			n(50, normDeg(a0))
			n(51, normDeg(a1))
		default:
			g(0, "POLYLINE")
			g(8, layer)
			g(66, "1")
			flags := 0
			if p.Closed {
				flags = 1
			}
			g(70, fmt.Sprint(flags))
			for _, q := range p.Pts {
				g(0, "VERTEX")
				g(8, layer)
				n(10, q.X)
				n(20, q.Y)
			}
			g(0, "SEQEND")
			g(8, layer)
		}
	}
	for _, f := range pg.Fills {
		if len(f.Pts) < 3 {
			continue
		}
		// SOLID's corners go 1, 2, 4, 3 round; a triangle repeats its last
		c := f.Pts
		q := [4]int{0, 1, 2, 2}
		if len(c) >= 4 {
			q = [4]int{0, 1, 3, 2}
		}
		g(0, "SOLID")
		g(8, PenThin.Name())
		for i, k := range q {
			n(10+i, c[k].X)
			n(20+i, c[k].Y)
		}
	}
	for _, t := range pg.Texts {
		o, _ := t.Origin()
		g(0, "TEXT")
		g(8, PenThin.Name())
		n(10, o.X)
		n(20, o.Y)
		// DXF text height is the capitals' height
		n(40, t.Size*capHeight)
		g(1, dxfString(t.S))
		if math.Abs(t.Angle) > 1e-9 {
			n(50, t.Angle)
		}
	}
	g(0, "ENDSEC")
	g(0, "EOF")
	return b.Flush()
}

func normDeg(a float64) float64 {
	a = math.Mod(a, 360)
	if a < 0 {
		a += 360
	}
	return a
}

// dxfString is s with DXF's control codes for Ø, ° and ±; other
// non-ASCII characters as AutoCAD's \U+ escapes.
func dxfString(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == 'Ø' || r == 'ø':
			b.WriteString("%%c")
		case r == '°':
			b.WriteString("%%d")
		case r == '±':
			b.WriteString("%%p")
		case r >= ' ' && r <= '~':
			b.WriteRune(r)
		default:
			fmt.Fprintf(&b, "\\U+%04X", r)
		}
	}
	return b.String()
}
