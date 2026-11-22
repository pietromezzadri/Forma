package drawing

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"strings"
	"time"
)

// WritePDF writes the page as a one-page vector PDF at full size, its text
// in the base fonts Helvetica and Helvetica-Bold (every reader has them,
// so nothing is embedded).
func (pg *Page) WritePDF(w io.Writer) error {
	const k = 72 / 25.4 // points per mm
	f := func(x float64) string { return trimNum(x*k, 3) }
	var c bytes.Buffer
	c.WriteString("1 J 1 j 0 G 0 g\n")
	// lines grouped by pen, so its width and dash are set once
	for pen := Pen(0); pen < nPens; pen++ {
		started := false
		for _, p := range pg.Paths {
			if p.Pen != pen || len(p.Pts) < 2 {
				continue
			}
			if !started {
				fmt.Fprintf(&c, "%s w ", f(pen.Width()))
				if d := pen.Dash(); d != nil {
					var s []string
					for _, x := range d {
						s = append(s, f(x))
					}
					fmt.Fprintf(&c, "0 J [%s] 0 d\n", strings.Join(s, " "))
				} else {
					c.WriteString("1 J [] 0 d\n")
				}
				started = true
			}
			fmt.Fprintf(&c, "%s %s m", f(p.Pts[0].X), f(p.Pts[0].Y))
			for _, q := range p.Pts[1:] {
				fmt.Fprintf(&c, " %s %s l", f(q.X), f(q.Y))
			}
			if p.Closed {
				c.WriteString(" h")
			}
			c.WriteString(" S\n")
		}
	}
	for _, fl := range pg.Fills {
		if len(fl.Pts) < 3 {
			continue
		}
		fmt.Fprintf(&c, "%s %s m", f(fl.Pts[0].X), f(fl.Pts[0].Y))
		for _, q := range fl.Pts[1:] {
			fmt.Fprintf(&c, " %s %s l", f(q.X), f(q.Y))
		}
		c.WriteString(" h f\n")
	}
	for _, t := range pg.Texts {
		o, d := t.Origin()
		font := "F1"
		if t.Bold {
			font = "F2"
		}
		fmt.Fprintf(&c, "BT /%s %s Tf %s %s %s %s %s %s Tm (%s) Tj ET\n", font, f(t.Size),
			trimNum(d.X, 6), trimNum(d.Y, 6), trimNum(-d.Y, 6), trimNum(d.X, 6), f(o.X), f(o.Y), pdfString(t.S))
	}
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	zw.Write(c.Bytes())
	zw.Close()

	var out bytes.Buffer
	var offs []int
	obj := func(body string) {
		offs = append(offs, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", len(offs), body)
	}
	out.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	obj("<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] /Contents 4 0 R /Resources << /Font << /F1 5 0 R /F2 6 0 R >> >> >>", f(pg.W), f(pg.H)))
	offs = append(offs, out.Len())
	fmt.Fprintf(&out, "4 0 obj\n<< /Length %d /Filter /FlateDecode >>\nstream\n", z.Len())
	out.Write(z.Bytes())
	out.WriteString("\nendstream\nendobj\n")
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
	obj(fmt.Sprintf("<< /Title (%s) /Producer (Forma) /CreationDate (D:%s) >>", pdfString(pg.Title), time.Now().UTC().Format("20060102150405Z")))
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offs)+1)
	for _, o := range offs {
		fmt.Fprintf(&out, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offs)+1, len(offs), xref)
	_, err := w.Write(out.Bytes())
	return err
}

// pdfString is s in WinAnsi with ( ) \ escaped.
func pdfString(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '(' || r == ')' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r >= ' ' && r <= '~':
			b.WriteRune(r)
		case winAnsi[r] != 0:
			fmt.Fprintf(&b, "\\%03o", winAnsi[r])
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}
