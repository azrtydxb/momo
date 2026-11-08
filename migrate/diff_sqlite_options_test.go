// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// TestDiffSQLiteTableOptions checks that a table which has WITHOUT ROWID, STRICT
// or AUTOINCREMENT in the database and not in the model is not an error (a model
// cannot declare them), while asking for one the table lacks still is.
func TestDiffSQLiteTableOptions(t *testing.T) {
	mk := func(set func(*schema.Table)) *schema.Schema {
		tbl := schema.Table{Name: "t", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}}, PrimaryKey: []string{"id"}}
		if set != nil {
			set(&tbl)
		}
		return &schema.Schema{Dialect: dialect.SQLite, Tables: []schema.Table{tbl}}
	}
	opts := map[string]func(*schema.Table){
		"WITHOUT ROWID": func(t *schema.Table) { t.WithoutRowID = true },
		"STRICT":        func(t *schema.Table) { t.Strict = true },
		"AUTOINCREMENT": func(t *schema.Table) { t.Autoincrement = true },
	}
	for name, set := range opts {
		changes, _, err := Diff(mk(set), mk(nil), DiffOptions{})
		if err != nil || len(changes) != 0 {
			t.Errorf("%s in the database only: want no error and no change, got %v %v", name, changes, err)
		}
		if _, _, err := Diff(mk(nil), mk(set), DiffOptions{}); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s asked for but missing: want an error naming it, got %v", name, err)
		}
	}
}
