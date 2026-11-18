// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"strings"
	"testing"

	testdb "github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// excludeSchema is a bookings table that forbids two bookings of one room from
// overlapping. exclude is nil for the table without the constraint.
func excludeSchema(exclude *schema.Exclude) *schema.Schema {
	t := schema.Table{
		Name: "bookings",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint"},
			{Name: "room_id", Type: "integer"},
			{Name: "during", Type: "tstzrange"},
		},
		PrimaryKey: []string{"id"},
	}
	if exclude != nil {
		t.Excludes = []schema.Exclude{*exclude}
	}
	return &schema.Schema{
		Dialect: "postgres",
		// The scratch database has pgvector installed, as the deferrable tests
		// declare it.
		Extensions: []schema.Extension{{Name: "btree_gist"}, {Name: "vector"}},
		Tables:     []schema.Table{t},
	}
}

func overlapExclude() *schema.Exclude {
	return &schema.Exclude{
		Name:   "bookings_no_overlap",
		Method: "gist",
		Elements: []schema.ExcludeElement{
			{Column: "room_id", Operator: "="},
			{Column: "during", Operator: "&&"},
		},
	}
}

// Catches: an exclusion constraint lost in CREATE TABLE or in inspection, so a
// database built from the model accepts overlapping bookings, or one that
// reads back as a different model; and a btree_gist extension that is not
// created before the constraint that needs it.
func TestPostgresExcludeConstraintRoundTrip(t *testing.T) {
	db := concurrentIndexDB(t)
	desired := excludeSchema(overlapExclude())
	applyStatements(t, db, planStatements(t, &schema.Schema{Dialect: "postgres"}, desired, migrate.DiffOptions{}))

	got := inspectIn(t, db, concurrentIndexSchema)
	if !schema.Equal(got, desired) {
		t.Fatalf("round trip has a diff:\n%s", jsonDiff(got, desired))
	}
	if again, _, err := migrate.Diff(got, desired, migrate.DiffOptions{}); err != nil || len(again) != 0 {
		t.Fatalf("second diff must be empty: %v %+v", err, again)
	}

	ctx := context.Background()
	exec := func(sql string) error {
		_, err := db.Driver().Exec(ctx, sql, nil)
		return err
	}
	if err := exec(`INSERT INTO bookings VALUES (1, 7, tstzrange('2026-01-01 10:00+00', '2026-01-01 12:00+00'))`); err != nil {
		t.Fatal(err)
	}
	// The same room over the same hours is refused; another room, or the same
	// room after the first booking ends (the upper bound is exclusive), is not.
	if err := exec(`INSERT INTO bookings VALUES (2, 7, tstzrange('2026-01-01 11:00+00', '2026-01-01 13:00+00'))`); err == nil {
		t.Fatal("an overlapping booking of the same room was accepted")
	}
	for _, sql := range []string{
		`INSERT INTO bookings VALUES (3, 8, tstzrange('2026-01-01 11:00+00', '2026-01-01 13:00+00'))`,
		`INSERT INTO bookings VALUES (4, 7, tstzrange('2026-01-01 12:00+00', '2026-01-01 13:00+00'))`,
	} {
		if err := exec(sql); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
}

// Catches: a deferrable exclusion constraint read back as an immediate one, or
// rendered without its DEFERRABLE clause.
func TestPostgresExcludeConstraintDeferrableRoundTrip(t *testing.T) {
	db := concurrentIndexDB(t)
	ex := overlapExclude()
	ex.Deferrable, ex.InitiallyDeferred = true, true
	desired := excludeSchema(ex)
	applyStatements(t, db, planStatements(t, &schema.Schema{Dialect: "postgres"}, desired, migrate.DiffOptions{}))

	got := inspectIn(t, db, concurrentIndexSchema)
	if !schema.Equal(got, desired) {
		t.Fatalf("round trip has a diff:\n%s", jsonDiff(got, desired))
	}
	if len(got.Tables) != 1 || len(got.Tables[0].Excludes) != 1 || !got.Tables[0].Excludes[0].InitiallyDeferred {
		t.Fatalf("the inspection lost the deferral: %+v", got.Tables)
	}
}

// Catches: a diff that does not add, drop or replace an exclusion constraint
// on a table that exists, or that plans a change for an unchanged one.
func TestPostgresExcludeConstraintDiffApplies(t *testing.T) {
	db := concurrentIndexDB(t)
	bare := excludeSchema(nil)
	applyStatements(t, db, planStatements(t, &schema.Schema{Dialect: "postgres"}, bare, migrate.DiffOptions{}))

	withEx := excludeSchema(overlapExclude())
	changes, _, err := migrate.Diff(bare, withEx, migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Kind != migrate.AddExclude || changes[0].Name != "bookings_no_overlap" {
		t.Fatalf("adding the constraint: changes = %+v", changes)
	}
	applyStatements(t, db, planStatements(t, bare, withEx, migrate.DiffOptions{}))
	if got := inspectIn(t, db, concurrentIndexSchema); !schema.Equal(got, withEx) {
		t.Fatalf("after the add:\n%s", jsonDiff(got, withEx))
	}

	// A different operator list is a different constraint: drop and add.
	changed := overlapExclude()
	changed.Elements = []schema.ExcludeElement{{Column: "during", Operator: "&&"}}
	next := excludeSchema(changed)
	changes, _, err = migrate.Diff(withEx, next, migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 || changes[0].Kind != migrate.DropExclude || changes[1].Kind != migrate.AddExclude {
		t.Fatalf("changing the elements: changes = %+v", changes)
	}
	applyStatements(t, db, planStatements(t, withEx, next, migrate.DiffOptions{}))
	if got := inspectIn(t, db, concurrentIndexSchema); !schema.Equal(got, next) {
		t.Fatalf("after the replace:\n%s", jsonDiff(got, next))
	}

	applyStatements(t, db, planStatements(t, next, bare, migrate.DiffOptions{}))
	if got := inspectIn(t, db, concurrentIndexSchema); !schema.Equal(got, bare) {
		t.Fatalf("after the drop:\n%s", jsonDiff(got, bare))
	}
}

// Catches: an exclusion constraint that momo cannot model (a WHERE predicate)
// being skipped, which would let Baseline and drift report a clean database
// that does not match its model, instead of refusing it with a message that
// names the constraint.
func TestPostgresInspectRefusesUnmodeledExclusion(t *testing.T) {
	// btree_gist is a database-wide extension that migrate's drift checks would
	// report as unmanaged: a scratch database keeps it out of the shared one.
	db := testopen.PostgresIn(t, testdb.ScratchDatabase(t, "momo_excl_"))
	ctx := context.Background()
	for _, sql := range []string{
		`CREATE EXTENSION IF NOT EXISTS btree_gist`,
		`CREATE TABLE slots (id bigint PRIMARY KEY, room int NOT NULL, live boolean NOT NULL, during tstzrange NOT NULL,
  CONSTRAINT slots_live_no_overlap EXCLUDE USING gist (room WITH =, during WITH &&) WHERE (live))`,
	} {
		if _, err := db.Driver().Exec(ctx, sql, nil); err != nil {
			t.Skipf("setup: %v", err)
		}
	}
	_, err := inspectOnce(ctx, t, db, migrate.InspectOptions{Schemas: []string{concurrentIndexSchema}})
	if err == nil {
		t.Fatal("an exclusion constraint with a WHERE predicate was inspected without an error")
	}
	for _, want := range []string{"slots_live_no_overlap", "exclusion"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	// Excluding the table is the documented way out.
	if _, err := inspectOnce(ctx, t, db, migrate.InspectOptions{Schemas: []string{concurrentIndexSchema}, Exclude: []string{"slots"}}); err != nil {
		t.Fatalf("an excluded table must not fail the inspection: %v", err)
	}
}
