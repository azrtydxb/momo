// SPDX-License-Identifier: Apache-2.0

package naming

// GORM reproduces gorm's default NamingStrategy: snake_case columns that keep
// acronyms together (HTTPURL becomes http_url) and plural snake_case tables.
//
// JoinTableName: gorm has no default many-to-many table name; the recorded
// fixture value echoes a many2many tag the recorder set explicitly. The rule
// here (owner snake_case + "_" + plural target) is a convention chosen to
// match that value, not a gorm default.
type GORM struct{}

// TableName returns the plural snake_case table name.
func (GORM) TableName(goModel string) string { return Plural(snakeInitialisms(goModel)) }

// ColumnName returns the snake_case column name.
func (GORM) ColumnName(goField string) string { return snakeInitialisms(goField) }

// JoinTableName returns owner snake_case followed by the plural target.
func (GORM) JoinTableName(owner, target string) string {
	return snakeInitialisms(owner) + "_" + Plural(snakeInitialisms(target))
}
