// SPDX-License-Identifier: Apache-2.0

package naming

import (
	"regexp"
	"strings"
)

// The two pluralizers below re-implement, from the rule tables of the pinned
// versions, the pluralization that gorm and bun share (jinzhu/inflection
// v1.0.0) and the one ent uses (go-openapi/inflect v0.19.0). They are
// verified word for word against the recorded "words" fixtures, so a rule that
// is missing shows up as a failing fixture case.

type rewrite struct {
	re   *regexp.Regexp
	repl string
}

func compileRewrites(pairs [][2]string) []rewrite {
	out := make([]rewrite, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, rewrite{regexp.MustCompile(p[0]), p[1]})
	}
	return out
}

// inflectionPlural holds jinzhu/inflection's plural rules in match order:
// uncountables first, then irregulars, then the regular rules last-to-first.
// Only the lowercase variants of each rule are kept because every caller
// passes an already lower-cased snake_case name.
var inflectionPlural = compileRewrites([][2]string{
	{"^(?i)(equipment|information|rice|money|species|series|fish|sheep|jeans|police)$", "${1}"},
	{"person$", "people"},
	{"man$", "men"},
	{"child$", "children"},
	{"sex$", "sexes"},
	{"move$", "moves"},
	{"mombie$", "mombies"},
	{"(quiz)$", "${1}zes"},
	{"^(oxen)$", "${1}"},
	{"^(ox)$", "${1}en"},
	{"^(m|l)ice$", "${1}ice"},
	{"^(m|l)ouse$", "${1}ice"},
	{"(matr|vert|ind)(?:ix|ex)$", "${1}ices"},
	{"(x|ch|ss|sh)$", "${1}es"},
	{"([^aeiouy]|qu)y$", "${1}ies"},
	{"(hive)$", "${1}s"},
	{"(?:([^f])fe|([lr])f)$", "${1}${2}ves"},
	{"sis$", "ses"},
	{"([ti])a$", "${1}a"},
	{"([ti])um$", "${1}a"},
	{"(buffal|tomat)o$", "${1}oes"},
	{"(bu)s$", "${1}ses"},
	{"(alias|status)$", "${1}es"},
	{"(octop|vir)i$", "${1}i"},
	{"(octop|vir)us$", "${1}i"},
	{"^(ax|test)is$", "${1}es"},
	{"s$", "s"},
	{"([a-z])$", "${1}s"},
})

// inflectionPluralize pluralizes the last word of a lowercase name the way
// jinzhu/inflection does.
func inflectionPluralize(s string) string {
	for _, r := range inflectionPlural {
		if r.re.MatchString(s) {
			return r.re.ReplaceAllString(s, r.repl)
		}
	}
	return s
}

// entRule is one go-openapi/inflect plural rule: a case-sensitive suffix, or a
// whole-word match when exact is set.
type entRule struct {
	suffix, repl string
	exact        bool
}

// entPluralRules lists the rules in match order (highest priority first).
var entPluralRules = buildEntRules()

var entUncountable = map[string]bool{
	"equipment": true, "information": true, "rice": true, "money": true,
	"species": true, "series": true, "fish": true, "sheep": true,
	"jeans": true, "police": true,
}

func buildEntRules() []entRule {
	var rules []entRule // insertion order; later entries win
	add := func(suffix, repl string) { rules = append(rules, entRule{suffix, repl, false}) }
	exact := func(suffix, repl string) { rules = append(rules, entRule{suffix, repl, true}) }
	for _, p := range [][2]string{
		{"s", "s"}, {"testis", "testes"}, {"axis", "axes"}, {"octopus", "octopi"},
		{"virus", "viri"}, {"octopi", "octopi"}, {"viri", "viri"},
		{"alias", "aliases"}, {"status", "statuses"}, {"bus", "buses"},
		{"buffalo", "buffaloes"}, {"tomato", "tomatoes"}, {"tum", "ta"},
		{"ium", "ia"}, {"ta", "ta"}, {"ia", "ia"}, {"sis", "ses"},
		{"lf", "lves"}, {"rf", "rves"},
	} {
		add(p[0], p[1])
	}
	// "<letter>fe" becomes "<letter>ves" for every letter except f.
	for c := 'a'; c <= 'z'; c++ {
		if c != 'f' {
			add(string(c)+"fe", string(c)+"ves")
		}
	}
	add("hive", "hives")
	add("quy", "quies")
	// "<consonant>y" becomes "<consonant>ies"; the vowels and y are absent.
	for _, c := range "bcdfghjklmnpqrstvwxz" {
		add(string(c)+"y", string(c)+"ies")
	}
	for _, p := range [][2]string{
		{"x", "xes"}, {"ch", "ches"}, {"ss", "sses"}, {"sh", "shes"},
		{"matrix", "matrices"}, {"vertix", "vertices"}, {"indix", "indices"},
		{"matrex", "matrices"}, {"vertex", "vertices"}, {"index", "indices"},
		{"mouse", "mice"}, {"louse", "lice"}, {"mice", "mice"}, {"lice", "lice"},
	} {
		add(p[0], p[1])
	}
	exact("ox", "oxen")
	exact("oxen", "oxen")
	exact("quiz", "quizzes")
	for _, p := range [][2]string{
		{"person", "people"}, {"man", "men"}, {"child", "children"},
		{"sex", "sexes"}, {"move", "moves"}, {"zombie", "zombies"},
	} {
		add(p[0], p[1])
		add(p[1], p[1])
	}
	// Reverse so the most recently added rule is tried first.
	for i, j := 0, len(rules)-1; i < j; i, j = i+1, j-1 {
		rules[i], rules[j] = rules[j], rules[i]
	}
	return rules
}

// entPluralize pluralizes a Go type name the way ent does. Matching is
// case-sensitive on the Go name, so "Child" is not the irregular "child" and
// becomes "Childs".
func entPluralize(name string) string {
	if name == "" {
		return name
	}
	words := strings.Split(name, " ")
	if entUncountable[strings.ToLower(words[len(words)-1])] {
		return name
	}
	for _, r := range entPluralRules {
		if r.exact {
			if name == r.suffix {
				return r.repl
			}
		} else if strings.HasSuffix(name, r.suffix) {
			return strings.TrimSuffix(name, r.suffix) + r.repl
		}
	}
	return name + "s"
}
