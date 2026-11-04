# momo-08 Task 3: Writes on MySQL: generated keys, upsert, conflict targets

Status: open
Created: 2026-11-04
Epic: momo-08-mysql
Sprint: -

## Description

Implements plan momo-08-mysql Task 3: Writes on MySQL: generated keys, upsert, conflict targets (`.procoder/plans/momo-08-mysql.md:346`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-5, S-59, S-84. Files: `mysql_render.go`, `mysql_render_test.go`, `ON CONFLICT`, `driver/mysqldriver/generated_keys.go`.

## Acceptance criteria

- [ ] `TestMySQLUpsertRendering` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestMySQLConflictTargetRule` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestMySQLGeneratedKeys` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-5] [S-9] `TestBulkInsertChunking` inserts more rows than one statement's parameter limit and counts them; `TestCopyInsert` (PostgreSQL) inserts identical data via COPY; fails if chunk size ignores the dialect limit.
- [ ] [S-59] `TestMySQLIntegration` runs the shared query/write/transaction/error suites against MySQL and MariaDB containers and asserts identical results to PostgreSQL or ErrUnsupported per the capability matrix; fails if a clause is silently dropped.
- [ ] [S-84] `TestCreateGraphAndFindOrCreate` asserts only listed relations are written, FindOrCreate is race-safe under concurrency (ON CONFLICT based), and Upsert updates all columns; fails if an unlisted relation is saved.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

