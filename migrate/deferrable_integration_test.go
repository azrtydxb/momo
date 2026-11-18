// SPDX-License-Identifier: Apache-2.0

//go:build integration

package migrate_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	testdb "github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// deferrableDesired is the schema of the deferrable drift tests as snapshot
// JSON: a unique that is DEFERRABLE INITIALLY DEFERRED, a foreign key that is
// DEFERRABLE, and a primary key that is DEFERRABLE INITIALLY DEFERRED.
func deferrableDesired(t *testing.T) *schema.Schema {
	t.Helper()
	const raw = `{
  "format_version": 1,
  "dialect": "postgres",
  "extensions": [{"name": "vector"}],
  "tables": [
    {"name": "routes", "columns": [{"name": "id", "type": "bigint"}], "primary_key": ["id"]},
    {"name": "stops",
     "columns": [{"name": "id", "type": "bigint"}, {"name": "route_id", "type": "bigint"}, {"name": "position", "type": "integer"}],
     "primary_key": ["id"],
     "uniques": [{"name": "stops_route_position_key", "columns": ["route_id", "position"], "deferrable": true, "initially_deferred": true}],
     "foreign_keys": [{"name": "stops_route_id_fkey", "columns": ["route_id"], "ref_table": "routes", "ref_columns": ["id"], "deferrable": true}]},
    {"name": "labels",
     "columns": [{"name": "id", "type": "bigint"}, {"name": "name", "type": "text"}],
     "primary_key": ["id"], "primary_key_deferrable": true, "primary_key_initially_deferred": true}
  ]
}`
	var s schema.Schema
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatal(err)
	}
	return &s
}

// deferrableMutations make the live constraints plain, the way a database
// built by another tool, or edited by hand, differs from the snapshot.
var deferrableMutations = []string{
	`ALTER TABLE stops ALTER CONSTRAINT stops_route_id_fkey NOT DEFERRABLE`,
	`ALTER TABLE stops DROP CONSTRAINT stops_route_position_key, ADD CONSTRAINT stops_route_position_key UNIQUE (route_id, position)`,
	`ALTER TABLE labels DROP CONSTRAINT labels_pkey, ADD CONSTRAINT labels_pkey PRIMARY KEY (id)`,
}

// deferrableFixture builds the database from the migrations of
// deferrableDesired, makes its constraints plain and leaves it without history.
// It runs in a database of its own: the repair builds an index CONCURRENTLY,
// which waits for every other transaction open in its database.
func deferrableFixture(t *testing.T) *driftFixture {
	t.Helper()
	db := testopen.PostgresIn(t, testdb.ScratchDatabase(t, "momo_cic_"))
	return newDriftFixtureOf(t, db, dialect.Postgres, deferrableDesired(t), deferrableMutations)
}

// Catches: drift and the baseline refusal not noticing that a constraint is
// not deferrable where the snapshot says it is: a taken-over database and a
// freshly built one would silently diverge.
func TestDriftReportsDeferrability(t *testing.T) {
	f := deferrableFixture(t)
	rep, err := f.engine().Drift(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	byRef := map[string]migrate.ObjectDiff{}
	for _, c := range rep.Changed {
		ref := c.Object.Kind + " " + c.Object.Name
		got = append(got, ref)
		byRef[ref] = c
	}
	slices.Sort(got)
	want := []string{"fk stops.stops_route_id_fkey", "table labels", "unique stops.stops_route_position_key"}
	if !slices.Equal(got, want) {
		t.Fatalf("changed = %v, want %v (missing %v, unmanaged %v)", got, want, rep.Missing, rep.Unmanaged)
	}
	uq := byRef["unique stops.stops_route_position_key"]
	if !strings.Contains(uq.Detail, "deferrable") || !strings.Contains(uq.Live, "deferrable=false") || !strings.Contains(uq.Snapshot, "deferrable=true") || !strings.Contains(uq.Snapshot, "initially_deferred=true") {
		t.Errorf("the unique differs as %+v, want deferrable=false live and deferrable=true, initially_deferred=true in the snapshot", uq)
	}
	fk := byRef["fk stops.stops_route_id_fkey"]
	if !strings.Contains(fk.Live, "deferrable=false") || !strings.Contains(fk.Snapshot, "deferrable=true") {
		t.Errorf("the foreign key differs as %+v", fk)
	}
	tbl := byRef["table labels"]
	if !strings.Contains(tbl.Detail, "primary_key_deferrable") || !strings.Contains(tbl.Live, "primary_key_deferrable=false") || !strings.Contains(tbl.Snapshot, "primary_key_deferrable=true") {
		t.Errorf("the table differs as %+v", tbl)
	}
}

// Catches: the baseline adopting a database whose constraints are not
// deferrable although the snapshot says they are, and a refusal that does not
// name the difference.
func TestBaselineRefusesADifferentDeferrability(t *testing.T) {
	f := deferrableFixture(t)
	err := f.engine().Baseline(context.Background(), f.version, false)
	if !errors.Is(err, migrate.ErrBaselineMismatch) {
		t.Fatalf("baseline of a database with plain constraints: %v, want ErrBaselineMismatch", err)
	}
	for _, want := range []string{"changed: unique stops.stops_route_position_key", "changed: fk stops.stops_route_id_fkey", "changed: table labels", "deferrable=false", "deferrable=true"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal lacks %q:\n%v", want, err)
		}
	}
}

// Catches: a repair that does not restore the deferrability (the database keeps
// differing from the snapshot), one that runs the drops without being asked to,
// and SQL that PostgreSQL rejects.
func TestDriftRepairRestoresDeferrability(t *testing.T) {
	ctx := context.Background()
	f := deferrableFixture(t)

	// Changing a constraint is a drop and an add, and a repair holds back every
	// drop until it is allowed.
	held, err := f.engine().DriftRepair(ctx, migrate.RepairOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(held.Withheld) == 0 || held.Complete() {
		t.Fatalf("repair = %+v, want the drops withheld", held)
	}

	r, err := f.engine().DriftRepair(ctx, migrate.RepairOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Complete() || len(r.Statements) == 0 {
		t.Fatalf("repair = %+v, want statements and nothing withheld", r)
	}
	sql := r.SQL()
	if n := strings.Count(sql, "DEFERRABLE INITIALLY DEFERRED"); n != 2 {
		t.Errorf("the script has %d initially deferred constraints, want 2:\n%s", n, sql)
	}
	if !strings.Contains(sql, `"stops_route_id_fkey"`) || !strings.Contains(sql, "DEFERRABLE") {
		t.Errorf("the script does not restore the foreign key:\n%s", sql)
	}
	f.applyStatements(r.Statements)
	rep, err := f.engine().Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean() {
		t.Fatalf("drift after the repair: %+v\nscript:\n%s", rep, sql)
	}
	if err := f.engine().Baseline(ctx, f.version, false); err != nil {
		t.Fatalf("baseline after the repair: %v", err)
	}
}
