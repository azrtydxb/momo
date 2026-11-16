// SPDX-License-Identifier: Apache-2.0

//go:build integration

package cli

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	testdb "github.com/azrtydxb/momo/internal/testdb"
)

// sequenceDDL is a database with a standalone sequence that a column default
// names, the shape of issue 207: nextval of a sequence no column owns, next to a
// serial column whose sequence is owned and so must stay out of the model.
var sequenceDDL = []string{
	`CREATE SEQUENCE redirect_jobs_seq AS bigint INCREMENT BY 1 START WITH 1000 MINVALUE 1000 CACHE 1`,
	`CREATE SEQUENCE ticket_seq AS integer INCREMENT BY 5 MINVALUE 1 MAXVALUE 100000 START WITH 1 CACHE 10 CYCLE`,
	// A counter that starts at 0 and a descending sequence whose maximum is -1:
	// the values the defaults would otherwise replace.
	`CREATE SEQUENCE counter_seq MINVALUE 0 START WITH 0`,
	`CREATE SEQUENCE countdown_seq INCREMENT BY -1 MAXVALUE -1 MINVALUE -1000 START WITH -1`,
	`CREATE TABLE redirect_jobs (
		id bigint PRIMARY KEY DEFAULT nextval('redirect_jobs_seq'::regclass),
		target text NOT NULL)`,
	`CREATE TABLE tickets (
		id bigserial PRIMARY KEY,
		number integer NOT NULL DEFAULT nextval('ticket_seq'::regclass))`,
}

func readText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // test path
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// sequenceSetup writes momo.yaml, adopts sequenceDDL with generate --from-db
// and returns the module root, the database URL and the init version.
func sequenceSetup(t *testing.T) (root, dsn, version string) {
	t.Helper()
	dsn = testdb.PostgresURL(t)
	execAll(t, dsn, sequenceDDL)
	root = copyModule(t, t.TempDir())
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
	return root, dsn, strings.SplitN(filepath.Base(files[0]), "_", 2)[0]
}

