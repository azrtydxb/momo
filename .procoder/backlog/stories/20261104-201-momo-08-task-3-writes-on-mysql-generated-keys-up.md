# momo-08 Task 3: Writes on MySQL: generated keys, upsert, conflict targets

Status: open
Created: 2026-11-04
Epic: momo-08-mysql
Sprint: -

## Description

Implements plan momo-08-mysql Task 3: Writes on MySQL: generated keys, upsert, conflict targets (`.procoder/plans/momo-08-mysql.md:346`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-5, S-59, S-84. Files: `mysql_render.go`, `mysql_render_test.go`, `ON CONFLICT`, `driver/mysqldriver/generated_keys.go`. Contributes to S-5, S-59, S-84; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestMySQLUpsertRendering` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestMySQLConflictTargetRule` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestMySQLGeneratedKeys` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

