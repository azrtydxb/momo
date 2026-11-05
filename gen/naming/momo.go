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
func ToSnake(s string) string {
	rs := []rune(s)
	var b strings.Builder
	for i, r := range rs {
		if unicode.IsUpper(r) && i > 0 {
			prev := rs[i-1]
			if unicode.IsLower(prev) || unicode.IsDigit(prev) ||
				(unicode.IsUpper(prev) && i+1 < len(rs) && unicode.IsLower(rs[i+1])) {
				b.WriteByte('_')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
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
