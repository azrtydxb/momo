# momo-03 Task 9b: TestPostgresSQLDBAdapter runs the PostgreSQL suite through *sql.DB (S-88)

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: -

## Description

Split from plan momo-03-drivers Task 9 (`.procoder/plans/momo-03-drivers.md:990`), Steps 2b, 2c and 5b. A team that already owns a `*sql.DB` needs proof that Momo over `pgxdriver.FromSQLDB` behaves like the native pgx adapter: the shared PostgreSQL suite passes through it, and every capability difference is listed with its reason. Depends on momo-05 Task 35 (`suite.RunAll`), so this story is scheduled after it.

## Acceptance criteria

- [x] `TestPostgresSQLDBAdapter` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-88] `TestPostgresSQLDBAdapter` runs the PostgreSQL suite through *sql.DB; fails if any capability differs without being listed in the matrix.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

CI run 42457640580 on b7ec703 (ci-agent: unit, lint, fuzz-smoke, integration postgres) concluded success; the named tests passed there.

CI run 42457640580 on b7ec703 (ci-agent: unit, lint, fuzz-smoke, integration postgres) concluded success; the named tests passed there.

Red evidence: ci-agent run 42453837637 on throwaway branch worktree-agent-ac8514dccee058855-red (commit a900207, the code under test broken, tests kept, branch since deleted) failed: --- FAIL: TestPostgresSQLDBAdapter, with FromSQLDB reporting Vectors false so a capability differed unlisted.
