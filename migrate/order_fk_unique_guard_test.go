// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"testing"

	"github.com/azrtydxb/momo/schema"
)

// Catches: the fix moving every foreign key out of its create_table, not only
// the ones whose target key arrives later in the plan.
func TestOrderFKStaysInlineWhenNothingPendingKeysIt(t *testing.T) {
	partial := uniqueIndexChange("licence", "licence_partial", "licence_id")
	partial.After.(*schema.Index).Where = "active"
	expr := Change{Kind: CreateIndex, Table: "licence", Name: "licence_lower", After: &schema.Index{
		Name: "licence_lower", Unique: true, Parts: []schema.IndexPart{{Expr: "lower(licence_id)"}},
	}}
	plain := Change{Kind: CreateIndex, Table: "licence", Name: "licence_plain", After: &schema.Index{
		Name: "licence_plain", Parts: []schema.IndexPart{{Column: "licence_id"}},
	}}
	other := uniqueIndexChange("licence", "licence_other", "other_id")
	cases := []struct {
		name string
		in   []Change
	}{
		{"no key change at all", []Change{ctable("licence"), ctable("usage", usageFK())}},
		{"partial unique index", []Change{ctable("licence"), ctable("usage", usageFK()), partial}},
		{"expression unique index", []Change{ctable("licence"), ctable("usage", usageFK()), expr}},
		{"non-unique index", []Change{ctable("licence"), ctable("usage", usageFK()), plain}},
		{"unique index on other columns", []Change{ctable("licence"), ctable("usage", usageFK()), other}},
		{"unique index on another table", []Change{ctable("licence"), ctable("usage", usageFK()), uniqueIndexChange("usage", "u", "licence_id")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := OrderErr(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if fks := inlineFKNames(t, got, "usage"); len(fks) != 1 {
				t.Fatalf("the foreign key must stay inline, kept %v: %v", fks, names(got))
			}
		})
	}
}

// Catches: a composite foreign key not matching a unique index whose columns
// are listed in another order; PostgreSQL accepts either order.
func TestOrderCompositeFKToUniqueIndexInOtherOrder(t *testing.T) {
	fk := schema.ForeignKey{Name: "usage_fkey", Columns: []string{"a", "b"}, RefTable: "licence", RefColumns: []string{"a", "b"}}
	got, err := OrderErr([]Change{ctable("licence"), ctable("usage", fk), uniqueIndexChange("licence", "licence_ba", "b", "a")})
	if err != nil {
		t.Fatal(err)
	}
	if idx(t, got, "add_fk:usage_fkey") < idx(t, got, "create_index:licence_ba") {
		t.Fatalf("add_fk must follow the unique index: %v", names(got))
	}
}

// Catches: a foreign key of a schema-qualified table not matching the unique
// index created on it, so the key stays inline.
func TestOrderQualifiedFKToUniqueIndex(t *testing.T) {
	usage := ctable("usage", schema.ForeignKey{Name: "usage_fkey", Columns: []string{"licence_id"}, RefTable: "licence", RefColumns: []string{"licence_id"}})
	usage.Schema = "app"
	usage.After.(*schema.Table).Schema = "app"
	idxc := uniqueIndexChange("licence", "licence_key", "licence_id")
	idxc.Schema = "app"
	got, err := OrderErr([]Change{usage, idxc})
	if err != nil {
		t.Fatal(err)
	}
	if fks := inlineFKNames(t, got, "usage"); len(fks) != 0 {
		t.Fatalf("the foreign key must move out of create_table: %v", names(got))
	}
}

// Catches: an index added to an existing table placed after a foreign key that
// is added to another existing table in the same plan.
func TestOrderAddFKAfterNewUniqueIndex(t *testing.T) {
	got, err := OrderErr([]Change{
		{Kind: AddFK, Table: "usage", Name: "usage_fkey", After: &schema.ForeignKey{Name: "usage_fkey", Columns: []string{"licence_id"}, RefTable: "licence", RefColumns: []string{"licence_id"}}},
		uniqueIndexChange("licence", "licence_key", "licence_id"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if idx(t, got, "add_fk:usage_fkey") < idx(t, got, "create_index:licence_key") {
		t.Fatalf("add_fk must follow the unique index: %v", names(got))
	}
}

// Catches: a unique index or constraint dropped before the foreign key that
// depends on it (PostgreSQL: cannot drop index because constraint depends on
// it; MySQL: needed in a foreign key constraint).
func TestOrderDropFKBeforeItsUniqueKey(t *testing.T) {
	cases := []struct {
		name string
		key  Change
	}{
		{"index", Change{Kind: DropIndex, Table: "licence", Name: "licence_key", Before: &schema.Index{Name: "licence_key", Unique: true}}},
		{"constraint", Change{Kind: DropUnique, Table: "licence", Name: "licence_key", Before: &schema.Unique{Name: "licence_key", Columns: []string{"licence_id"}}}},
		{"primary key", Change{Kind: DropPrimaryKey, Table: "licence", Name: "licence_pkey", Before: []string{"licence_id"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := OrderErr([]Change{tc.key, {Kind: DropFK, Table: "usage", Name: "usage_fkey"}})
			if err != nil {
				t.Fatal(err)
			}
			if idx(t, got, "drop_fk:usage_fkey") > idx(t, got, string(tc.key.Kind)+":"+tc.key.Name) {
				t.Fatalf("drop_fk must come first: %v", names(got))
			}
		})
	}
}

// Catches: an index that is replaced (same name, new columns) being created
// before the foreign key on the old index is dropped, or the foreign key being
// re-added before the new index exists.
func TestOrderReplacedUniqueIndexAroundForeignKey(t *testing.T) {
	got, err := OrderErr([]Change{
		{Kind: AddFK, Table: "usage", Name: "usage_fkey", After: &schema.ForeignKey{Name: "usage_fkey", Columns: []string{"k"}, RefTable: "licence", RefColumns: []string{"k"}}},
		uniqueIndexChange("licence", "licence_key", "k"),
		{Kind: DropIndex, Table: "licence", Name: "licence_key", Before: &schema.Index{Name: "licence_key", Unique: true}},
		{Kind: DropFK, Table: "usage", Name: "usage_fkey"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"drop_fk:usage_fkey", "drop_index:licence_key", "create_index:licence_key", "add_fk:usage_fkey"}
	for i := 1; i < len(want); i++ {
		if idx(t, got, want[i-1]) > idx(t, got, want[i]) {
			t.Fatalf("got %v, want the order %v", names(got), want)
		}
	}
}
