# momo-04 Task 20: AutoMigrate additive

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 20: AutoMigrate additive (`.procoder/plans/momo-04-migrations.md:2272`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-11, S-14. Files: `migrate/automigrate.go`. Contributes to S-11; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestAutoMigrateAdditive` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-14] `TestAutoMigrateAdditive`: AutoMigrate creates missing tables/columns/indexes, returns skipped changes for a type change and a removed column, and the column still exists afterwards; fails if AutoMigrate ever emits DROP or ALTER TYPE.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red first: `go test -tags integration ./migrate -run AutoMigrate` failed to compile with "e.AutoMigrate undefined" before `migrate/automigrate.go` existed; green after (`-race`, count=1).
- `TestAutoMigrateAdditive` follows the plan test (adapted to the real helpers `pinned` and `migratetest.NewCountingDriver`), and also asserts the skip reasons, that `things.kind` stays text, that a second run applies nothing and emits no DDL, and that no history table is created.
- Further tests: `TestAutoMigrateNeverDestroys` (unmanaged table, NOT NULL column without default and unique index on an existing table are skipped), `TestAutoMigrateNewTableKeepsFKAndRLS`, `TestAutoMigrateConcurrent` (6 engines at once, all succeed), `TestAutoMigrateNilDesired`.
- Deviations from the plan's kept list: create_schema and create_extension are kept (a new table may need them); a new table keeps its own add_fk, set_rls and create_policy (otherwise a created table would silently lack RLS); a unique index on an existing table is skipped.
- `go test -tags integration -race -count=1 ./migrate/... ./schema/...`, `go vet ./...`, `go vet -tags integration ./...`, `gofmt -l .` and `go test ./...` are clean.
- Review fixes: a kept change that depends on a skipped one is now skipped too, iterating to a fixed point (`migrate/automigrate_deps.go`), with reason "depends on a skipped change: kind name". Covered: index on a skipped add_column or skipped type change, FK/policy/RLS on a skipped table, columns typed by a skipped enum or extension, transitive chains. Tests: `TestAutoMigrateSkipsDependentOfSkipped` (the reported repro: NOT NULL column without default plus an index on it), `TestAutoMigrateSkipsIndexOnSkippedTypeChange`, `TestSkipDependentsKinds`.
- add_column that rewrites the table (stored generated column, identity column, default other than a constant literal, NULL, now() or CURRENT_TIMESTAMP) is skipped with a reason: `TestAutoMigrateSkipsRewritingAddColumn`, `TestDefaultNeedsNoRewrite`, `TestAddColumnRewrites`.
- The AutoMigrate doc now states that non-unique indexes on existing tables are built CONCURRENTLY only when the schema index sets Concurrently (otherwise the build blocks writes), and that AutoMigrate ignores migration history (dirty history, checksums). Plan Task 20 is silent on a history gate, so none was added.
- Integration `-race` run of `./migrate/...`, `go vet ./...`, `go vet -tags integration ./migrate/...`, `gofmt -l .` and `go test ./...` are clean after these fixes.
