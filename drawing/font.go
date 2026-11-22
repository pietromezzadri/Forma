package drawing

// Text is laid out with Helvetica's metrics (the PDF base font every reader
// has; SVG asks for Helvetica or Arial, which share them), so text sits
// where it was meant to in every writer.

const capHeight = 0.718 // Helvetica's capitals, in ems

// helvetica and helveticaBold are the advance widths of ' ' … '~' in
// thousandths of an em (Adobe's AFM files).
var helvetica = [95]int{
	278, 278, 355, 556, 556, 889, 667, 191, 333, 333, 389, 584, 278, 333, 278, 278,
	556, 556, 556, 556, 556, 556, 556, 556, 556, 556, 278, 278, 584, 584, 584, 556,
	1015, 667, 667, 722, 722, 667, 611, 778, 722, 278, 500, 667, 556, 833, 722, 778,
	667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, 278, 278, 278, 469, 556,
	333, 556, 556, 500, 556, 556, 278, 556, 556, 222, 222, 500, 222, 833, 556, 556,
	556, 556, 333, 500, 278, 556, 500, 722, 500, 500, 500, 334, 260, 334, 584,
}

var helveticaBold = [95]int{
	278, 333, 474, 556, 556, 889, 722, 238, 333, 333, 389, 584, 278, 333, 278, 278,
	556, 556, 556, 556, 556, 556, 556, 556, 556, 556, 333, 333, 584, 584, 584, 611,
	975, 722, 722, 722, 722, 667, 611, 778, 722, 278, 556, 722, 611, 833, 722, 778,
	667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, 333, 278, 333, 584, 556,
	333, 556, 611, 556, 611, 556, 333, 611, 611, 278, 278, 556, 278, 889, 611, 611,
	611, 611, 389, 556, 333, 611, 556, 778, 556, 556, 500, 389, 280, 389, 584,
}

// winAnsi is the WinAnsi code of the few non-ASCII characters drawings use.
var winAnsi = map[rune]byte{
	'Ø': 0xD8, 'ø': 0xF8, '°': 0xB0, '±': 0xB1, '×': 0xD7, 'µ': 0xB5, '²': 0xB2, '³': 0xB3,
	'–': 0x96, '—': 0x97, '·': 0xB7, '©': 0xA9, 'é': 0xE9, 'è': 0xE8, 'á': 0xE1, 'ã': 0xE3,
	'ç': 0xE7, 'ó': 0xF3, 'ö': 0xF6, 'ü': 0xFC, 'ä': 0xE4, 'í': 0xED, 'ú': 0xFA, 'õ': 0xF5,
}

var extraWidth = map[rune]int{
	'Ø': 778, 'ø': 611, '°': 400, '±': 584, '×': 584, 'µ': 556, '²': 333, '³': 333,
	'–': 556, '—': 1000, '·': 278, '©': 737,
}

// TextWidth is how long s runs at size (mm).
func TextWidth(s string, size float64, bold bool) float64 {
	tab := &helvetica
	if bold {
		tab = &helveticaBold
	}
	w := 0
	for _, r := range s {
		switch {
		case r >= ' ' && r <= '~':
			w += tab[r-' ']
		case extraWidth[r] != 0:
			w += extraWidth[r]
		default:
			w += 556
		}
	}
	return float64(w) / 1000 * size
}
