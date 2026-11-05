// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"io/fs"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

const canonVersion = "20260101000000"

// canonFS is a migration directory whose only migration has the snapshot of s.
func canonFS(t *testing.T, s *schema.Schema) fs.FS {
	t.Helper()
	var snap bytes.Buffer
	if err := migrate.WriteSnapshot(&snap, migrate.SnapshotFile{Version: canonVersion, Schema: s}); err != nil {
		t.Fatal(err)
	}
	return migratetest.WithSum(t, fstest.MapFS{
		canonVersion + "_init.up.sql":        {Data: []byte("SELECT 1;")},
		canonVersion + "_init.snapshot.json": {Data: snap.Bytes()},
	})
}

func canonSchema(tables ...schema.Table) *schema.Schema {
	return &schema.Schema{Dialect: "postgres", Extensions: []schema.Extension{{Name: "vector"}}, Tables: tables}
}

func canonEngine(t *testing.T, drv momo.Driver, fsys fs.FS, schemas ...string) *migrate.Engine {
	t.Helper()
	if len(schemas) == 0 {
		schemas = []string{testdb.Schema(t)}
	}
	return migratetest.NewEngine(t, mpg.New(), drv, fsys, migrate.WithConfig(migrate.Config{Schemas: schemas}))
}

func skippedNames(rep *migrate.DriftReport) []string {
	var out []string
	for _, r := range rep.CanonicalizeSkipped {
		out = append(out, r.Kind+":"+r.Name)
	}
	return out
}

// Two tables of one name in different schemas both carry text. Drift and
// Baseline canonicalize each in its own scratch schema instead of failing, and
// each keeps its own check (no crossing).
func TestCanonicalizeTwoSchemasSameTableName(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	cur := testdb.Schema(t)
	other := cur + "_b"
	mustExec(t, db.Driver(), `CREATE SCHEMA `+other)
	t.Cleanup(func() { _, _ = db.Driver().Exec(context.Background(), `DROP SCHEMA IF EXISTS `+other+` CASCADE`, nil) })
	mustExec(t, db.Driver(), `CREATE TABLE t (n int CONSTRAINT t_chk CHECK (n IN (1, 2)))`)
	mustExec(t, db.Driver(), `CREATE TABLE `+other+`.t (n int CONSTRAINT t_chk CHECK (n BETWEEN 1 AND 5))`)
	s := canonSchema(
		schema.Table{Name: "t", Columns: []schema.Column{{Name: "n", Type: "integer", Nullable: true}}, Checks: []schema.Check{{Name: "t_chk", Expr: "n IN (1, 2)"}}},
		schema.Table{Schema: other, Name: "t", Columns: []schema.Column{{Name: "n", Type: "integer", Nullable: true}}, Checks: []schema.Check{{Name: "t_chk", Expr: "n BETWEEN 1 AND 5"}}},
	)
	s.Schemas = []schema.SchemaNamespace{{Name: other}}
	e := canonEngine(t, db.Driver(), canonFS(t, s), cur, other)
	if err := e.Baseline(ctx, canonVersion, false); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	rep, err := e.Drift(ctx)
	if err != nil {
		t.Fatalf("drift: %v", err)
	}
	if !rep.Clean() || len(rep.CanonicalizeSkipped) != 0 {
		t.Fatalf("want a clean report with nothing skipped: %+v", rep)
	}
	// A real change in one schema's copy is still seen, on that table.
	mustExec(t, db.Driver(), `ALTER TABLE `+other+`.t DROP CONSTRAINT t_chk, ADD CONSTRAINT t_chk CHECK (n BETWEEN 1 AND 6)`)
	rep, err = e.Drift(ctx)
	if err != nil || len(rep.Changed) != 1 || rep.Changed[0].Object.Schema != other {
		t.Fatalf("a changed check in the second schema must be reported there: %+v %v", rep, err)
	}
}

// failingCreateDriver turns every CREATE TABLE of a table named in bad into a
// statement PostgreSQL rejects, so that table's scratch copy fails for real
// (the transaction is aborted until the savepoint is rolled back).
type failingCreateDriver struct {
	momo.Driver
	bad string
}

func (d failingCreateDriver) Pin(ctx context.Context) (momo.PinnedConn, error) {
	pc, err := d.Driver.(momo.Pinner).Pin(ctx)
	if err != nil {
		return nil, err
	}
	return failingCreateConn{PinnedConn: pc, bad: d.bad}, nil
}

type failingCreateConn struct {
	momo.PinnedConn
	bad string
}

func (c failingCreateConn) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	if strings.HasPrefix(sql, "CREATE TABLE") && strings.Contains(sql, `"`+c.bad+`"`) {
		sql = "SELECT 1/0"
	}
	return c.PinnedConn.Exec(ctx, sql, args)
}

func (c failingCreateConn) Discard() error {
	if d, ok := c.PinnedConn.(momo.Discarder); ok {
		return d.Discard()
	}
	return c.PinnedConn.Release()
}

