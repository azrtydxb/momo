# momo-04 Task 26: Migration lint with severities and safer alternatives

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 26: Migration lint with severities and safer alternatives (`.procoder/plans/momo-04-migrations.md:2764`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-28. Files: `migrate/lint.go`.

## Acceptance criteria

- [ ] `TestMigrateLint` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-28] `TestMigrateLint` asserts each listed risky operation is reported with severity and an alternative and that `momo migrate lint` exits 1 on errors; `TestLockTimeoutApplied` asserts `SHOW lock_timeout` inside an applying migration equals the configured value; fails if apply runs without timeouts on PostgreSQL.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

