// SPDX-License-Identifier: Apache-2.0

//go:build integration

package cli

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	testdb "github.com/azrtydxb/momo/internal/testdb"
)

// deferrableDDL is a database with the three kinds of deferrable constraint:
// a unique constraint that lets one transaction reorder the stops of a route
// (the shape of issue 237), a foreign key that is deferrable but checked
// immediately, and a primary key that is initially deferred.
var deferrableDDL = []string{
	`CREATE TABLE routes (id bigint PRIMARY KEY)`,
	`CREATE TABLE stops (
		id bigint PRIMARY KEY,
		route_id bigint NOT NULL,
		position integer NOT NULL,
		CONSTRAINT stops_route_position_key UNIQUE (route_id, position) DEFERRABLE INITIALLY DEFERRED,
		CONSTRAINT stops_route_id_fkey FOREIGN KEY (route_id) REFERENCES routes (id) DEFERRABLE)`,
	`CREATE TABLE labels (
		id bigint NOT NULL,
		name text NOT NULL,
		CONSTRAINT labels_pkey PRIMARY KEY (id) DEFERRABLE INITIALLY DEFERRED)`,
}

// constraintDeferral reads condeferrable and condeferred of the named
// constraint in the schema of dsn.
func constraintDeferral(t *testing.T, dsn, name string) (deferrable, deferred bool) {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	row := db.QueryRowContext(context.Background(), `SELECT c.condeferrable, c.condeferred
FROM pg_constraint c JOIN pg_namespace n ON n.oid = c.connamespace
WHERE c.conname = $1 AND n.nspname = current_schema()`, name)
	if err := row.Scan(&deferrable, &deferred); err != nil {
		t.Fatalf("constraint %s: %v", name, err)
	}
	return deferrable, deferred
}

// requireDeferral fails t unless the constraints of dsn have the wanted
// deferrability: deferrable and initially deferred, in that order.
func requireDeferral(t *testing.T, dsn string, want map[string][2]bool) {
	t.Helper()
	for name, w := range want {
		deferrable, deferred := constraintDeferral(t, dsn, name)
		if deferrable != w[0] || deferred != w[1] {
			t.Errorf("constraint %s: deferrable %v, initially deferred %v; want %v, %v", name, deferrable, deferred, w[0], w[1])
		}
	}
}

// TestGenerateFromDBDeferrable is issue 237 end to end: generate --from-db
// declares the deferrable constraints, the init migration creates them
// deferrable on an empty database, and the source database and the migrated one
// both compare equal to the models.
//
// Catches: DEFERRABLE and INITIALLY DEFERRED dropped between inspection and the
// init migration, so a fresh install fails the transaction that reorders the
// rows with a unique violation while drift and baseline on the source database
// look clean.
func TestGenerateFromDBDeferrable(t *testing.T) {
	dsn := testdb.PostgresURL(t)
	execAll(t, dsn, deferrableDDL)
	root := copyModule(t, t.TempDir())
	writeFile(t, filepath.Join(root, "momo.yaml"), `version: 1
packages:
  - ./models
dialects: [postgres]
migrate:
  dir:
    postgres: migrations/postgres
  url_env:
    postgres: MOMO_TEST_URL
  exclude: [vector, pgcrypto]
`)
	chdir(t, root)
	t.Setenv("MOMO_TEST_URL", dsn)
	if code, out, errOut := fromDB(t); code != 0 {
		t.Fatalf("generate --from-db: exit %d\n%s%s", code, out, errOut)
	}
	files, _ := filepath.Glob(filepath.Join(root, "migrations", "postgres", "*_init.up.sql"))
	if len(files) != 1 {
		t.Fatalf("init migration: %v", files)
	}
	var o, e bytes.Buffer
	if c := Run([]string{"generate"}, &o, &e); c != 0 {
		t.Fatalf("generate: exit %d\n%s%s", c, o.String(), e.String())
	}
	version := strings.SplitN(filepath.Base(files[0]), "_", 2)[0]

	stops := readText(t, filepath.Join(root, "models", "stops_model.go"))
	for _, want := range []string{`c.Unique("stops_route_position_key"`, ".InitiallyDeferred()", `c.ForeignKey("stops_route_id_fkey"`, ".Deferrable()"} {
		if !strings.Contains(stops, want) {
			t.Errorf("the stops model does not declare %s:\n%s", want, stops)
		}
	}
	if labels := readText(t, filepath.Join(root, "models", "labels_model.go")); !strings.Contains(labels, "c.PrimaryKeyInitiallyDeferred()") {
		t.Errorf("the labels model does not declare its deferred primary key:\n%s", labels)
	}
	initSQL := readText(t, files[0])
	if n := strings.Count(initSQL, "DEFERRABLE INITIALLY DEFERRED"); n != 2 {
		t.Errorf("the init migration has %d initially deferred constraints, want 2 (the unique and the labels key):\n%s", n, initSQL)
	}
	if n := strings.Count(initSQL, "DEFERRABLE"); n != 3 {
		t.Errorf("the init migration has %d deferrable constraints, want 3:\n%s", n, initSQL)
	}

	// The source database adopts the migration without a difference.
	if c, o, e := cliRun(t, dsn, "baseline", "--version", version); c != 0 {
		t.Fatalf("baseline: exit %d\n%s%s", c, o, e)
	}
	if c, o, e := cliRun(t, dsn, "diff", "after"); c != 0 || !strings.Contains(o, "no changes") {
		t.Fatalf("diff after baseline must be empty: exit %d\n%s%s", c, o, e)
	}
	if c, o, e := cliRun(t, dsn, "drift"); c != 0 {
		t.Fatalf("drift: exit %d\n%s%s", c, o, e)
	}

	// An empty database builds from the init migration alone, with the same
	// deferrability, and reorders rows in one statement.
	t.Run("empty database", func(t *testing.T) {
		fresh := testdb.PostgresURL(t)
		if c, o, e := cliRun(t, fresh, "apply"); c != 0 {
			t.Fatalf("apply on an empty database: exit %d\n%s%s", c, o, e)
		}
		requireDeferral(t, fresh, map[string][2]bool{
			"routes_pkey":              {false, false},
			"stops_route_position_key": {true, true},
			"stops_route_id_fkey":      {true, false},
			"labels_pkey":              {true, true},
		})
		if c, o, e := cliRun(t, fresh, "diff", "after"); c != 0 || !strings.Contains(o, "no changes") {
			t.Fatalf("diff on the migrated database must be empty: exit %d\n%s%s", c, o, e)
		}
		if c, o, e := cliRun(t, fresh, "drift"); c != 0 {
			t.Fatalf("drift on the migrated database: exit %d\n%s%s", c, o, e)
		}
		execAll(t, fresh, []string{
			`INSERT INTO routes VALUES (1)`,
			`INSERT INTO stops VALUES (1, 1, 1), (2, 1, 2)`,
			`UPDATE stops SET position = 3 - position WHERE route_id = 1`,
		})
	})
}
