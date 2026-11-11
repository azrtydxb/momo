// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

func countRelIndexes(t *testing.T, db *momo.DB, rel string) int {
	t.Helper()
	rows, err := db.Driver().Query(context.Background(),
		`SELECT count(*) FROM pg_indexes WHERE schemaname = current_schema() AND tablename = $1`, []any{rel})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var n int
	if !rows.Next() {
		t.Fatal("no count row")
	}
	if err := rows.Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Catches: inspection modeling an index attached to a parent partitioned index
// as a standalone partition index. A fresh apply of that model creates the
// attached index twice and drift never goes clean (issue #86).
func TestInspectSkipsIndexesAttachedToParentIndex(t *testing.T) {
	db := testopen.Postgres(t)
	mustExec(t, db.Driver(), `CREATE TABLE ev (id bigint, org_id bigint, created_at timestamptz NOT NULL, PRIMARY KEY (id, created_at)) PARTITION BY RANGE (created_at)`)
	mustExec(t, db.Driver(), `CREATE TABLE ev_2026 PARTITION OF ev FOR VALUES FROM ('2026-01-01') TO ('2027-01-01')`)
	mustExec(t, db.Driver(), `CREATE TABLE ev_default PARTITION OF ev DEFAULT`)
	mustExec(t, db.Driver(), `CREATE TABLE ev_own PARTITION OF ev FOR VALUES FROM ('2027-01-01') TO ('2028-01-01')`)
	mustExec(t, db.Driver(), `CREATE INDEX ev_org_time ON ev (org_id, created_at DESC)`)
	// Indexes created directly on a partition are not attached to the parent
	// index: they stay modeled on the partition.
	mustExec(t, db.Driver(), `CREATE INDEX ev_own_org ON ev_own (org_id)`)
	mustExec(t, db.Driver(), `CREATE UNIQUE INDEX ev_own_uniq ON ev_own (org_id, id)`)
	// The attached partition indexes keep their old names after a rename.
	mustExec(t, db.Driver(), `ALTER TABLE ev RENAME COLUMN org_id TO company_id`)

	got := inspectNow(t, db)
	parent, _ := got.Table("ev")
	if parent == nil || len(parent.Indexes) != 1 || parent.Indexes[0].Name != "ev_org_time" {
		t.Fatalf("parent indexes: %+v", parent)
	}
	for _, p := range []string{"ev_2026", "ev_default"} {
		child, _ := got.Table(p)
		if child == nil || len(child.Indexes) != 0 {
			t.Fatalf("%s must not carry the attached index: %+v", p, child)
		}
	}

	own, _ := got.Table("ev_own")
	if own == nil || len(own.Indexes) != 2 || own.Indexes[0].Name != "ev_own_org" || own.Indexes[1].Name != "ev_own_uniq" || !own.Indexes[1].Unique {
		t.Fatalf("ev_own must keep its own indexes and drop the attached one: %+v", own)
	}

	// Rebuild from the inspected model: no duplicates, and nothing to diff.
	mustExec(t, db.Driver(), `DROP TABLE ev CASCADE`)
	applyStatements(t, db, planStatements(t, &schema.Schema{Dialect: "postgres"}, got, migrate.DiffOptions{}))
	for _, p := range []string{"ev_2026", "ev_default"} {
		if n := countRelIndexes(t, db, p); n != 2 {
			t.Fatalf("%s has %d indexes after a fresh apply, want 2 (pkey + attached)", p, n)
		}
	}
	if n := countRelIndexes(t, db, "ev_own"); n != 4 {
		t.Fatalf("ev_own has %d indexes after a fresh apply, want 4 (pkey, attached, own, own unique)", n)
	}
	after := inspectNow(t, db)
	if changes, _, err := migrate.Diff(after, got, migrate.DiffOptions{}); err != nil || len(changes) != 0 {
		t.Fatalf("diff after fresh apply: %v %v\n%s", changes, err, jsonDiff(after, got))
	}
}
