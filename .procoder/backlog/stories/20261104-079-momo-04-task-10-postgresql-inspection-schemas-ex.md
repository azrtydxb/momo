# momo-04 Task 10: PostgreSQL inspection: schemas, extensions, enums, tables, columns, constraints

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 10: PostgreSQL inspection: schemas, extensions, enums, tables, columns, constraints (`.procoder/plans/momo-04-migrations.md:1128`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-12, S-36, S-74. Files: `migrate/postgres/dialect.go`. Contributes to S-12, S-36, S-74; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestPostgresInspectColumnsAndConstraints` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

