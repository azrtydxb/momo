// SPDX-License-Identifier: Apache-2.0

// Package pluralize holds the English pluralization of the default momo
// naming preset: a handful of rules and irregular words applied to the last
// underscore-separated word of a snake_case name.
package pluralize

import "strings"

var irregular = map[string]string{
	"person": "people",
	"child":  "children",
	"man":    "men",
	"woman":  "women",
	"mouse":  "mice",
}

// Plural pluralizes the last underscore-separated word of s.
func Plural(s string) string {
	i := strings.LastIndexByte(s, '_')
	head, word := s[:i+1], s[i+1:]
	return head + pluralWord(word)
}

func pluralWord(w string) string {
	if w == "" {
		return w
	}
	if p, ok := irregular[w]; ok {
		return p
	}
	if n := len(w); n >= 2 && w[n-1] == 'y' && !strings.ContainsRune("aeiou", rune(w[n-2])) {
		return w[:n-1] + "ies"
	}
	for _, suf := range []string{"s", "x", "z", "ch", "sh"} {
		if strings.HasSuffix(w, suf) {
			return w + "es"
		}
	}
	return w + "s"
}
