# momo-04 Task 22: PostgreSQL schemas and drift across configured schemas

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 22: PostgreSQL schemas and drift across configured schemas (`.procoder/plans/momo-04-migrations.md:2490`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-36. Files: `migrate/postgres/schemas_test.go`, `migrate/postgres/render.go`.

## Acceptance criteria

- [ ] `TestPostgresSchemas` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-36] `TestPostgresSchemas`: models in two non-public schemas migrate, query and round-trip with no diff, and drift reports objects per schema; fails if an identifier is not schema-qualified.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

CI run 42414158119 on 764d4d1 (workflow ci-agent: unit, lint, fuzz-smoke, integration postgres) concluded success; the named tests ran and passed there.

Left open: the criterion that the test "failed before the implementation" is not shown. The code under test already existed before this story and only the test was added, so there is no implementation to revert and no red run exists. Not ticked rather than weakened.
