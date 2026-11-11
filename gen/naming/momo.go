// SPDX-License-Identifier: Apache-2.0

package naming

import (
	"strings"
	"unicode"
)

// Momo is the default preset: snake_case columns and plural snake_case tables.
type Momo struct{}

// TableName returns the plural snake_case table name.
func (Momo) TableName(goModel string) string { return Plural(ToSnake(goModel)) }

// ColumnName returns the snake_case column name.
func (Momo) ColumnName(goField string) string { return ToSnake(goField) }

// JoinTableName joins the two table names with an underscore.
func (m Momo) JoinTableName(owner, target string) string {
	return m.TableName(owner) + "_" + m.TableName(target)
}

// ToSnake converts a Go identifier to snake_case, keeping acronyms together.
// A plural s after an initialism of the word list belongs to it: RoleIDs is
// role_ids and URLs is urls, not role_i_ds and ur_ls.
func ToSnake(s string) string {
	rs := []rune(s)
	noSplit, forceSplit := pluralInitialisms(rs)
	var b strings.Builder
	for i, r := range rs {
		if unicode.IsUpper(r) && i > 0 {
			prev := rs[i-1]
			split := unicode.IsLower(prev) || unicode.IsDigit(prev) ||
				(unicode.IsUpper(prev) && i+1 < len(rs) && unicode.IsLower(rs[i+1]))
			if (split && !noSplit[i]) || forceSplit[i] {
				b.WriteByte('_')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// pluralInitialisms finds the upper-case runs that end in an initialism of the
// word list followed by a lone plural s (the "IDs" of RoleIDs). noSplit marks
// the last letter of such a run, which the usual rule would split off before
// the s, and forceSplit the first letter of the initialism when the run has
// letters before it (the "ID" of HTTPIDs). The s must end the name or be
// followed by something that is not a lower-case letter, so "IDsx" is not a
// plural.
func pluralInitialisms(rs []rune) (noSplit, forceSplit []bool) {
	noSplit = make([]bool, len(rs))
	forceSplit = make([]bool, len(rs))
	for j := 0; j < len(rs); j++ {
		if !unicode.IsUpper(rs[j]) {
			continue
		}
		e := j
		for e+1 < len(rs) && unicode.IsUpper(rs[e+1]) {
			e++
		}
		if e+1 < len(rs) && rs[e+1] == 's' && (e+2 >= len(rs) || !unicode.IsLower(rs[e+2])) {
			if n := initialismSuffix(rs[j : e+1]); n > 0 {
				noSplit[e] = true
				if k := e + 1 - n; k > j {
					forceSplit[k] = true
				}
			}
		}
		j = e
	}
	return noSplit, forceSplit
}

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
