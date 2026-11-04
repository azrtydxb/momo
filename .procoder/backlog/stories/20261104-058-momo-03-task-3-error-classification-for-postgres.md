# momo-03 Task 3: Error classification for PostgreSQL (pgx) with value redaction at the source

Status: open
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: -

## Description

Implements plan momo-03-drivers Task 3: Error classification for PostgreSQL (pgx) with value redaction at the source (`.procoder/plans/momo-03-drivers.md:268`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `driver/pgxdriver/classify.go`, `driver/pgxdriver/classify_test.go`.

## Acceptance criteria

- [ ] `TestClassifyUniqueRedactsDetail` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestClassifySerialization` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

