// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/azrtydxb/momo"
	testdb "github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

// countingConn counts the statements sent through Query.
type countingConn struct {
	momo.PinnedConn
	queries atomic.Int64
}

func (c *countingConn) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	c.queries.Add(1)
	return c.PinnedConn.Query(ctx, sql, args)
}

func countInspectQueries(ctx context.Context, t *testing.T, db *momo.DB, schemas []string) int64 {
	t.Helper()
	pc, err := db.Driver().(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pc.Release() }()
	c := &countingConn{PinnedConn: pc}
	if _, err := mpg.New().Inspect(ctx, c, migrate.InspectOptions{Schemas: schemas}); err != nil {
		t.Fatal(err)
	}
	return c.queries.Load()
}

// findTable returns the table named name in the namespace ns ("" for the
// current schema).
func findTable(t *testing.T, s *schema.Schema, ns, name string) *schema.Table {
	t.Helper()
	for i := range s.Tables {
		if s.Tables[i].Schema == ns && s.Tables[i].Name == name {
			return &s.Tables[i]
		}
	}
	t.Fatalf("table %q.%q missing from %d tables", ns, name, len(s.Tables))
	return nil
}

func colNames(t *schema.Table) []string {
	var out []string
	for _, c := range t.Columns {
		out = append(out, c.Name)
	}
	return out
}

