// SPDX-License-Identifier: Apache-2.0

package gendocs

import (
	"strings"
	"unicode/utf8"
)

// table renders a Markdown table the way the repository's Markdown formatter
// writes it: cells padded to the widest cell of the column, a separator row of
// dashes as wide as the column. Cells are ASCII and hold no pipe character.
func table(header []string, rows [][]string) string {
	width := make([]int, len(header))
	for i, h := range header {
		width[i] = max(3, utf8.RuneCountInString(h))
	}
	for _, r := range rows {
		for i, c := range r {
			width[i] = max(width[i], utf8.RuneCountInString(c))
		}
	}
	line := func(cells []string) string {
		var b strings.Builder
		b.WriteString("|")
		for i, c := range cells {
			b.WriteString(" ")
			b.WriteString(c)
			b.WriteString(strings.Repeat(" ", width[i]-utf8.RuneCountInString(c)))
			b.WriteString(" |")
		}
		return b.String()
	}
	sep := make([]string, len(header))
	for i := range sep {
		sep[i] = strings.Repeat("-", width[i])
	}
	out := []string{line(header), line(sep)}
	for _, r := range rows {
		out = append(out, line(r))
	}
	return strings.Join(out, "\n")
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func code(s string) string { return "`" + s + "`" }

func codes(list []string) string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = code(s)
	}
	return strings.Join(out, ", ")
}
