# momo-05 Task 19: Upsert by conflict target (S-5)

Status: open
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