// One table whose scratch CREATE fails falls back to its declared text alone:
// the other table is still canonicalized (its IN check would be a phantom
// change otherwise), and the report names only the failed one.
func TestDriftOneFailingTableFallsBackAlone(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExec(t, db.Driver(), `CREATE TABLE good (n int CONSTRAINT good_chk CHECK (n IN (1, 2)))`)
	mustExec(t, db.Driver(), `CREATE TABLE bad (n int DEFAULT 0)`)
	s := canonSchema(
		schema.Table{Name: "good", Columns: []schema.Column{{Name: "n", Type: "integer", Nullable: true}}, Checks: []schema.Check{{Name: "good_chk", Expr: "n IN (1, 2)"}}},
		schema.Table{Name: "bad", Columns: []schema.Column{{Name: "n", Type: "integer", Default: "0", Nullable: true}}},
	)
	fsys := canonFS(t, s)
	if err := canonEngine(t, db.Driver(), fsys).Baseline(ctx, canonVersion, false); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	rep, err := canonEngine(t, failingCreateDriver{Driver: db.Driver(), bad: "bad"}, fsys).Drift(ctx)
	if err != nil {
		t.Fatalf("drift must not fail: %v", err)
	}
	if !rep.Clean() {
		t.Fatalf("the good table must still be canonicalized: %+v", rep)
	}
	if got := skippedNames(rep); !slices.Equal(got, []string{"table:bad"}) {
		t.Fatalf("CanonicalizeSkipped = %v, want [table:bad]", got)
	}
}

// readOnlyDriver pins connections whose transactions are read-only by default,
// as on a hot standby or under default_transaction_read_only.
type readOnlyDriver struct{ momo.Driver }

func (d readOnlyDriver) Pin(ctx context.Context) (momo.PinnedConn, error) {
	pc, err := d.Driver.(momo.Pinner).Pin(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := pc.Exec(ctx, `SET default_transaction_read_only = on`, nil); err != nil {
		_ = pc.Release()
		return nil, err
	}
	return readOnlyConn{pc}, nil
}

type readOnlyConn struct{ momo.PinnedConn }

func (c readOnlyConn) Release() error {
	_, err := c.PinnedConn.Exec(context.Background(), `RESET default_transaction_read_only`, nil)
	if err != nil {
		return c.Discard()
	}
	return c.PinnedConn.Release()
}

func (c readOnlyConn) Discard() error {
	if d, ok := c.PinnedConn.(momo.Discarder); ok {
		return d.Discard()
	}
	return c.PinnedConn.Release()
}

// A read-only session cannot create the scratch schema. Drift still reports,
// comparing table text as declared and saying so; Canonicalize, which Baseline
// uses, stays strict and fails.
func TestDriftReadOnlySessionFallsBackWithWarning(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExec(t, db.Driver(), `CREATE TABLE t (n int CONSTRAINT t_chk CHECK (n IN (1, 2)))`)
	s := canonSchema(schema.Table{Name: "t", Columns: []schema.Column{{Name: "n", Type: "integer", Nullable: true}}, Checks: []schema.Check{{Name: "t_chk", Expr: "n IN (1, 2)"}}})
	fsys := canonFS(t, s)
	if err := canonEngine(t, db.Driver(), fsys).Baseline(ctx, canonVersion, false); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	ro := readOnlyDriver{db.Driver()}
	rep, err := canonEngine(t, ro, fsys).Drift(ctx)
	if err != nil {
		t.Fatalf("drift on a read-only session must not fail: %v", err)
	}
	if got := skippedNames(rep); !slices.Equal(got, []string{"table:t"}) {
		t.Fatalf("CanonicalizeSkipped = %v, want [table:t]", got)
	}
	if len(rep.Changed) != 1 || rep.Changed[0].Object.Name != "t.t_chk" {
		t.Fatalf("the declared text is compared as it is, so the IN check differs: %+v", rep)
	}

	pc, err := ro.Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Release() })
	if err := mpg.New().Canonicalize(ctx, pc, s); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("Canonicalize must stay strict on a read-only session: %v", err)
	}
}

// A default or generation expression that PostgreSQL stores with added
// parentheses ("1 + 2 * 3" is stored as "(1 + (2 * 3))") must not drift after a
// baseline: schema normalization alone cannot make these equal, so the default
// and generated rewrites are what keeps the report clean.
func TestCanonicalizeRewritesColumnDefaults(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExec(t, db.Driver(), `CREATE TABLE d (
		a int DEFAULT 1 + 2 * 3,
		n int,
		g int GENERATED ALWAYS AS (n * 2 + 1) STORED)`)
	s := canonSchema(schema.Table{Name: "d", Columns: []schema.Column{
		{Name: "a", Type: "integer", Default: "1 + 2 * 3", Nullable: true},
		{Name: "n", Type: "integer", Nullable: true},
		{Name: "g", Type: "integer", Generated: "n * 2 + 1", Nullable: true},
	}})
	e := canonEngine(t, db.Driver(), canonFS(t, s))
	if err := e.Baseline(ctx, canonVersion, false); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	rep, err := e.Drift(ctx)
	if err != nil || !rep.Clean() || len(rep.CanonicalizeSkipped) != 0 {
		t.Fatalf("phantom drift in defaults: %+v %v", rep, err)
	}
}

