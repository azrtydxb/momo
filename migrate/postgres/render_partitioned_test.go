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
