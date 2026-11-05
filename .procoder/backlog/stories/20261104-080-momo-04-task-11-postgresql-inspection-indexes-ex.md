# momo-04 Task 11: PostgreSQL inspection: indexes (expression, partial, method, opclass, invalid)

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 11: PostgreSQL inspection: indexes (expression, partial, method, opclass, invalid) (`.procoder/plans/momo-04-migrations.md:1235`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-12. Files: `migrate/postgres/inspect_index.go`. Contributes to S-12; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestPostgresIndexInspection` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

