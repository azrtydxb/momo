# momo-04 Task 10: PostgreSQL inspection: schemas, extensions, enums, tables, columns, constraints

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 10: PostgreSQL inspection: schemas, extensions, enums, tables, columns, constraints (`.procoder/plans/momo-04-migrations.md:1128`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-12, S-36, S-74. Files: `migrate/postgres/dialect.go`.

## Acceptance criteria

- [ ] `TestPostgresInspectColumnsAndConstraints` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-12] `TestPostgresObjects` on real PostgreSQL+pgvector: vector extension, enum, partial index, expression index, HNSW index with opclass and a CONCURRENTLY index apply and round-trip with no diff; fails if CONCURRENTLY runs inside a transaction.
- [ ] [S-36] `TestPostgresSchemas`: models in two non-public schemas migrate, query and round-trip with no diff, and drift reports objects per schema; fails if an identifier is not schema-qualified.
- [ ] [S-74] `TestFKActions` asserts each action's effect on every dialect and that the no-constraint option produces no FK in inspection; fails if actions are lost in a round trip.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

