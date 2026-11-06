// SPDX-License-Identifier: Apache-2.0

package naming

import "strings"

// Bun reproduces bun's default naming: tables are the plural (jinzhu/
// inflection rules, the same as gorm) of the underscored type name, and an
// underscore goes before an upper-case letter only when a neighbour is lower
// case, so an acronym run is not split (HTTPURL becomes httpurl, IDs becomes
// i_ds). Checked against a recorded word list. Names are assumed ASCII.
//
// JoinTableName: bun has no default many-to-many table name; the recorded
// fixture value echoes a m2m tag the recorder set explicitly. The rule here
// (owner + "_" + plural target) is a convention chosen to match that value,
// not a bun default.
type Bun struct{}

// TableName returns the plural bun-style snake_case table name.
func (Bun) TableName(goModel string) string { return inflectionPluralize(bunSnake(goModel)) }

// ColumnName returns the bun-style snake_case column name.
func (Bun) ColumnName(goField string) string { return bunSnake(goField) }

// JoinTableName returns owner followed by the plural target.
func (b Bun) JoinTableName(owner, target string) string {
	return bunSnake(owner) + "_" + b.TableName(target)
}

func bunSnake(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !isUpperByte(c) {
			b.WriteByte(c)
			continue
		}
		if i > 0 && i+1 < len(s) && (isLowerByte(s[i-1]) || isLowerByte(s[i+1])) {
			b.WriteByte('_')
		}
		b.WriteByte(c + 32)
	}
	return b.String()
}

func isLowerByte(c byte) bool { return c >= 'a' && c <= 'z' }
