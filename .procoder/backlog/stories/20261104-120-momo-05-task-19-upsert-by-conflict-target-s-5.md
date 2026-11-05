# momo-05 Task 19: Upsert by conflict target (S-5)

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 19: Upsert by conflict target (S-5) (`.procoder/plans/momo-05-writes-queries.md:1586`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-5. Files: `upsert_test.go`, `insert.go`. Contributes to S-5; those criteria are verified in the stories that define their named tests.

Scope rule: the builder must render its statement inside a target-table scope via `Builder.withScope` (builder.go), so a correlated subquery may reference the target table.

## Acceptance criteria

- [x] `TestUpsertUpdatesAllColumns` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `TestUpsertUpdatesAllColumns` (upsert_test.go): before implementing, `go test -run Upsert .` failed with `undefined: momo.Upsert`. After: `go test -tags integration -race -count=1 -run Upsert -v .` passes on postgres, and the full `go test -tags integration -race -count=1 .` suite is ok.
- Unit tests: `TestUpsertGoldenSQL` (Postgres and SQLite goldens, with OVERRIDING SYSTEM VALUE on Postgres only), `TestUpsertMySQLUnsupported`, `TestUpsertRefusesReservedColumnsInDoUpdate` (PK, version, soft-delete, created-at, updated-at), `TestUpsertValidation` (mixed identity), `TestUpsertZeroIdentityInsertsNewRow`.
- Implementation is in upsert.go; insert.go gains only the hooks. Rendering still goes through `Builder.withScope` in `InsertQuery.render`.

- `procoder check`: 0 blocking findings. `go vet ./...`, `gofmt -l .` clean, `go test ./...` ok.

### Review fixes

- Updated-at on conflict: the default DO UPDATE SET now ends with updated_at = EXCLUDED.updated_at, and an explicit DoUpdate list that omits it gets it appended. created_at is never assigned. `TestUpsertRefreshesUpdatedAtKeepsCreatedAt` (Postgres, clock t1 then t2) asserts updated_at == t2 and created_at == t1; `TestUpsertExplicitDoUpdateStillRefreshesUpdatedAt` pins the appended assignment. Existing goldens were updated for the new trailing assignment.
- Doc comment on Upsert: version is neither advanced nor checked on conflict, and explicit identity values do not advance the Postgres identity sequence (caller must setval). `TestUpsertLeavesVersionUnchangedOnConflict` pins the unchanged version.
- Added tests: `TestUpsertCompositePrimaryKeyGolden`, `TestUpsertDoNothingGolden` (OnConflict on the PK then DoNothing), `TestUpsertNonPKTargetNeverInsertsIdentity` (non-zero ID with a non-PK target: no identity column, no OVERRIDING SYSTEM VALUE).
- SQLite integration is not available yet, so these ran on Postgres only (SQLite coverage is by the golden tests).
- Story 121 follow-ups made on this branch: the clock is normalized to UTC (not truncated; Postgres stores microseconds, test compares against the microsecond-truncated value), an unsupported timestamp field type (named int64, sql.NullTime) now fails with ErrValidation naming column and type, and tests cover timestampValue/timestampUnset and the clock reaching a Tx insert.
- Verification: `go test -tags integration -race -count=1 .` ok, `go vet ./...`, `gofmt -l .`, `go test ./...` clean.
- Second fresh review: mutations on the updated_at append and the unsupported-timestamp-type check are killed. The UTC test had an empty assertion body; it now fails when `.UTC()` is removed from `execer.now()` (verified) and passes with it. Behaviour note: a created_at/updated_at field of an unsupported type (named int64, sql.NullTime) is now rejected with ErrValidation on insert and update instead of being silently skipped; no model in the repo is affected.
