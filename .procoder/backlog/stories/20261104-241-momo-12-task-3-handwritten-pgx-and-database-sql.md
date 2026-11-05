# momo-12 Task 3: Handwritten pgx and database/sql adapters

Status: open
Created: 2026-11-04
Epic: momo-12-performance
Sprint: -

## Description

Implements plan momo-12-performance Task 3: Handwritten pgx and database/sql adapters (`.procoder/plans/momo-12-performance.md:223`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-19. Files: `bench/adapters/pgxhand/pgxhand.go`, `bench/adapters/sqlhand/sqlhand.go`, `bench/adapters/pool.go`. Contributes to S-19; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestPgxhandGolden` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

