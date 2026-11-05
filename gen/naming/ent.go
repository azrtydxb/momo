// SPDX-License-Identifier: Apache-2.0

package naming

import "strings"

// Ent reproduces ent's defaults: snake_case columns and tables from a
// pluralizer with no irregular words (Person becomes persons) that leaves a
// word already ending in "s" alone (Status stays status).
//
// JoinTableName is a real ent default: the owner snake_case followed by the
// edge name, which for a plural edge is the plural target.
type Ent struct{}

// TableName returns the snake_case plural table name.
func (Ent) TableName(goModel string) string {
	s := snakeInitialisms(goModel)
	i := strings.LastIndexByte(s, '_')
	head, w := s[:i+1], s[i+1:]
	return head + entPlural(w)
}

// ColumnName returns the snake_case column name.
func (Ent) ColumnName(goField string) string { return snakeInitialisms(goField) }

// JoinTableName returns owner snake_case followed by the plural target.
func (e Ent) JoinTableName(owner, target string) string {
	return snakeInitialisms(owner) + "_" + e.TableName(target)
}

func entPlural(w string) string {
	if w == "" || strings.HasSuffix(w, "s") {
		return w
	}
	if n := len(w); n >= 2 && w[n-1] == 'y' && !strings.ContainsRune("aeiou", rune(w[n-2])) {
		return w[:n-1] + "ies"
	}
	for _, suf := range []string{"x", "z", "ch", "sh"} {
		if strings.HasSuffix(w, suf) {
			return w + "es"
		}
	}
	return w + "s"
}
