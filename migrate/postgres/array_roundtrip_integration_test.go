// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"testing"

	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

func arraySchema(extra ...schema.Column) *schema.Schema {
	cols := []schema.Column{
		{Name: "id", Type: "bigint", Identity: "by_default"},
		{Name: "products", Type: "text[]", Default: "'{}'"},
		{Name: "scores", Type: "integer[]"},
		{Name: "counts", Type: "bigint[]"},
		{Name: "flags", Type: "boolean[]"},
		{Name: "weights", Type: "double precision[]"},
		{Name: "codes", Type: "character varying(8)[]", Nullable: true},
	}
	return &schema.Schema{
		Dialect:    "postgres",
		Extensions: []schema.Extension{{Name: "vector"}},
		Tables: []schema.Table{{
			Name:       "contacts",
			PrimaryKey: []string{"id"},
			Columns:    append(cols, extra...),
		}},
	}
}

// Catches: an array column type that the inspector reports differently from
// the generator's spelling (text[] versus _text), which would show up as a
// perpetual diff after the first migration.
func TestArrayColumnsMigrationRoundTrip(t *testing.T) {
	db := testopen.Postgres(t)
	got := roundTrip(t, db, arraySchema())
	tbl, ok := got.Table("contacts")
	if !ok {
		t.Fatal("contacts missing after round trip")
	}
	for name, typ := range map[string]string{
		"products": "text[]", "scores": "integer[]", "counts": "bigint[]",
		"flags": "boolean[]", "weights": "double precision[]", "codes": "character varying(8)[]",
	} {
		c, found := tbl.Column(name)
		if !found || c.Type != typ {
			t.Errorf("column %s = %+v, want type %s", name, c, typ)
		}
	}

	// Adding an array column to a live table is a plain diff and round-trips.
	next := arraySchema(schema.Column{Name: "tags", Type: "text[]", Nullable: true})
	applyStatements(t, db, planStatements(t, got, next, migrate.DiffOptions{}))
	after := inspectNow(t, db)
	if !schema.Equal(after, next) {
		t.Fatalf("added array column has a diff:\n%s", jsonDiff(after, next))
	}
	if again, _, err := migrate.Diff(after, next, migrate.DiffOptions{}); err != nil || len(again) != 0 {
		t.Fatalf("second diff must be empty: %v %+v", err, again)
	}
}
