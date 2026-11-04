# momo-04 Task 15: Runner, history table, failure recovery, resolve

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 15: Runner, history table, failure recovery, resolve (`.procoder/plans/momo-04-migrations.md:1720`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-8, S-11, S-22. Files: `migrate/history.go`, `migrate/options.go`.

## Acceptance criteria

- [ ] `TestNonTransactionalFailureRecovery` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestTransactionalFailureRolledBack` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestHistoryFormatVersion` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-8] `TestErrorClassification` (both dialects): each error kind matches errors.Is and the driver error is reachable with errors.As (*pgconn.PgError, sqlite3.Error); `TestEmptyResultNotNotFound` asserts All on zero rows returns nil error; fails if classification loses the cause.
- [ ] [S-11] `TestStagedChange` on a populated DB: add nullable column → data migration backfill → SET NOT NULL applies and all rows satisfy the constraint; fails if diff emits NOT NULL directly for a populated table without a default.
- [ ] [S-22] `TestReleaseConfig` parses release-please-config.json and asserts the pre-1.0 minor-bump setting and the changelog path; `TestFormatVersions` asserts that a snapshot, checksum file and history table from format N-1 are read by the current version and that a future format version fails with an error naming the required Momo version; fails if a persisted format lacks its version field.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

