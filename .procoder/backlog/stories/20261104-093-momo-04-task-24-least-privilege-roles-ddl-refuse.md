# momo-04 Task 24: Least-privilege roles: DDL refused for the application role

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 24: Least-privilege roles: DDL refused for the application role (`.procoder/plans/momo-04-migrations.md:2648`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-47. Files: `migrate/postgres/approle_test.go`.

## Acceptance criteria

- [x] `TestAppRoleCannotDDL` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-47] `TestAppRoleCannotDDL` runs in integration CI, which connects as a non-superuser application role; `TestAppRoleCannotDDL` asserts DDL fails for it; fails if tests run as superuser.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

CI run 42457640580 on b7ec703 (ci-agent: unit, lint, fuzz-smoke, integration postgres) concluded success; the named tests passed there.

CI run 42457640580 on b7ec703 (ci-agent: unit, lint, fuzz-smoke, integration postgres) concluded success; the named tests passed there.

Red evidence: ci-agent run 42453837637 on throwaway branch worktree-agent-ac8514dccee058855-red (commit a900207, the code under test broken, tests kept, branch since deleted) failed: --- FAIL: TestAppRoleCannotDDL, with the testdb harness granting the app role CREATE on the schema.
