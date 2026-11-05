// SPDX-License-Identifier: Apache-2.0

package naming

import (
	"strings"
	"unicode"
)

// Bun reproduces bun's default naming: plural snake_case tables and columns
// where an underscore is inserted only at a lower-to-upper boundary, so an
// acronym run is not split (HTTPURL becomes httpurl).
//
// JoinTableName: bun has no default many-to-many table name; the recorded
// fixture value echoes a m2m tag the recorder set explicitly. The rule here
// (owner + "_" + plural target) is a convention chosen to match that value,
// not a bun default.
type Bun struct{}

// TableName returns the plural bun-style snake_case table name.
func (Bun) TableName(goModel string) string { return Plural(bunSnake(goModel)) }

// ColumnName returns the bun-style snake_case column name.
func (Bun) ColumnName(goField string) string { return bunSnake(goField) }

// JoinTableName returns owner followed by the plural target.
func (Bun) JoinTableName(owner, target string) string {
	return bunSnake(owner) + "_" + Plural(bunSnake(target))
}

func bunSnake(s string) string {
	rs := []rune(s)
	var b strings.Builder
	for i, r := range rs {
		if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(rs[i-1]) || unicode.IsDigit(rs[i-1])) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
