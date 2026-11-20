// SPDX-License-Identifier: Apache-2.0

package naming

import (
	"github.com/azrtydxb/momo/internal/pluralize"
	"github.com/azrtydxb/momo/internal/snake"
)

// Momo is the default preset: snake_case columns and plural snake_case tables.
type Momo struct{}

// TableName returns the plural snake_case table name.
func (Momo) TableName(goModel string) string { return pluralize.Plural(snake.ToSnake(goModel)) }

// ColumnName returns the snake_case column name.
func (Momo) ColumnName(goField string) string { return snake.ToSnake(goField) }

// JoinTableName joins the two table names with an underscore.
func (m Momo) JoinTableName(owner, target string) string {
	return m.TableName(owner) + "_" + m.TableName(target)
}

// ToSnake converts a Go identifier to snake_case, keeping acronyms together.
//
// Deprecated: use Momo.ColumnName. Removed in the next release.
func ToSnake(s string) string { return snake.ToSnake(s) }

// Plural pluralizes the last underscore-separated word of s.
//
// Deprecated: use Momo.TableName. Removed in the next release.
func Plural(s string) string { return pluralize.Plural(s) }
