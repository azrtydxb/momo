// SPDX-License-Identifier: Apache-2.0

package schema

import "testing"

// Catches: a table engine found by inspection making a declared schema unequal
// to the database, and two different engines being treated as equal.
func TestEqualTableEngineCountsOnlyWhenBothSet(t *testing.T) {
	mk := func(engine string) *Schema {
		return &Schema{Dialect: "mysql", Tables: []Table{{
			Name: "articles", Engine: engine,
			Columns:    []Column{{Name: "id", Type: "bigint"}},
			PrimaryKey: []string{"id"},
		}}}
	}
	if !Equal(mk(""), mk("MyISAM")) || !Equal(mk("MyISAM"), mk("")) {
		t.Fatal("an engine on one side only must not make the schemas differ")
	}
	if !Equal(mk("MyISAM"), mk("MyISAM")) {
		t.Fatal("the same engine on both sides must be equal")
	}
	if Equal(mk("MyISAM"), mk("Aria")) {
		t.Fatal("two different engines must differ")
	}
}
