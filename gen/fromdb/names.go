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
	s = strings.ToValidUTF8(s, "")
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
	return exported(out)
}

// exported prefixes an X when the name does not start with an upper-case
// letter (a digit, or a letter such as a CJK character or ß that has no
// upper case), because Go exports only names that do.
func exported(name string) string {
	if r, _ := utf8.DecodeRuneInString(name); !unicode.IsUpper(r) {
		return "X" + name
	}
	return name
}

// irregularSingular lists plurals no spelling rule gets right: the irregular
// ones, and the -ses, -oes and -ches words that look like another group
// (alias and release both end in -ases/-eases, tomatoes and shoes in -oes).
// A listed word is tried first; the caller still keeps it only if the naming
// preset pluralizes it back.
var irregularSingular = map[string]string{
	"people": "person", "children": "child", "men": "man", "women": "woman", "mice": "mouse",
	"quizzes": "quiz", "uses": "use",
	"aliases": "alias", "biases": "bias", "canvases": "canvas", "gases": "gas",
	"atlases": "atlas", "lenses": "lens", "irises": "iris",
	"tomatoes": "tomato", "buffaloes": "buffalo", "potatoes": "potato",
	"heroes": "hero", "echoes": "echo", "vetoes": "veto",
	"caches": "cache", "niches": "niche",
}

// singularUs lists the singular words in -us, whose stem keeps its s (status,
// not Statu). A plural in -us, such as menus, is not listed and loses its s.
var singularUs = map[string]bool{
	"status": true, "bus": true, "campus": true, "virus": true, "bonus": true,
	"corpus": true, "focus": true, "census": true, "genus": true,
	"radius": true, "apparatus": true, "consensus": true, "surplus": true,
	"prospectus": true, "syllabus": true, "chorus": true, "circus": true,
	"cactus": true, "octopus": true, "nexus": true,
}

// needsEs reports whether a word ending in "es" takes that ending in full, so
// the -es strip is right: -sses, -xes, -zzes, -ches, -shes, and -uses
// (statuses, buses) except -ouses and -auses (houses, causes). Any other
// "-es" word, such as releases, cases, sizes or courses, loses only its "s".
// A -zes word that is not a doubled z, such as waltzes, becomes Waltze.
func needsEs(w string) bool {
	stem := strings.TrimSuffix(w, "es")
	for _, suf := range []string{"ss", "x", "zz", "ch", "sh"} {
		if strings.HasSuffix(stem, suf) {
			return true
		}
	}
	return strings.HasSuffix(w, "uses") &&
		!strings.HasSuffix(w, "ouses") && !strings.HasSuffix(w, "auses")
}

// singulars returns the candidate singular forms of a lower-case word, the
// most likely first. The caller picks the one whose plural the naming preset
// gives back, so the order has to be right on its own: a preset that
// pluralizes both "releas" and "release" to "releases" keeps the first.
func singulars(w string) []string {
	var out []string
	if s, ok := irregularSingular[w]; ok {
		out = append(out, s)
	}
	if strings.HasSuffix(w, "ss") || singularUs[w] {
		// A word in -ss (class, address, process) is already singular: no
		// plural ends in a lone s after another s, so stripping it would
		// give Clas. The -us words below are listed because menus and
		// emus are plurals that the same shape cannot tell apart.
		return append(out, w)
	}
	if strings.HasSuffix(w, "ies") && len(w) > 3 {
		out = append(out, w[:len(w)-3]+"y")
	}
	if !strings.HasSuffix(w, "s") || len(w) < 2 {
		return append(out, w)
	}
	strip := w[:len(w)-1]
	if !strings.HasSuffix(w, "es") || len(w) < 3 {
		return append(out, strip, w)
	}
	es := w[:len(w)-2]
	if needsEs(w) {
		return append(out, es, strip, w)
	}
	return append(out, strip, es, w)
}
