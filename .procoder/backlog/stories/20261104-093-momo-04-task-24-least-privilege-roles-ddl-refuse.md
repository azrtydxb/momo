# momo-04 Task 24: Least-privilege roles: DDL refused for the application role

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 24: Least-privilege roles: DDL refused for the application role (`.procoder/plans/momo-04-migrations.md:2648`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-47. Files: `migrate/postgres/approle_test.go`.

## Acceptance criteria

- [ ] `TestAppRoleCannotDDL` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-47] `TestAppRoleCannotDDL` runs in integration CI, which connects as a non-superuser application role; `TestAppRoleCannotDDL` asserts DDL fails for it; fails if tests run as superuser.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

CI run 42414158119 on 764d4d1 (workflow ci-agent: unit, lint, fuzz-smoke, integration postgres) concluded success; the named tests ran and passed there.

Left open: the criterion that the named test "failed before the implementation" cannot be ticked. The test and the implementation were not pushed as a red commit followed by a green one, so no CI run shows the failing state. Not ticked rather than weakened.
