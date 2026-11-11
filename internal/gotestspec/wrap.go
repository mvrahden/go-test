package gotestspec

import (
	"strings"
	"unicode/utf8"
)

// WithWidth wraps the terminal spec's labels at cols columns; 0 does not wrap.
func WithWidth(cols int) RenderOption {
	return func(c *renderConfig) { c.width = cols }
}

// wrapLabel breaks text at spaces so each line fits width-indent columns,
// where lines after the first start at hang and the deeper of the two leaves
// less room. A word longer than the room stays whole on its own line. Runes
// count one column each; a width of 0 or less does not wrap.
func wrapLabel(text string, indent, hang, width int) []string {
	words := strings.Fields(text)
	if width <= 0 || len(words) == 0 {
		return []string{text}
	}
	room := max(width-max(indent, hang), 1)
	var lines []string
	line, n := "", 0
	for _, w := range words {
		wn := utf8.RuneCountInString(w)
		switch {
		case line == "":
			line, n = w, wn
		case n+1+wn <= room:
			line, n = line+" "+w, n+1+wn
		default:
			lines = append(lines, line)
			line, n = w, wn
		}
	}
	return append(lines, line)
}
