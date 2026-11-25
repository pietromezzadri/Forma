package drawing

import (
	"math"
	"strings"

	"forma/geom"
)

// Hole callouts: a leader from a hole's rim to a shoulder carrying the
// hole's description, one line above the shoulder and the rest under it
// (ISO 15786 / ASME Y14.5 style): "2× Ø5.5 THRU", "{cbore} Ø10 {depth} 5.4".
// The depth, counterbore and countersink symbols aren't in the drawing
// font, so {depth}, {cbore} and {csink} in a line are drawn as lines.

// callout of the anchor's hole: its body's Callout by either face of it.
func (b *Built) callout(a Anchor) []string {
	for _, bd := range b.Proj.Bodies {
		if bd.Key != a.Body || bd.Callout == nil {
			continue
		}
		for _, f := range []string{a.A, a.B} {
			if f == "" {
				continue
			}
			if l := bd.Callout(f); len(l) > 0 {
				return l
			}
		}
	}
	return nil
}

// callout draws a hole callout for the circle at c (radius r) with its
// shoulder at q.
func callout(pg *Page, c geom.Vec2, r float64, q geom.Vec2, lines []string) {
	w := q.Sub(c)
	if w.Len() < 1e-9 {
		w = geom.V2(1, 1)
	}
	w = w.Norm()
	rim := c.Add(w.Scale(r))
	pg.line(PenThin, rim, q)
	arrow(pg, rim, w.Scale(-1), arrowLen)
	width := 0.0
	for _, l := range lines {
		width = math.Max(width, richWidth(l, dimText))
	}
	side := 1.0
	if w.X < 0 {
		side = -1
	}
	pg.line(PenThin, q, q.Add(geom.V2(side*(width+2), 0)))
	for i, l := range lines {
		y := q.Y + 1
		if i > 0 {
			y = q.Y - 1 - dimText*capHeight - float64(i-1)*dimText*1.5
		}
		x := q.X + 1
		if side < 0 {
			x = q.X - 1 - richWidth(l, dimText)
		}
		richText(pg, geom.V2(x, y), dimText, l)
	}
}

// the symbols' tokens and their widths (in ems)
var symbols = map[string]float64{"{depth}": 0.62, "{cbore}": 0.78, "{csink}": 0.78}

// runs splits a line into text and symbol tokens.
func runs(s string) []string {
	var out []string
	for s != "" {
		i := strings.IndexByte(s, '{')
		if i < 0 {
			return append(out, s)
		}
		tok := ""
		for t := range symbols {
			if strings.HasPrefix(s[i:], t) {
				tok = t
			}
		}
		if tok == "" {
			out = append(out, s[:i+1])
			s = s[i+1:]
			continue
		}
		if i > 0 {
			out = append(out, s[:i])
		}
		out = append(out, tok)
		s = s[i+len(tok):]
	}
	return out
}

// richWidth is how long a line with symbols runs at size.
func richWidth(s string, size float64) float64 {
	w := 0.0
	for _, r := range runs(s) {
		if e, ok := symbols[r]; ok {
			w += e * size
		} else {
			w += TextWidth(r, size, false)
		}
	}
	return w
}

// richText writes a line with symbols, left aligned, its baseline at at.
func richText(pg *Page, at geom.Vec2, size float64, s string) {
	x := at.X
	h := size * capHeight
	for _, r := range runs(s) {
		e, ok := symbols[r]
		if !ok {
			if strings.TrimSpace(r) != "" {
				pg.text(Text{At: geom.V2(x, at.Y), Size: size, S: r})
			}
			x += TextWidth(r, size, false)
			continue
		}
		sw := e * size
		m := sw * 0.12 // side bearing
		x0, x1, y0, y1 := x+m, x+sw-m, at.Y, at.Y+h
		xm := (x0 + x1) / 2
		switch r {
		case "{depth}": // ↧: a stem down onto a bar, an arrowhead on it
			pg.line(PenThin, geom.V2(xm, y1), geom.V2(xm, y0))
			pg.line(PenThin, geom.V2(x0, y0), geom.V2(x1, y0))
			pg.line(PenThin, geom.V2(x0+0.15*sw, y0+0.38*h), geom.V2(xm, y0), geom.V2(x1-0.15*sw, y0+0.38*h))
		case "{cbore}": // ⌴: a square-bottomed cup
			pg.line(PenThin, geom.V2(x0, y1), geom.V2(x0, y0), geom.V2(x1, y0), geom.V2(x1, y1))
		case "{csink}": // ⌵: a V
			pg.line(PenThin, geom.V2(x0, y1), geom.V2(xm, y0), geom.V2(x1, y1))
		}
		x += sw
	}
}
