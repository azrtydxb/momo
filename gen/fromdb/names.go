// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// initialisms are written in capitals inside a Go name (UserID, not UserId).
var initialisms = map[string]string{
	"id": "ID", "url": "URL", "uri": "URI", "uuid": "UUID", "api": "API",
	"http": "HTTP", "https": "HTTPS", "ip": "IP", "json": "JSON", "sql": "SQL",
	"html": "HTML", "xml": "XML", "tls": "TLS", "ssh": "SSH", "cpu": "CPU",
}

// splitWords splits a database identifier into words on separators and on
// camel case boundaries, so user_id, user-id, userId and UserID give the
// same two words.
func splitWords(s string) []string {
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, string(cur))
			cur = nil
		}
	}
	rs := []rune(s)
	for i, r := range rs {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			flush()
			continue
		}
		if len(cur) > 0 && unicode.IsUpper(r) {
			prev := rs[i-1]
			nextLower := i+1 < len(rs) && unicode.IsLower(rs[i+1])
			if unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && nextLower) {
				flush()
			}
		}
		cur = append(cur, r)
	}
	flush()
	return words
}

// titleWord capitalizes one word, keeping an initialism in capitals.
func titleWord(w string) string {
	lower := strings.ToLower(w)
	if v, ok := initialisms[lower]; ok {
		return v
	}
	r, n := utf8.DecodeRuneInString(lower)
	return string(unicode.ToUpper(r)) + lower[n:]
}

// pascal joins words into an exported Go name. An empty result becomes
// fallback, and a leading digit gets an X in front.
func pascal(words []string, fallback string) string {
	var b strings.Builder
	for _, w := range words {
		b.WriteString(titleWord(w))
	}
	out := b.String()
	if out == "" {
		return fallback
	}
	if r, _ := utf8.DecodeRuneInString(out); unicode.IsDigit(r) {
		out = "X" + out
	}
	return out
}

var irregularSingular = map[string]string{
	"people": "person", "children": "child", "men": "man", "women": "woman", "mice": "mouse",
}

// singulars returns the candidate singular forms of a lower-case word, the
// most likely first. The caller picks the one whose plural the naming preset
// gives back, so a wrong guess costs nothing.
func singulars(w string) []string {
	var out []string
	if s, ok := irregularSingular[w]; ok {
		out = append(out, s)
	}
	if strings.HasSuffix(w, "ies") && len(w) > 3 {
		out = append(out, w[:len(w)-3]+"y")
	}
	if strings.HasSuffix(w, "es") && len(w) > 2 {
		out = append(out, w[:len(w)-2])
	}
	if strings.HasSuffix(w, "s") && len(w) > 1 {
		out = append(out, w[:len(w)-1])
	}
	return append(out, w)
}
