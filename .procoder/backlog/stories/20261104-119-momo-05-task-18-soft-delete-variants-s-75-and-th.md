# momo-05 Task 18: Soft delete variants (S-75) and the soft-delete read filter (S-5)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 18: Soft delete variants (S-75) and the soft-delete read filter (S-5) (`.procoder/plans/momo-05-writes-queries.md:1480`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-5, S-75. Files: `softdelete.go`, `softdelete_test.go`. Contributes to S-5; those criteria are verified in the stories that define their named tests.

Scope rule: the builder must render its statement inside a target-table scope via `Builder.withScope` (builder.go), so a correlated subquery may reference the target table.

## Acceptance criteria

- [x] `TestSoftDelete` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSoftDeleteVariants` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-75] `TestSoftDeleteVariants` asserts each marker kind hides rows by default, OnlyDeleted and ForceDelete behave as documented and a unique index permits re-creating a soft-deleted key; fails if ForceDelete soft-deletes.
- [x] plain `Delete` on a soft-delete model fails with ErrValidation (since Task 17); `ForceDelete` is the only hard delete for such models and bypasses that refusal, with a test for both
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red first: before any implementation, `go test -run SoftDelete .` failed to build (`unknown field SoftDeleteKind in struct literal of type momo.Meta[sdUnix]`, `undefined: momo.SoftDeleteUnix`, `undefined: momo.SoftDelete`). After implementation: green.
- `TestSoftDelete` (softdelete_integration_test.go, Postgres): soft-deletes one of two rows, visible=1, WithDeleted=2, OnlyDeleted=1, a repeat SoftDelete affects 0 rows. Passes with `go test -tags integration -race -count=1 .` (ok).
- `TestSoftDeleteVariants` (same file): runs for the timestamp (documents), unix (docs_unix) and boolean (docs_bool) markers. Asserts hidden by default, OnlyDeleted=1, re-creating the soft-deleted key under a partial unique index succeeds, WithDeleted=2, ForceDelete removes 2 rows and leaves 0 (it would fail if ForceDelete soft-deleted).
- `TestSoftDeleteSQLGoldens` (softdelete_test.go, unit): PG, SQLite and MySQL goldens for SoftDelete (timestamp/unix/bool), ForceDelete, OnlyDeleted, default filter, WithDeleted, RETURNING.
- `TestSoftDeleteGuards` (unit) and `TestPlainDeleteRefusedForceDeleteWorks` (integration): plain Delete on a soft-delete model fails with ErrValidation and the row survives; ForceDelete deletes it. Unfiltered SoftDelete/ForceDelete still fail with ErrUnfilteredMutation (hasEffectiveFilter), Set on SoftDelete is refused, SoftDelete on a model without a marker is ErrValidation.
- Deviation: plan says the unix/bool variants run through `WithTableName` (Task 8), which does not exist in this tree; they use separate hand-written models `sdUnix` and `sdBool` instead. The plan's `createSoftVariants(t, h)` takes `db` here. Bool marker is set with a bound `true` argument rather than a literal. Statement time is the Go-side clock (`execer.now()`: WithClock or time.Now), bound as UTC, not the database now(); updated_at is not touched by SoftDelete (documented on SoftDelete).
- Commands: `go vet ./...`, `go vet -tags integration .`, `gofmt -l .` clean; `go test ./...` and the integration run green; `procoder check` gate clean (see commit).
- Review fixes: the generator ignored `soft_delete:unix` and `soft_delete:bool`, so generated models behaved as timestamps. gen/emit_meta.go now emits `SoftDeleteKind: momo.SoftDeleteUnix` or `momo.SoftDeleteBool` (omitted for timestamp, the zero value, so existing goldens are byte-identical). Red first: `TestEmitMetaSoftDeleteKind` (gen/emit_meta_test.go) failed with `unix kind = 0` against the old generator; it passes now, and the generated fixture (gen/testdata/metarun/models, UnixDoc and BoolDoc) compiles and matches its golden. The momo generate check in internal/testmodels/example stays clean.
- Added goldens for `WithDeleted().OnlyDeleted()` (timestamp and bool) expecting the IS NOT NULL and = TRUE predicates. Mutation: replacing the assignment in OnlyDeleted so it no longer clears withDeleted makes both new cases fail; restored, green.
