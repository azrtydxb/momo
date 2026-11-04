# momo-03 Task 9b: TestPostgresSQLDBAdapter runs the PostgreSQL suite through *sql.DB (S-88)

Status: open
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: -

## Description

Split from plan momo-03-drivers Task 9 (`.procoder/plans/momo-03-drivers.md:990`), Steps 2b, 2c and 5b. A team that already owns a `*sql.DB` needs proof that Momo over `pgxdriver.FromSQLDB` behaves like the native pgx adapter: the shared PostgreSQL suite passes through it, and every capability difference is listed with its reason. Depends on momo-05 Task 35 (`suite.RunAll`), so this story is scheduled after it.

## Acceptance criteria

- [ ] `TestPostgresSQLDBAdapter` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-88] `TestPostgresSQLDBAdapter` runs the PostgreSQL suite through *sql.DB; fails if any capability differs without being listed in the matrix.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

