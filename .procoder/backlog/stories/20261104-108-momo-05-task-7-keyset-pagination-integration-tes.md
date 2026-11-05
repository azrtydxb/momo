# momo-05 Task 7: Keyset pagination integration tests (consumes momo-01 After)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 7: Keyset pagination integration tests (consumes momo-01 After) (`.procoder/plans/momo-05-writes-queries.md:532`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `internal/suite/keyset.go`, `integration_keyset_test.go`.

## Acceptance criteria

- [ ] `TestKeysetPaginationMixedDirections` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestKeysetRejectsNullableKey` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

