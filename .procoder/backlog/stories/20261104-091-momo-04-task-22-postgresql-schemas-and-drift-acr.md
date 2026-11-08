# momo-04 Task 22: PostgreSQL schemas and drift across configured schemas

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 22: PostgreSQL schemas and drift across configured schemas (`.procoder/plans/momo-04-migrations.md:2490`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-36. Files: `migrate/postgres/schemas_test.go`, `migrate/postgres/render.go`.

## Acceptance criteria

- [x] `TestPostgresSchemas` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-36] `TestPostgresSchemas`: models in two non-public schemas migrate, query and round-trip with no diff, and drift reports objects per schema; fails if an identifier is not schema-qualified.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

CI run 42457640580 on b7ec703 (ci-agent: unit, lint, fuzz-smoke, integration postgres) concluded success; the named tests passed there.

CI run 42457640580 on b7ec703 (ci-agent: unit, lint, fuzz-smoke, integration postgres) concluded success; the named tests passed there.

Red evidence: ci-agent run 42453837637 on throwaway branch worktree-agent-ac8514dccee058855-red (commit a900207, the code under test broken, tests kept, branch since deleted) failed: --- FAIL: TestPostgresSchemas, with tableRef in migrate/postgres/render.go no longer schema-qualifying names.
