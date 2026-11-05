// SPDX-License-Identifier: Apache-2.0

package naming

import (
	"strings"
	"unicode"
)

// Ent reproduces ent's defaults for a Go name: the table is snake(pluralize
// (type name)) with go-openapi/inflect's case-sensitive suffix rules, so Child
// becomes childs, Mouse becomes mouses and Person becomes persons, and the
// column is snake(field name). Ent's snake has no initialism list, so HTTPURL
// is httpurl. Both are checked against a recorded word list. Names are assumed
// ASCII.
//
// In a real ent project the column is the schema field's own name (the
// fixture's http_url is the schema's "http_url"), which a Go name cannot
// recover; this preset answers for a Go identifier run through ent's snake.
//
// JoinTableName is a real ent default: the owner snake_case followed by the
// edge name, which for a plural edge is the plural target.
type Ent struct{}

// TableName returns ent's table name for a type name.
func (Ent) TableName(goModel string) string { return entSnake(entPluralize(goModel)) }

// ColumnName returns ent's column name for a field name.
func (Ent) ColumnName(goField string) string { return entSnake(goField) }

// JoinTableName returns owner snake_case followed by the plural target.
func (e Ent) JoinTableName(owner, target string) string {
	return entSnake(owner) + "_" + e.TableName(target)
}

// entSnake is ent's snake: an underscore goes before an upper-case letter
// that follows a lower-case one, or that precedes a lower-case letter after a
// letter that did not itself just start a word.
func entSnake(s string) string {
	var (
		j int
		b strings.Builder
	)
	for i := 0; i < len(s); i++ {
		r := rune(s[i])
		if i > 0 && i < len(s)-1 && unicode.IsUpper(r) {
			if unicode.IsLower(rune(s[i-1])) ||
				j != i-1 && unicode.IsLower(rune(s[i+1])) && unicode.IsLetter(rune(s[i-1])) {
				j = i
				b.WriteString("_")
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
