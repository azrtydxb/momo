// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// stubCheckIn stands in for a value-list check that main does not have: it
// renders the list as c.Check does, one expression with the values in the
// order they were written.
func stubCheckIn(values ...string) *schema.Schema {
	lits := make([]string, len(values))
	for i, v := range values {
		lits[i] = "'" + v + "'"
	}
	return &schema.Schema{Dialect: dialect.Postgres, Tables: []schema.Table{{
		Name:       "notes",
		Columns:    []schema.Column{{Name: "id", Type: "bigint"}, {Name: "kind", Type: "text"}},
		PrimaryKey: []string{"id"},
		Checks:     []schema.Check{{Name: "notes_kind_check", Expr: `"kind" IN (` + strings.Join(lits, ",") + `)`}},
	}}}
}

// The order of the values must not make a change.
func TestRedValueListOrderIsNoChange(t *testing.T) {
	changes, _, err := migrate.Diff(stubCheckIn("a", "b"), stubCheckIn("b", "a"), migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("the same set in another order produced %d changes: %+v", len(changes), changes)
	}
}

// Adding a value must be one change that widens the list, not a drop and an add.
func TestRedAddedValueIsOneChange(t *testing.T) {
	changes, _, err := migrate.Diff(stubCheckIn("a"), stubCheckIn("a", "b"), migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Kind != "alter_check_in" {
		kinds := make([]string, len(changes))
		for i, c := range changes {
			kinds[i] = string(c.Kind)
		}
		t.Fatalf("adding a value produced %v, want one alter_check_in", kinds)
	}
}
