package shell

import (
	"strings"
)

// wrapDetail bounds each row in runes and adds a visible truncation marker.
// Drawing and scrolling keep all resulting text inside the shared row rect.
func wrapDetail(text string, columns, maxLines int) []string {
	if columns < 1 || maxLines < 1 {
		return nil
	}
	var lines []string
	var line []rune
	for _, word := range strings.Fields(text) {
		runes := []rune(word)
		if len(line) > 0 && len(line)+1+len(runes) > columns {
			lines = append(lines, string(line))
			line = nil
		}
		for len(runes) > columns {
			lines = append(lines, string(runes[:columns]))
			runes = runes[columns:]
		}
		if len(line) > 0 {
			line = append(line, ' ')
		}
		line = append(line, runes...)
	}
	if len(line) > 0 {
		lines = append(lines, string(line))
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		last := []rune(lines[maxLines-1])
		if len(last) > max(0, columns-3) {
			last = last[:max(0, columns-3)]
		}
		lines[maxLines-1] = string(last) + strings.Repeat(".", min(columns, 3))
	}
	return lines
}
