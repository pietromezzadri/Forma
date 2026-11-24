package doc

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseColor reads a "#rrggbb" colour as red, green and blue from 0 to 1.
func ParseColor(s string) ([3]float64, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return [3]float64{}, false
	}
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return [3]float64{}, false
	}
	return [3]float64{float64(n>>16&255) / 255, float64(n>>8&255) / 255, float64(n&255) / 255}, true
}

// FormatColor writes red, green and blue from 0 to 1 as "#rrggbb".
func FormatColor(c [3]float64) string {
	b := func(x float64) int {
		v := int(x*255 + 0.5)
		return max(0, min(255, v))
	}
	return fmt.Sprintf("#%02x%02x%02x", b(c[0]), b(c[1]), b(c[2]))
}
