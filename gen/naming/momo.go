// SPDX-License-Identifier: Apache-2.0

package naming

import (
	"strings"

	"github.com/azrtydxb/momo/internal/snake"
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
func ToSnake(s string) string { return snake.ToSnake(s) }

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
