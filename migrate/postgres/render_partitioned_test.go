// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"testing"

	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// TestRenderAddUniqueByTableKind: an unpartitioned table gets the concurrent
// unique index adopted with USING INDEX; a partitioned one, where CONCURRENTLY
// is rejected, gets a plain transactional ADD CONSTRAINT.
func TestRenderAddUniqueByTableKind(t *testing.T) {
	u := &schema.Unique{Name: "u", Columns: []string{"a", "b"}}
	plain := renderSQL(t, migrate.Change{Kind: migrate.AddUnique, Table: "t", Name: "u", After: u})
	if len(plain) != 2 || plain[0].SQL != `CREATE UNIQUE INDEX CONCURRENTLY "u" ON "t" ("a", "b")` || !plain[0].NoTx ||
		plain[1].SQL != `ALTER TABLE "t" ADD CONSTRAINT "u" UNIQUE USING INDEX "u"` || !plain[1].NoTx {
		t.Fatalf("unpartitioned: %+v", plain)
	}
	part := renderSQL(t, migrate.Change{Kind: migrate.AddUnique, Table: "t", Name: "u", After: u, Partitioned: true})
	if len(part) != 1 || part[0].SQL != `ALTER TABLE "t" ADD CONSTRAINT "u" UNIQUE ("a", "b")` || part[0].NoTx {
		t.Fatalf("partitioned: %+v", part)
	}
}

// TestRenderIndexOnPartitionedTable: CREATE and DROP INDEX CONCURRENTLY are
// rejected on a partitioned table, so Partitioned renders them plain and
// transactional even when the index asks for Concurrently.
func TestRenderIndexOnPartitionedTable(t *testing.T) {
	ix := &schema.Index{Name: "i", Concurrently: true, Parts: []schema.IndexPart{{Column: "a"}}}
	create := renderSQL(t, migrate.Change{Kind: migrate.CreateIndex, Table: "t", Name: "i", After: ix, Partitioned: true})
	if create[0].SQL != `CREATE INDEX "i" ON "t" ("a")` || create[0].NoTx {
		t.Fatalf("create: %+v", create)
	}
	drop := renderSQL(t, migrate.Change{Kind: migrate.DropIndex, Table: "t", Name: "i", Before: ix, Partitioned: true})
	if drop[0].SQL != `DROP INDEX "i"` || drop[0].NoTx {
		t.Fatalf("drop: %+v", drop)
	}
	// Unpartitioned tables keep CONCURRENTLY.
	plain := renderSQL(t, migrate.Change{Kind: migrate.CreateIndex, Table: "t", Name: "i", After: ix})
	if !plain[0].NoTx {
		t.Fatalf("unpartitioned: %+v", plain)
	}
}

// TestRenderDropIfExists: a drop flagged IfExists renders idempotently.
func TestRenderDropIfExists(t *testing.T) {
	ix := &schema.Index{Name: "i", Concurrently: true}
	st := renderSQL(t,
		migrate.Change{Kind: migrate.DropIndex, Table: "t", Name: "i", Before: ix, IfExists: true},
		migrate.Change{Kind: migrate.DropCheck, Table: "t", Name: "c", IfExists: true},
	)
	if st[0].SQL != `DROP INDEX CONCURRENTLY IF EXISTS "i"` || !st[0].NoTx || st[1].SQL != `ALTER TABLE "t" DROP CONSTRAINT IF EXISTS "c"` {
		t.Fatalf("got %+v", st)
	}
}
