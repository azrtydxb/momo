// SPDX-License-Identifier: Apache-2.0

//go:build integration

package history

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBaselineSnapshotsEachMigration(t *testing.T) {
	scratch := testopen.Postgres(t)
	conv, _, err := Convert(Source{Tool: "goose", Migrations: []Raw{
		{Version: "1", Name: "a", Up: []byte("CREATE TABLE a (x int);"), Origin: "1_a.sql"},
		{Version: "2", Name: "b", Up: []byte("CREATE TABLE b (y int);\nCREATE INDEX b_y ON b (y);"), Origin: "2_b.sql"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := Baseline(context.Background(), scratch, mpg.New(), conv)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 2 {
		t.Fatalf("want one snapshot per migration, got %d", len(snaps))
	}
	if snaps[0].ParentVersion != "" || snaps[1].ParentVersion != conv[0].Version {
		t.Fatalf("parent chain wrong: %q, %q", snaps[0].ParentVersion, snaps[1].ParentVersion)
	}
	if len(snaps[0].Schema.Tables) != 1 || len(snaps[1].Schema.Tables) != 2 {
		t.Fatalf("snapshots must show the schema after each migration: %d, %d tables",
			len(snaps[0].Schema.Tables), len(snaps[1].Schema.Tables))
	}
}

func TestBaselineRefusesNonEmptyScratch(t *testing.T) {
	scratch := testopen.Postgres(t)
	if _, err := momo.Exec(context.Background(), scratch, "CREATE TABLE leftover (x int)"); err != nil {
		t.Fatal(err)
	}
	if _, err := Baseline(context.Background(), scratch, mpg.New(), nil); err == nil {
		t.Fatal("Baseline must refuse a scratch database that is not empty")
	}
}

// A pool of one connection must not deadlock Baseline: it may not hold a
// connection while Inspect asks for another. The deadline turns a regression
// into a failure instead of a hang.
func TestBaselineWorksOnASingleConnectionPool(t *testing.T) {
	u, err := url.Parse(testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("pool_max_conns", "1")
	u.RawQuery = q.Encode()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if got := pool.Config().MaxConns; got != 1 {
		t.Fatalf("the test needs a one-connection pool, got %d", got)
	}
	scratch := momo.New(pgxdriver.New(pool))
	// Session state (search_path, a temp table) works inside a migration, a
	// bare jsonb ? operator is not read as a placeholder, and nothing leaks
	// into the next migration or the inspection.
	schema := testdb.Schema(t)
	first := "SET search_path TO " + strconv.Quote(schema) + ", public;\n" +
		"CREATE TEMP TABLE scratch_tmp (x int);\n" +
		"CREATE TABLE a (data jsonb, CONSTRAINT a_has_id CHECK (data ? 'id'));"
	conv, _, err := Convert(Source{Tool: "goose", Migrations: []Raw{
		{Version: "1", Name: "a", Up: []byte(first), Origin: "1_a.sql"},
		{Version: "2", Name: "b", Up: []byte("CREATE TABLE b (y int);"), Origin: "2_b.sql"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := Baseline(ctx, scratch, mpg.New(), conv)
	if err != nil {
		t.Fatalf("Baseline on a one-connection pool: %v", err)
	}
	if len(snaps) != 2 || len(snaps[0].Schema.Tables) != 1 || len(snaps[1].Schema.Tables) != 2 {
		t.Fatalf("unexpected snapshots: %+v", snaps)
	}
}

// A migration that is not marked non-transactional runs in one transaction, as
// goose, golang-migrate, Atlas, Bun and momo migrate apply run it, so an ON
// COMMIT DROP temp table lives until the end of the migration (issue 87).
func TestBaselineRunsATransactionalMigrationInOneTransaction(t *testing.T) {
	scratch := testopen.Postgres(t)
	up := "CREATE TABLE t (id int);\n" +
		"CREATE TEMP TABLE m ON COMMIT DROP AS SELECT 1 AS id;\n" +
		"INSERT INTO t SELECT id FROM m;"
	conv, _, err := Convert(Source{Tool: "goose", Migrations: []Raw{
		{Version: "1", Name: "x", Up: []byte(up), Origin: "00001_x.sql"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := Baseline(context.Background(), scratch, mpg.New(), conv)
	if err != nil {
		t.Fatalf("Baseline of an ON COMMIT DROP migration: %v", err)
	}
	if len(snaps) != 1 || len(snaps[0].Schema.Tables) != 1 {
		t.Fatalf("want one snapshot with table t, got %+v", snaps)
	}
}

// A failing statement rolls the whole migration back and surfaces the error,
// and a NoTx migration still runs outside a transaction (CREATE INDEX
// CONCURRENTLY is refused inside one).
func TestBaselineRollsBackAndHonoursNoTx(t *testing.T) {
	ctx := context.Background()
	scratch := testopen.Postgres(t)
	bad, _, err := Convert(Source{Tool: "goose", Migrations: []Raw{
		{Version: "1", Name: "bad", Up: []byte("CREATE TABLE a (x int);\nSELECT * FROM missing_table;"), Origin: "1_bad.sql"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Baseline(ctx, scratch, mpg.New(), bad); err == nil {
		t.Fatal("Baseline must report the failing statement")
	}
	if _, err := momo.Exec(ctx, scratch, "SELECT 1 FROM a"); err == nil {
		t.Fatal("the failed migration must be rolled back, but table a exists")
	}
	scratch2 := testopen.Postgres(t)
	conv, _, err := Convert(Source{Tool: "goose", Migrations: []Raw{
		{Version: "1", Name: "a", Up: []byte("CREATE TABLE a (x int);"), Origin: "1_a.sql"},
		{Version: "2", Name: "idx", Up: []byte("CREATE INDEX CONCURRENTLY a_x ON a (x);"), NoTx: true, Origin: "2_idx.sql"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Baseline(ctx, scratch2, mpg.New(), conv); err != nil {
		t.Fatalf("a NoTx migration must run outside a transaction: %v", err)
	}
}

// The directory WriteImport produces passes migrate validate (issue 88): the
// imported migrations create the schema, so they are not custom migrations.
func TestWriteImportValidates(t *testing.T) {
	scratch := testopen.Postgres(t)
	conv, _, err := Convert(Source{Tool: "goose", Migrations: []Raw{
		{Version: "1", Name: "a", Up: []byte("CREATE TABLE a (x int);"), Origin: "1_a.sql"},
		{Version: "2", Name: "b", Up: []byte("CREATE TABLE b (y int);"), Origin: "2_b.sql"},
		{Version: "3", Name: "c", Up: []byte("CREATE TABLE c (z int);"), NoTx: true, Origin: "3_c.sql"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := Baseline(context.Background(), scratch, mpg.New(), conv)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "migrations")
	if err := WriteImport(dir, conv, snaps); err != nil {
		t.Fatal(err)
	}
	rep, err := migrate.ValidateOffline(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Problems) != 0 {
		t.Fatalf("the imported directory must validate, got: %v", rep.Problems)
	}
}
