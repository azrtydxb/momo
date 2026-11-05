# momo-04 Task 11: PostgreSQL inspection: indexes (expression, partial, method, opclass, invalid)

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 11: PostgreSQL inspection: indexes (expression, partial, method, opclass, invalid) (`.procoder/plans/momo-04-migrations.md:1235`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-12. Files: `migrate/postgres/inspect_index.go`. Contributes to S-12; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestPostgresIndexInspection` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- FAIL before implementation: go test -tags integration ./migrate/postgres/ failed to build: Dialect{}.inspectIndexes undefined.
- PASS after: TestPostgresIndexInspection, TestPostgresIndexInspectionDetails and TestInspectIndexesReportsInvalid pass; the integration run of ./migrate/... ./schema/... is ok.
- Mutation: inverting the DESC NULLS LAST rule made TestPostgresIndexInspectionDetails fail; reverted.
- Files: migrate/postgres/inspect_index.go, inspect_index_test.go, inspect_index_internal_test.go, inspect.go (wiring).