// TestGenerateFromDBStandaloneSequence is issue 207 end to end: the models and
// the init migration carry the standalone sequences, the init migration applies
// on an empty database, and the migrated database compares equal to the source
// one (no diff, no drift, a clean baseline).
//
// Catches: a standalone sequence lost between inspection and the init
// migration, so the init migration fails on an empty database with `relation
// "redirect_jobs_seq" does not exist` while drift and baseline on the source
// database look clean.
func TestGenerateFromDBStandaloneSequence(t *testing.T) {
	root, dsn, version := sequenceSetup(t)

	// An empty database builds from the init migration alone and ends up equal.
	t.Run("empty database", emptyDatabaseBuilds)

	models := readText(t, filepath.Join(root, "models", "redirect_jobs_model.go")) + readText(t, filepath.Join(root, "models", "tickets_model.go"))
	for _, want := range []string{`c.Sequence("redirect_jobs_seq")`, `c.Sequence("ticket_seq")`, `c.Sequence("counter_seq").MinValue(0)`, `c.Sequence("countdown_seq").Increment(-1).MinValue(-1000)`} {
		if !strings.Contains(models, want) {
			t.Fatalf("the models do not declare %s:\n%s", want, models)
		}
	}
	if strings.Contains(models, "tickets_id_seq") {
		t.Fatalf("the sequence a serial column owns must stay out of the models:\n%s", models)
	}
	initSQL := readText(t, filepath.Join(root, "migrations", "postgres", version+"_init.up.sql"))
	seqAt, tableAt := strings.Index(initSQL, `CREATE SEQUENCE "redirect_jobs_seq"`), strings.Index(initSQL, `CREATE TABLE "redirect_jobs"`)
	if seqAt < 0 || tableAt < 0 || seqAt > tableAt {
		t.Fatalf("the init migration must create the sequence before the table that uses it:\n%s", initSQL)
	}
	if strings.Contains(initSQL, `"tickets_id_seq"`) {
		t.Fatalf("the init migration creates the owned sequence of a serial column:\n%s", initSQL)
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
}

// emptyDatabaseBuilds applies the migrations of the working directory to an
// empty database and compares the result with the models.
func emptyDatabaseBuilds(t *testing.T) {
	fresh := testdb.PostgresURL(t)
	if c, o, e := cliRun(t, fresh, "apply"); c != 0 {
		t.Fatalf("apply on an empty database: exit %d\n%s%s", c, o, e)
	}
	if c, o, e := cliRun(t, fresh, "diff", "after"); c != 0 || !strings.Contains(o, "no changes") {
		t.Fatalf("diff on the migrated database must be empty: exit %d\n%s%s", c, o, e)
	}
	if c, o, e := cliRun(t, fresh, "drift"); c != 0 {
		t.Fatalf("drift on the migrated database: exit %d\n%s%s", c, o, e)
	}
	var start, incr, cache int64
	var cycle bool
	db, err := sql.Open("pgx", fresh)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	row := db.QueryRowContext(context.Background(), `SELECT seqstart, seqincrement, seqcache, seqcycle FROM pg_sequence WHERE seqrelid = 'ticket_seq'::regclass`)
	if err := row.Scan(&start, &incr, &cache, &cycle); err != nil {
		t.Fatal(err)
	}
	if start != 1 || incr != 5 || cache != 10 || !cycle {
		t.Fatalf("ticket_seq = start %d increment %d cache %d cycle %v", start, incr, cache, cycle)
	}
	var minValue, maxValue int64
	row = db.QueryRowContext(context.Background(), `SELECT seqstart, seqmin FROM pg_sequence WHERE seqrelid = 'counter_seq'::regclass`)
	if err := row.Scan(&start, &minValue); err != nil {
		t.Fatal(err)
	}
	if start != 0 || minValue != 0 {
		t.Fatalf("counter_seq = start %d min %d, want START 0 MINVALUE 0", start, minValue)
	}
	row = db.QueryRowContext(context.Background(), `SELECT seqstart, seqmax FROM pg_sequence WHERE seqrelid = 'countdown_seq'::regclass`)
	if err := row.Scan(&start, &maxValue); err != nil {
		t.Fatal(err)
	}
	if start != -1 || maxValue != -1 {
		t.Fatalf("countdown_seq = start %d max %d, want START -1 MAXVALUE -1", start, maxValue)
	}
}

// TestStandaloneSequenceAlterAndDrift changes a sequence in the model, which is
// one migration with one ALTER SEQUENCE, and changes one in the database, which
// drift reports.
//
// Catches: an increment change that is dropped, or rendered as a drop and a
// create that resets the counter, and drift that cannot see a sequence.
func TestStandaloneSequenceAlterAndDrift(t *testing.T) {
	root, dsn, version := sequenceSetup(t)
	if c, o, e := cliRun(t, dsn, "baseline", "--version", version); c != 0 {
		t.Fatalf("baseline: exit %d\n%s%s", c, o, e)
	}

	// A change made outside momo is drift.
	execAll(t, dsn, []string{`ALTER SEQUENCE redirect_jobs_seq INCREMENT BY 7`})
	c, out, _ := cliRun(t, dsn, "drift")
	if c != 1 || !strings.Contains(out, "changed: sequence redirect_jobs_seq") {
		t.Fatalf("drift must report the altered sequence: exit %d\n%s", c, out)
	}
	execAll(t, dsn, []string{`ALTER SEQUENCE redirect_jobs_seq INCREMENT BY 1`})
	if c, o, e := cliRun(t, dsn, "drift"); c != 0 {
		t.Fatalf("drift after restoring the sequence: exit %d\n%s%s", c, o, e)
	}

	// A change made in the model is one ALTER SEQUENCE.
	path := filepath.Join(root, "models", "redirect_jobs_model.go")
	src := readText(t, path)
	edited := strings.Replace(src, `c.Sequence("redirect_jobs_seq")`, `c.Sequence("redirect_jobs_seq").Increment(10)`, 1)
	if edited == src {
		t.Fatalf("the model has no redirect_jobs_seq declaration to edit:\n%s", src)
	}
	if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	if c, o, e := cliRun(t, dsn, "diff", "bump_increment"); c != 0 {
		t.Fatalf("diff: exit %d\n%s%s", c, o, e)
	}
	ups, _ := filepath.Glob(filepath.Join(root, "migrations", "postgres", "*_bump_increment.up.sql"))
	if len(ups) != 1 {
		t.Fatalf("want one migration for the change, got %v", ups)
	}
	body := readText(t, ups[0])
	if !strings.Contains(body, `ALTER SEQUENCE "redirect_jobs_seq"`) || !strings.Contains(body, "INCREMENT BY 10") ||
		strings.Contains(body, "CREATE SEQUENCE") || strings.Contains(body, "DROP SEQUENCE") {
		t.Fatalf("want exactly one ALTER SEQUENCE:\n%s", body)
	}
	if n := strings.Count(body, "SEQUENCE"); n != 1 {
		t.Fatalf("want one sequence statement, got %d:\n%s", n, body)
	}
	if c, o, e := cliRun(t, dsn, "apply"); c != 0 {
		t.Fatalf("apply: exit %d\n%s%s", c, o, e)
	}
	if c, o, e := cliRun(t, dsn, "drift"); c != 0 {
		t.Fatalf("drift after apply: exit %d\n%s%s", c, o, e)
	}
	if c, o, e := cliRun(t, dsn, "diff", "again"); c != 0 || !strings.Contains(o, "no changes") {
		t.Fatalf("diff after apply must be empty: exit %d\n%s%s", c, o, e)
	}
}