func TestPostgresInspectManyTablesAcrossSchemas(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	cur := testdb.Schema(t)
	aux := cur + "_aux"
	mustExec(t, db.Driver(), `CREATE SCHEMA `+aux)
	t.Cleanup(func() { _, _ = db.Driver().Exec(context.Background(), `DROP SCHEMA `+aux+` CASCADE`, nil) })
	mustExec(t, db.Driver(), `CREATE TABLE `+aux+`.orgs (id bigint PRIMARY KEY, name text NOT NULL)`)
	mustExec(t, db.Driver(), `CREATE TABLE accounts (id bigserial PRIMARY KEY, email text NOT NULL, CONSTRAINT accounts_email_uq UNIQUE (email))`)
	mustExec(t, db.Driver(), `CREATE TABLE docs (
        id bigint PRIMARY KEY,
        org_id bigint NOT NULL REFERENCES `+aux+`.orgs(id) ON DELETE CASCADE,
        acct_id bigint REFERENCES accounts(id) ON UPDATE SET NULL,
        slug text NOT NULL,
        CONSTRAINT docs_slug_uq UNIQUE (slug),
        CONSTRAINT docs_id_check CHECK (id > 0)
    )`)
	mustExec(t, db.Driver(), `CREATE INDEX docs_org_idx ON docs (org_id DESC)`)
	mustExec(t, db.Driver(), `CREATE INDEX docs_live_idx ON docs (lower(slug)) WHERE id > 10`)
	mustExec(t, db.Driver(), `CREATE TABLE `+aux+`.audit (id int, note text)`)
	mustExec(t, db.Driver(), `CREATE TABLE no_cols ()`)
	mustExec(t, db.Driver(), `CREATE TABLE `+aux+`.ev (id int NOT NULL, ts timestamptz NOT NULL, PRIMARY KEY (id, ts)) PARTITION BY RANGE (ts)`)
	mustExec(t, db.Driver(), `CREATE INDEX ev_ts_idx ON `+aux+`.ev (ts)`)
	mustExec(t, db.Driver(), `CREATE TABLE `+aux+`.ev_2026 PARTITION OF `+aux+`.ev FOR VALUES FROM ('2026-01-01') TO ('2027-01-01')`)
	mustExec(t, db.Driver(), `CREATE TABLE ev_2027 PARTITION OF `+aux+`.ev FOR VALUES FROM ('2027-01-01') TO ('2028-01-01')`)

	got, err := inspectOnce(ctx, t, db, migrate.InspectOptions{Schemas: []string{cur, aux}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tables) != 8 {
		t.Fatalf("want 8 tables, got %d: %+v", len(got.Tables), got.Tables)
	}

	orgs := findTable(t, got, aux, "orgs")
	if !slices.Equal(colNames(orgs), []string{"id", "name"}) || !slices.Equal(orgs.PrimaryKey, []string{"id"}) {
		t.Fatalf("orgs: %+v", orgs)
	}
	if len(orgs.ForeignKeys)+len(orgs.Uniques)+len(orgs.Checks)+len(orgs.Indexes) != 0 {
		t.Fatalf("orgs must carry only a primary key: %+v", orgs)
	}

	accounts := findTable(t, got, "", "accounts")
	if id, _ := accounts.Column("id"); id.Identity != "serial" {
		t.Fatalf("accounts.id: %+v", id)
	}
	if len(accounts.Uniques) != 1 || accounts.Uniques[0].Name != "accounts_email_uq" ||
		!slices.Equal(accounts.Uniques[0].Columns, []string{"email"}) {
		t.Fatalf("accounts uniques: %+v", accounts.Uniques)
	}

	docs := findTable(t, got, "", "docs")
	if !slices.Equal(colNames(docs), []string{"id", "org_id", "acct_id", "slug"}) {
		t.Fatalf("docs columns: %v", colNames(docs))
	}
	if !slices.Equal(docs.PrimaryKey, []string{"id"}) {
		t.Fatalf("docs pk: %v", docs.PrimaryKey)
	}
	if len(docs.ForeignKeys) != 2 {
		t.Fatalf("docs fks: %+v", docs.ForeignKeys)
	}
	var cross, local *schema.ForeignKey
	for i := range docs.ForeignKeys {
		switch docs.ForeignKeys[i].RefTable {
		case aux + ".orgs":
			cross = &docs.ForeignKeys[i]
		case "accounts":
			local = &docs.ForeignKeys[i]
		}
	}
	if cross == nil || !slices.Equal(cross.Columns, []string{"org_id"}) || !slices.Equal(cross.RefColumns, []string{"id"}) ||
		cross.OnDelete != "CASCADE" || cross.OnUpdate != "" {
		t.Fatalf("cross-schema fk: %+v", docs.ForeignKeys)
	}
	if local == nil || !slices.Equal(local.Columns, []string{"acct_id"}) || local.OnUpdate != "SET NULL" || local.OnDelete != "" {
		t.Fatalf("local fk: %+v", docs.ForeignKeys)
	}
	if len(docs.Uniques) != 1 || docs.Uniques[0].Name != "docs_slug_uq" {
		t.Fatalf("docs uniques: %+v", docs.Uniques)
	}
	if len(docs.Checks) != 1 || docs.Checks[0].Name != "docs_id_check" || docs.Checks[0].Expr != "(id > 0)" {
		t.Fatalf("docs checks: %+v", docs.Checks)
	}
	if len(docs.Indexes) != 2 || docs.Indexes[0].Name != "docs_live_idx" || docs.Indexes[1].Name != "docs_org_idx" {
		t.Fatalf("docs indexes (relname order): %+v", docs.Indexes)
	}
	if live := docs.Indexes[0]; live.Where == "" || len(live.Parts) != 1 || live.Parts[0].Expr == "" || live.Parts[0].Column != "" {
		t.Fatalf("partial expression index: %+v", live)
	}
	if org := docs.Indexes[1]; len(org.Parts) != 1 || org.Parts[0].Column != "org_id" || !org.Parts[0].Desc {
		t.Fatalf("desc index: %+v", org)
	}

	audit := findTable(t, got, aux, "audit")
	if !slices.Equal(colNames(audit), []string{"id", "note"}) || audit.PrimaryKey != nil ||
		audit.Indexes != nil || audit.Partition != nil || audit.PartitionOf != nil {
		t.Fatalf("audit: %+v", audit)
	}

	noCols := findTable(t, got, "", "no_cols")
	if len(noCols.Columns) != 0 || len(noCols.Indexes) != 0 || noCols.PrimaryKey != nil {
		t.Fatalf("no_cols: %+v", noCols)
	}

	ev := findTable(t, got, aux, "ev")
	if ev.Partition == nil || ev.Partition.Strategy != "range" || ev.Partition.Key != "RANGE (ts)" || ev.PartitionOf != nil {
		t.Fatalf("ev partition: %+v", ev)
	}
	if !slices.Equal(ev.PrimaryKey, []string{"id", "ts"}) {
		t.Fatalf("ev pk: %v", ev.PrimaryKey)
	}
	if len(ev.Indexes) != 1 || ev.Indexes[0].Name != "ev_ts_idx" {
		t.Fatalf("ev indexes: %+v", ev.Indexes)
	}
	// A partition child inherits the key and the index from its parent; neither
	// is reported on the child.
	for _, c := range []struct{ ns, name string }{{aux, "ev_2026"}, {"", "ev_2027"}} {
		child := findTable(t, got, c.ns, c.name)
		if child.PartitionOf == nil || child.PartitionOf.Parent != aux+".ev" ||
			!strings.HasPrefix(child.PartitionOf.Bound, "FOR VALUES FROM (") {
			t.Fatalf("%s partition-of: %+v", c.name, child.PartitionOf)
		}
		if child.Partition != nil || child.PrimaryKey != nil || len(child.Indexes) != 0 {
			t.Fatalf("%s must not carry inherited objects: %+v", c.name, child)
		}
		if !slices.Equal(colNames(child), []string{"id", "ts"}) {
			t.Fatalf("%s columns: %v", c.name, colNames(child))
		}
	}
}

// The per-table metadata queries (columns, constraints, partitions and the
// index list) must not grow with the number of tables. The tables here have no
// secondary index and no serial column: an index's key columns are still read
// with one query per index, and the owned-sequence check runs once per column
// with a nextval default, so both are left out of this bound.
func TestPostgresInspectPerTableMetadataQueriesIndependentOfTableCount(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	cur := testdb.Schema(t)
	aux := cur + "_aux"
	mustExec(t, db.Driver(), `CREATE SCHEMA `+aux)
	t.Cleanup(func() { _, _ = db.Driver().Exec(context.Background(), `DROP SCHEMA `+aux+` CASCADE`, nil) })
	mustExec(t, db.Driver(), `CREATE TABLE `+aux+`.base (id bigint PRIMARY KEY)`)
	mustExec(t, db.Driver(), `CREATE TABLE first_t (id bigint PRIMARY KEY, base_id bigint REFERENCES `+aux+`.base(id), CONSTRAINT first_t_chk CHECK (id > 0))`)
	schemas := []string{cur, aux}
	few := countInspectQueries(ctx, t, db, schemas)

	for _, name := range []string{"t_a", "t_b", "t_c", "t_d"} {
		mustExec(t, db.Driver(), `CREATE TABLE `+name+` (id bigint PRIMARY KEY, base_id bigint REFERENCES `+aux+`.base(id), CONSTRAINT `+name+`_chk CHECK (id > 0))`)
		mustExec(t, db.Driver(), `CREATE TABLE `+aux+`.`+name+` (id bigint PRIMARY KEY, v int NOT NULL)`)
	}
	mustExec(t, db.Driver(), `CREATE TABLE ev (ts timestamptz NOT NULL) PARTITION BY RANGE (ts)`)
	mustExec(t, db.Driver(), `CREATE TABLE ev_2026 PARTITION OF ev FOR VALUES FROM ('2026-01-01') TO ('2027-01-01')`)
	many := countInspectQueries(ctx, t, db, schemas)
	if few != many {
		t.Fatalf("inspection sent %d statements for 2 tables and %d for 12: the per-table metadata queries must not depend on the table count", few, many)
	}
}
