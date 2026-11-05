# momo-04 Task 3: Migrate core types, errors, options, Dialect interface, test helpers

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 3: Migrate core types, errors, options, Dialect interface, test helpers (`.procoder/plans/momo-04-migrations.md:359`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-8, S-10. Files: `migrate/types.go`. Contributes to S-8, S-10; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestSentinelsAreDistinct` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- TestSentinelsAreDistinct FAIL before (undefined: ErrStagedRequired, ErrNoDown, ErrNoSnapshot, ErrRebuildBlocked, DriftReport); PASS after.
- Mutation: ErrNoDown = ErrDirty made the test fail (sentinels 1 and 6 are equal); reverted.
- go build, go vet, go test -race pass; procoder check: 0 blocking findings.
- Commit: feat(migrate): core types, errors and dialect interface (types.go, options.go, dialect.go, types_test.go; errors.go aligned to plan text).
- Not done here (blocked on later tasks): migratetest and helpers_test.go need Engine, HistoryRow, ComputeSum and the postgres dialect.
