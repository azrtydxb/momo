// SPDX-License-Identifier: Apache-2.0

// Package naming maps Go model and field names to SQL identifiers through
// named presets.
package naming

import "fmt"

// Namer derives SQL identifiers from Go names.
type Namer interface {
	// TableName returns the table name for a Go model type name.
	TableName(goModel string) string
	// ColumnName returns the column name for a Go field name.
	ColumnName(goField string) string
	// JoinTableName returns the many-to-many join table for two models.
	JoinTableName(owner, target string) string
}

// ByName returns the preset with the given name.
func ByName(name string) (Namer, error) {
	switch name {
	case "momo":
		return Momo{}, nil
	case "gorm":
		return GORM{}, nil
	case "bun":
		return Bun{}, nil
	case "ent":
		return Ent{}, nil
	}
	return nil, fmt.Errorf("unknown naming preset %q", name)
}
