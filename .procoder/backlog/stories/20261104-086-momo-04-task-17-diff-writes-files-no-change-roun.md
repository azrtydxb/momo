# momo-04 Task 17: Diff writes files; no-change round trip (PostgreSQL)

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Note (Task 9 review): use `migrate.OrderErr` (or `OrderWithViews`), not `Order`, and surface the cycle error it returns.

Implements plan momo-04-migrations Task 17: Diff writes files; no-change round trip (PostgreSQL) (`.procoder/plans/momo-04-migrations.md:2036`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-10, S-11, S-27. Files: `migrate/engine.go`, `migrate/diff_engine_test.go`. Contributes to S-10, S-11, S-27; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestMigrationRoundTrip` exists as written in the plan, failed before the implementation, and passes now
- [ ] The no-change round trip ignores the `momo_migrations` history table (the engine Inspect used by Diff excludes it)
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

