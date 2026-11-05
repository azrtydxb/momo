# momo-04 Task 13: PostgreSQL render for all change kinds

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Note (Task 9 review): use `migrate.OrderErr` (or `OrderWithViews`), not `Order`, and surface the cycle error it returns.

Implements plan momo-04-migrations Task 13: PostgreSQL render for all change kinds (`.procoder/plans/momo-04-migrations.md:1401`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-12, S-37, S-38, S-39. Files: `migrate/postgres/render.go`. Contributes to S-37, S-38; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestPostgresObjects` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestPostgresConcurrentIndexNoTx` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestManagedFunctionsTriggersPartitions` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-12] `TestPostgresObjects` on real PostgreSQL+pgvector: vector extension, enum, partial index, expression index, HNSW index with opclass and a CONCURRENTLY index apply and round-trip with no diff; fails if CONCURRENTLY runs inside a transaction.
- [ ] [S-39] `TestManagedFunctionsTriggersPartitions`: a function, trigger and partitioned table are created by migrations, round-trip with no diff, and a changed function body produces CREATE OR REPLACE; fails if they are reported as unmanaged.
- [ ] `procoder check` gate is clean and the task's commit is made
- [ ] PostgreSQL render emits `Column.Collate` (`COLLATE`) and `Column.Comment` (`COMMENT ON COLUMN`) for create and alter, with a round-trip test against inspection

## Evidence

- Note: this task carries the inspection and render part of S-74 (FK ON DELETE/ON UPDATE on every dialect, and no FK in inspection for the no-constraint option); the SQLite render story carries the SQLite half. Task 8 (story 077) covers only the diff part.
