# momo-03 Task 1: testopen package with Postgres and Each (milestone M1)

Status: open
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-03-drivers Task 1: testopen package with Postgres and Each (milestone M1) (`.procoder/plans/momo-03-drivers.md:50`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `internal/testdb/testopen/testopen.go`, `internal/testdb/testopen/testopen_test.go`.

## Acceptance criteria

- [ ] `TestEachRunsPostgres` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestOptionsReachDB` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestEachPassesOptions` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

