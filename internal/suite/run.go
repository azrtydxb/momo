// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"testing"

	"github.com/azrtydxb/momo"
)

// Queries runs the query features that exist today as named subtests on the
// same db, each starting with CreateDocuments: joins and self-joins,
// aggregates, subqueries and CTEs, RETURNING, and keyset pagination.
//
// Members of the plan's Task 35 list whose feature stories have not merged yet
// (locking, hints, table override, raw arguments, JSON and array
// operators, window functions) are added to this list by those stories; a
// member never exists as a skipped stub.
func Queries(t *testing.T, db *momo.DB, caps Caps) {
	t.Run("joins", func(t *testing.T) { JoinsAliasesSelfJoin(t, db, caps) })
	t.Run("aggregates", func(t *testing.T) { AggregatesGroupHaving(t, db, caps) })
	t.Run("subqueries", func(t *testing.T) { SubqueriesCTEsRecursive(t, db, caps) })
	t.Run("returning", func(t *testing.T) { ReturningRendersAndScans(t, db, caps) })
	t.Run("raw", func(t *testing.T) { RawArgs(t, db, caps) })
	t.Run("keyset", func(t *testing.T) {
		t.Run("mixed-directions", func(t *testing.T) { KeysetPaginationMixedDirections(t, db, caps) })
		t.Run("nullable-key", func(t *testing.T) { KeysetRejectsNullableKey(t, db, caps) })
	})
}

// Writes runs the write features that exist today as named subtests on the
// same db: model insert with omit and conflict, update guardrails, optimistic
// lock, delete, soft delete and its variants, upsert and automatic timestamps.
// Bulk insert, COPY, write states, defaults and validators, MERGE and bulk
// update are added by their stories.
func Writes(t *testing.T, db *momo.DB, caps Caps) {
	t.Run("insert", func(t *testing.T) { InsertModelOmitAndConflict(t, db, caps) })
	t.Run("guardrails", func(t *testing.T) { MutationGuardrails(t, db, caps) })
	t.Run("optimistic-lock", func(t *testing.T) { OptimisticLock(t, db, caps) })
	t.Run("delete", func(t *testing.T) { DeleteByModelAndGuard(t, db, caps) })
	t.Run("soft-delete", func(t *testing.T) { SoftDelete(t, db, caps) })
	t.Run("soft-delete-variants", func(t *testing.T) { SoftDeleteVariants(t, db, caps) })
	t.Run("upsert", func(t *testing.T) { UpsertUpdatesAllColumns(t, db, caps) })
	t.Run("timestamps", func(t *testing.T) { AutoTimestamps(t, db, caps) })
}

// RunAll runs Transactions, Errors, Queries and Writes in that order on the
// same db, each member creating its own tables. Every table is created with an
// unqualified name, so RunAll needs a connection whose search_path is the same
// on every server connection that serves it. A transaction-pooled connection
// cannot rely on a startup search_path (the PgBouncer image cannot track it),
// so the PgBouncer test runs RunAll in a per-run scratch database with the
// default search_path instead.
func RunAll(t *testing.T, db *momo.DB, caps Caps) {
	t.Run("transactions", func(t *testing.T) { Transactions(t, db, caps) })
	t.Run("errors", func(t *testing.T) { Errors(t, db, caps) })
	t.Run("queries", func(t *testing.T) { Queries(t, db, caps) })
	t.Run("writes", func(t *testing.T) { Writes(t, db, caps) })
}
