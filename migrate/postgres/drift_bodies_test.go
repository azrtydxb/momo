// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"testing"

	"github.com/azrtydxb/momo"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// baselinedDeclared creates the objects of declaredObjects by hand, baselines
// them (the snapshot declares their text in a different spelling than the
// server reports) and returns the database and the migration directory.
func baselinedDeclared(t *testing.T) (*momo.DB, fs.FS) {
	t.Helper()
	db := testopen.Postgres(t)
	createDeclaredLive(t, db.Driver())
	fsys := canonFS(t, declaredObjects())
	if err := canonEngine(t, db.Driver(), fsys).Baseline(context.Background(), canonVersion, false); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	return db, fsys
}

// changedBy returns the Changed entries of rep as "kind:name" to Detail.
func changedBy(rep *migrate.DriftReport) map[string]string {
	out := map[string]string{}
	for _, c := range rep.Changed {
		out[c.Object.Kind+":"+c.Object.Name] = c.Detail
	}
	return out
}

// A body declared in another spelling than the server reports is not drift,
// and nothing is left unverified.
func TestDriftBodiesEquivalentSpellingIsClean(t *testing.T) {
	db, fsys := baselinedDeclared(t)
	rep, err := canonEngine(t, db.Driver(), fsys).Drift(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean() || len(rep.Unverified) != 0 || len(rep.CanonicalizeSkipped) != 0 {
		t.Fatalf("want a clean report with nothing unverified or skipped, got %+v", rep)
	}
}

// Catches: a drift report that compares view, trigger and policy identity only,
// so a policy loosened to USING (true) out of band leaves the report Clean.
func TestDriftReportsOutOfBandBodyChanges(t *testing.T) {
	cases := []struct {
		name   string
		ddl    []string
		key    string
		detail string
	}{
		{
			name:   "policy using",
			ddl:    []string{`DROP POLICY docs_pos ON docs`, `CREATE POLICY docs_pos ON docs FOR SELECT USING (true)`},
			key:    "policy:docs.docs_pos",
			detail: "using",
		},
		{
			name:   "view query",
			ddl:    []string{`CREATE OR REPLACE VIEW v_docs AS SELECT id, title FROM docs`},
			key:    "view:v_docs",
			detail: "query",
		},
		{
			name:   "materialized view query",
			ddl:    []string{`DROP MATERIALIZED VIEW m_count`, `CREATE MATERIALIZED VIEW m_count AS SELECT count(*) + 1 AS n FROM docs`},
			key:    "view:m_count",
			detail: "query",
		},
		{
			name:   "trigger when",
			ddl:    []string{`DROP TRIGGER docs_touch ON docs`, `CREATE TRIGGER docs_touch BEFORE INSERT ON docs FOR EACH ROW WHEN (NEW.id > 100) EXECUTE FUNCTION touch()`},
			key:    "trigger:docs.docs_touch",
			detail: "when",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db, fsys := baselinedDeclared(t)
			for _, q := range c.ddl {
				mustExec(t, db.Driver(), q)
			}
			rep, err := canonEngine(t, db.Driver(), fsys).Drift(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			got, ok := changedBy(rep)[c.key]
			if !ok || !strings.Contains(got, c.detail) {
				t.Fatalf("want %s changed with a detail naming %q, got %+v", c.key, c.detail, rep)
			}
			if len(rep.Changed) != 1 || len(rep.Missing) != 0 || len(rep.Unmanaged) != 0 {
				t.Fatalf("only %s may differ: %+v", c.key, rep)
			}
		})
	}
}

// checkPolicyObjects is a model with an INSERT policy that has a WITH CHECK.
func checkPolicyObjects() *schema.Schema {
	return &schema.Schema{
		Dialect:    "postgres",
		Extensions: []schema.Extension{{Name: "vector"}},
		Tables: []schema.Table{{
			Name:       "docs",
			Columns:    []schema.Column{{Name: "id", Type: "bigint"}},
			PrimaryKey: []string{"id"},
			RLS:        &schema.RLS{Enabled: true},
		}},
		Policies: []schema.Policy{{Table: "docs", Name: "docs_ins", Command: "INSERT", Permissive: true, Check: "id>0"}},
	}
}

func TestDriftReportsOutOfBandPolicyCheckChange(t *testing.T) {
	db := testopen.Postgres(t)
	for _, q := range []string{
		`CREATE TABLE docs (id bigint PRIMARY KEY)`,
		`ALTER TABLE docs ENABLE ROW LEVEL SECURITY`,
		`CREATE POLICY docs_ins ON docs FOR INSERT WITH CHECK (id > 0)`,
	} {
		mustExec(t, db.Driver(), q)
	}
	fsys := canonFS(t, checkPolicyObjects())
	e := canonEngine(t, db.Driver(), fsys)
	if err := e.Baseline(context.Background(), canonVersion, false); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if rep, err := e.Drift(context.Background()); err != nil || !rep.Clean() {
		t.Fatalf("before the change: %+v %v", rep, err)
	}
	mustExec(t, db.Driver(), `DROP POLICY docs_ins ON docs`)
	mustExec(t, db.Driver(), `CREATE POLICY docs_ins ON docs FOR INSERT WITH CHECK (true)`)
	rep, err := e.Drift(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := changedBy(rep)["policy:docs.docs_ins"]; !ok || !strings.Contains(got, "check") {
		t.Fatalf("want the policy changed with a detail naming check, got %+v", rep)
	}
}

// Drift stays read-only: every DDL it runs sits between a BEGIN and the
// ROLLBACK that ends it, so nothing is committed and nothing is created in the
// application schema.
func TestDriftBodiesRunsDDLOnlyInRolledBackTransaction(t *testing.T) {
	db, fsys := baselinedDeclared(t)
	ctx := context.Background()
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
		case "COMMIT", "INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFRESH":
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
	rows, err := db.Driver().Query(ctx, `SELECT count(*) FROM pg_namespace WHERE nspname LIKE 'momo_canon_%'`, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var left int
	if rows.Next() {
		if err := rows.Scan(&left); err != nil {
			t.Fatal(err)
		}
	}
	if left != 0 {
		t.Fatalf("%d scratch schemas left behind", left)
	}
}

// The privilege Drift needs to compare view, trigger and policy bodies is
// CREATE on the database (the scratch schema). The non-superuser application
// role lacks it: Drift returns an error wrapping ErrCanonicalizePrivilege that
// names it, and never reports a clean drift for bodies it could not compare.
func TestDriftBodiesNeedCreateOnDatabase(t *testing.T) {
	admin := testopen.Postgres(t)
	// The role's default privileges are installed when it is opened, so open it
	// before the administrator creates the tables.
	app := openAppRole(t)
	createDeclaredLive(t, admin.Driver())
	fsys := canonFS(t, declaredObjects())
	if err := canonEngine(t, admin.Driver(), fsys).Baseline(context.Background(), canonVersion, false); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	// Loosen the policy so a skipped comparison would be a missed finding.
	mustExec(t, admin.Driver(), `DROP POLICY docs_pos ON docs`)
	mustExec(t, admin.Driver(), `CREATE POLICY docs_pos ON docs FOR SELECT USING (true)`)

	rep, err := canonEngine(t, app.Driver(), fsys).Drift(context.Background())
	if !errors.Is(err, migrate.ErrCanonicalizePrivilege) {
		t.Fatalf("want ErrCanonicalizePrivilege, got %+v %v", rep, err)
	}
	for _, want := range []string{"CREATE", "database", "privilege"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the error must mention %q: %v", want, err)
		}
	}
}

// A snapshot without view, trigger or policy bodies needs no privilege for
// them: the application role still gets a report, with the table text it could
// not canonicalize listed in CanonicalizeSkipped.
func TestDriftTablesOnlyRunsAsApplicationRole(t *testing.T) {
	admin := testopen.Postgres(t)
	app := openAppRole(t)
	mustExec(t, admin.Driver(), `CREATE TABLE t (n int CONSTRAINT t_chk CHECK (n IN (1, 2)))`)
	s := canonSchema(schema.Table{Name: "t", Columns: []schema.Column{{Name: "n", Type: "integer", Nullable: true}}, Checks: []schema.Check{{Name: "t_chk", Expr: "n IN (1, 2)"}}})
	fsys := canonFS(t, s)
	if err := canonEngine(t, admin.Driver(), fsys).Baseline(context.Background(), canonVersion, false); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	rep, err := canonEngine(t, app.Driver(), fsys).Drift(context.Background())
	if err != nil {
		t.Fatalf("a snapshot without bodies must not need CREATE: %v", err)
	}
	if got := skippedNames(rep); len(got) != 1 || got[0] != "table:t" {
		t.Fatalf("want table t listed in CanonicalizeSkipped, got %v (%+v)", got, rep)
	}
}
