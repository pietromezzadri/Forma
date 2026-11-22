package drawing

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"strings"

	"forma/geom"
)

// WriteSVG writes the page as SVG in millimetres: lines in classes by pen
// (named as the DXF layers), true circles and arcs where the page knows
// them, text in Helvetica / Arial.
func (pg *Page) WriteSVG(w io.Writer) error {
	b := bufio.NewWriter(w)
	f := func(x float64) string { return trimNum(x, 3) }
	fmt.Fprintf(b, `<?xml version="1.0" encoding="UTF-8"?>`+"\n")
	fmt.Fprintf(b, `<svg xmlns="http://www.w3.org/2000/svg" width="%smm" height="%smm" viewBox="0 0 %s %s">`+"\n", f(pg.W), f(pg.H), f(pg.W), f(pg.H))
	if pg.Title != "" {
		fmt.Fprintf(b, "<title>%s</title>\n", xmlEscape(pg.Title))
	}
	b.WriteString("<style>\n")
	b.WriteString("path,circle,polyline,polygon{fill:none;stroke:#000;stroke-linecap:round;stroke-linejoin:round}\n")
	for p := Pen(0); p < nPens; p++ {
		fmt.Fprintf(b, ".%s{stroke-width:%s", p.Name(), f(p.Width()))
		if d := p.Dash(); d != nil {
			var s []string
			for _, x := range d {
				s = append(s, f(x))
			}
			fmt.Fprintf(b, ";stroke-dasharray:%s;stroke-linecap:butt", strings.Join(s, " "))
		}
		b.WriteString("}\n")
	}
	b.WriteString(".fill{fill:#000;stroke:none}\ntext{font-family:Helvetica,Arial,sans-serif;fill:#000}\n</style>\n")
	fmt.Fprintf(b, `<rect width="%s" height="%s" fill="#fff"/>`+"\n", f(pg.W), f(pg.H))
	y := func(v float64) float64 { return pg.H - v }
	pt := func(p geom.Vec2) string { return f(p.X) + " " + f(y(p.Y)) }
	for _, p := range pg.Paths {
		if len(p.Pts) < 2 {
			continue
		}
		cls := p.Pen.Name()
		if p.Arc != nil && p.Closed {
			fmt.Fprintf(b, `<circle class="%s" cx="%s" cy="%s" r="%s"/>`+"\n", cls, f(p.Arc.C.X), f(y(p.Arc.C.Y)), f(p.Arc.R))
			continue
		}
		var d strings.Builder
		d.WriteString("M" + pt(p.Pts[0]))
		if p.Arc != nil {
			a0 := p.Pts[0].Sub(p.Arc.C).Angle()
			a1 := p.Pts[len(p.Pts)-1].Sub(p.Arc.C).Angle()
			sweep := a1 - a0
			if p.CCW {
				for sweep <= 0 {
					sweep += 2 * math.Pi
				}
			} else {
				for sweep >= 0 {
					sweep -= 2 * math.Pi
				}
			}
			large, sw := 0, 0
			if math.Abs(sweep) > math.Pi {
				large = 1
			}
			if p.CCW {
				sw = 1 // counter-clockwise with y up is clockwise with y down
			}
			fmt.Fprintf(&d, "A%s %s 0 %d %d %s", f(p.Arc.R), f(p.Arc.R), large, sw, pt(p.Pts[len(p.Pts)-1]))
		} else {
			for _, q := range p.Pts[1:] {
				d.WriteString("L" + pt(q))
			}
			if p.Closed {
				d.WriteString("Z")
			}
		}
		fmt.Fprintf(b, `<path class="%s" d="%s"/>`+"\n", cls, d.String())
	}
	for _, fl := range pg.Fills {
		var s []string
		for _, q := range fl.Pts {
			s = append(s, f(q.X)+","+f(y(q.Y)))
		}
		fmt.Fprintf(b, `<polygon class="fill" points="%s"/>`+"\n", strings.Join(s, " "))
	}
	for _, t := range pg.Texts {
		o, _ := t.Origin()
		weight := ""
		if t.Bold {
			weight = ` font-weight="bold"`
		}
		rot := ""
		if math.Abs(t.Angle) > 1e-9 {
			rot = fmt.Sprintf(` transform="rotate(%s %s %s)"`, f(-t.Angle), f(o.X), f(y(o.Y)))
		}
		fmt.Fprintf(b, `<text x="%s" y="%s" font-size="%s"%s%s>%s</text>`+"\n", f(o.X), f(y(o.Y)), f(t.Size), weight, rot, xmlEscape(t.S))
	}
	b.WriteString("</svg>\n")
	return b.Flush()
}

func trimNum(x float64, places int) string {
	s := fmt.Sprintf("%.*f", places, x)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	if s == "-0" {
		return "0"
	}
	return s
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
