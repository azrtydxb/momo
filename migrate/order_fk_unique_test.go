// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"slices"
	"testing"

	"github.com/azrtydxb/momo/schema"
)

func uniqueIndexChange(table, name string, cols ...string) Change {
	parts := make([]schema.IndexPart, len(cols))
	for i, c := range cols {
		parts[i] = schema.IndexPart{Column: c}
	}
	return Change{Kind: CreateIndex, Table: table, Name: name, After: &schema.Index{Name: name, Unique: true, Parts: parts}}
}

// inlineFKNames returns the foreign keys that the create_table of table keeps.
func inlineFKNames(t *testing.T, cs []Change, table string) []string {
	t.Helper()
	for _, c := range cs {
		if c.Kind == CreateTable && c.Name == table {
			var out []string
			for _, fk := range c.After.(*schema.Table).ForeignKeys {
				out = append(out, fk.Name)
			}
			return out
		}
	}
	t.Fatalf("create_table:%s missing from %v", table, names(cs))
	return nil
}

func usageFK() schema.ForeignKey {
	return schema.ForeignKey{Name: "usage_licence_fkey", Columns: []string{"licence_id"}, RefTable: "licence", RefColumns: []string{"licence_id"}}
}

// Catches: a foreign key written inside CREATE TABLE although the unique index
// it references is created later in the plan. PostgreSQL rejects that with
// SQLSTATE 42830, and MySQL and MariaDB need the index to exist as well.
func TestOrderFKToUniqueIndexOfNewTable(t *testing.T) {
	got, err := OrderErr([]Change{
		ctable("licence"),
		ctable("usage", usageFK()),
		uniqueIndexChange("licence", "licence_licence_id_key", "licence_id"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if fks := inlineFKNames(t, got, "usage"); len(fks) != 0 {
		t.Fatalf("the foreign key must not stay inline in create_table, kept %v: %v", fks, names(got))
	}
	if idx(t, got, "add_fk:usage_licence_fkey") < idx(t, got, "create_index:licence_licence_id_key") {
		t.Fatalf("add_fk must follow the unique index: %v", names(got))
	}
}

// Catches: the same ordering error when the unique index is added to a table
// that already exists, in the same plan as a new table that references it.
func TestOrderFKToNewUniqueIndexOfExistingTable(t *testing.T) {
	got, err := OrderErr([]Change{
		ctable("usage", usageFK()),
		uniqueIndexChange("licence", "licence_licence_id_key", "licence_id"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if fks := inlineFKNames(t, got, "usage"); len(fks) != 0 {
		t.Fatalf("the foreign key must not stay inline in create_table, kept %v: %v", fks, names(got))
	}
	if idx(t, got, "add_fk:usage_licence_fkey") < idx(t, got, "create_index:licence_licence_id_key") {
		t.Fatalf("add_fk must follow the unique index: %v", names(got))
	}
}

// Catches: a unique constraint added to an existing table (add_unique) landing
// after a new table whose inline foreign key references it.
func TestOrderFKToNewUniqueConstraintOfExistingTable(t *testing.T) {
	got, err := OrderErr([]Change{
		ctable("usage", usageFK()),
		{Kind: AddUnique, Table: "licence", Name: "licence_key", After: &schema.Unique{Name: "licence_key", Columns: []string{"licence_id"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if fks := inlineFKNames(t, got, "usage"); len(fks) != 0 {
		t.Fatalf("the foreign key must not stay inline in create_table, kept %v: %v", fks, names(got))
	}
	if idx(t, got, "add_fk:usage_licence_fkey") < idx(t, got, "add_unique:licence_key") {
		t.Fatalf("add_fk must follow the unique constraint: %v", names(got))
	}
}

// Catches: Order mutating the table payload of its input while it moves a
// foreign key out of the create_table.
func TestOrderFKToUniqueIndexKeepsInputIntact(t *testing.T) {
	in := []Change{
		ctable("usage", usageFK()),
		uniqueIndexChange("licence", "licence_licence_id_key", "licence_id"),
	}
	if _, err := OrderErr(in); err != nil {
		t.Fatal(err)
	}
	if n := len(in[0].After.(*schema.Table).ForeignKeys); n != 1 {
		t.Fatalf("the input create_table lost its foreign key: %d left", n)
	}
	if !slices.Equal(names(in), []string{"create_table:usage", "create_index:licence_licence_id_key"}) {
		t.Fatalf("input reordered: %v", names(in))
	}
}
