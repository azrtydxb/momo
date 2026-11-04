# momo-04 Task 11: PostgreSQL inspection: indexes (expression, partial, method, opclass, invalid)

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 11: PostgreSQL inspection: indexes (expression, partial, method, opclass, invalid) (`.procoder/plans/momo-04-migrations.md:1235`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-12. Files: `migrate/postgres/inspect_index.go`.

## Acceptance criteria

- [ ] `TestPostgresIndexInspection` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-12] `TestPostgresObjects` on real PostgreSQL+pgvector: vector extension, enum, partial index, expression index, HNSW index with opclass and a CONCURRENTLY index apply and round-trip with no diff; fails if CONCURRENTLY runs inside a transaction.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