// An expression key PostgreSQL stores as a plain column (v::vector(3) with an
// operator class on a vector(3) column) reads back as a column part. The
// canonicalized part must be the read-back one, or the report shows "parts
// differ".
func TestCanonicalizeIndexExpressionStoredAsColumn(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExec(t, db.Driver(), `CREATE TABLE v (id bigint PRIMARY KEY, v vector(3))`)
	mustExec(t, db.Driver(), `CREATE INDEX v_cos ON v USING hnsw (v vector_cosine_ops)`)
	s := canonSchema(schema.Table{
		Name:       "v",
		Columns:    []schema.Column{{Name: "id", Type: "bigint"}, {Name: "v", Type: "vector(3)", Nullable: true}},
		PrimaryKey: []string{"id"},
		Indexes: []schema.Index{{Name: "v_cos", Method: "hnsw", Parts: []schema.IndexPart{
			{Expr: "(v::vector(3))", OpClass: "vector_cosine_ops"},
		}}},
	})
	e := canonEngine(t, db.Driver(), canonFS(t, s))
	if err := e.Baseline(ctx, canonVersion, false); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	rep, err := e.Drift(ctx)
	if err != nil || !rep.Clean() {
		t.Fatalf("phantom drift in index parts: %+v %v", rep, err)
	}
}

// recordingDriver records every statement the engine runs on pinned connections
// in order.
type recordingDriver struct {
	momo.Driver
	mu   *sync.Mutex
	stmt *[]string
}

func (d recordingDriver) Pin(ctx context.Context) (momo.PinnedConn, error) {
	pc, err := d.Driver.(momo.Pinner).Pin(ctx)
	if err != nil {
		return nil, err
	}
	return recordingConn{PinnedConn: pc, d: d}, nil
}

type recordingConn struct {
	momo.PinnedConn
	d recordingDriver
}

func (c recordingConn) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	c.d.mu.Lock()
	*c.d.stmt = append(*c.d.stmt, sql)
	c.d.mu.Unlock()
	return c.PinnedConn.Exec(ctx, sql, args)
}

func (c recordingConn) Discard() error {
	if d, ok := c.PinnedConn.(momo.Discarder); ok {
		return d.Discard()
	}
	return c.PinnedConn.Release()
}

// Drift with history changes nothing durable: every DDL it runs sits between a
// BEGIN and the ROLLBACK that ends it, nothing is committed, no row of any
// table is written, and no scratch schema survives.
func TestDriftWithHistoryOnlyRunsDDLInRolledBackTransaction(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExec(t, db.Driver(), `CREATE TABLE t (n int CONSTRAINT t_chk CHECK (n IN (1, 2)))`)
	s := canonSchema(schema.Table{Name: "t", Columns: []schema.Column{{Name: "n", Type: "integer", Nullable: true}}, Checks: []schema.Check{{Name: "t_chk", Expr: "n IN (1, 2)"}}})
	fsys := canonFS(t, s)
	if err := canonEngine(t, db.Driver(), fsys).Baseline(ctx, canonVersion, false); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	var (
		mu   sync.Mutex
		recd []string
	)
	rep, err := canonEngine(t, recordingDriver{Driver: db.Driver(), mu: &mu, stmt: &recd}, fsys).Drift(ctx)
	if err != nil || !rep.Clean() {
		t.Fatalf("drift: %+v %v", rep, err)
	}
	inTx, ddl, begins := false, 0, 0
	for _, sql := range recd {
		first, _, _ := strings.Cut(strings.ToUpper(strings.TrimSpace(sql)), " ")
		switch first {
		case "BEGIN":
			inTx = true
			begins++
		case "ROLLBACK":
			inTx = false
		case "COMMIT", "INSERT", "UPDATE", "DELETE", "TRUNCATE":
			t.Fatalf("Drift ran %q", sql)
		case "CREATE", "ALTER", "DROP":
			ddl++
			if !inTx {
				t.Fatalf("DDL outside the scratch transaction: %q", sql)
			}
		}
	}
	if inTx || begins == 0 || ddl == 0 {
		t.Fatalf("want DDL inside a BEGIN..ROLLBACK (begins=%d ddl=%d open=%v):\n%s", begins, ddl, inTx, strings.Join(recd, "\n"))
	}
	var left int
	rows, err := db.Driver().Query(ctx, `SELECT count(*) FROM pg_namespace WHERE nspname LIKE 'momo_canon_%'`, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		if err := rows.Scan(&left); err != nil {
			t.Fatal(err)
		}
	}
	if left != 0 {
		t.Fatalf("%d scratch schemas left behind", left)
	}
}
