# momo-04 Task 3: Migrate core types, errors, options, Dialect interface, test helpers

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 3: Migrate core types, errors, options, Dialect interface, test helpers (`.procoder/plans/momo-04-migrations.md:359`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-8, S-10. Files: `migrate/types.go`.

## Acceptance criteria

- [ ] `TestSentinelsAreDistinct` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-8] `TestErrorClassification` (both dialects): each error kind matches errors.Is and the driver error is reachable with errors.As (*pgconn.PgError, sqlite3.Error); `TestEmptyResultNotNotFound` asserts All on zero rows returns nil error; fails if classification loses the cause.
- [ ] [S-10] `TestCLIContracts`: each command's exit codes and outputs as documented in Interfaces; offline commands run with no database URL set; fails if an offline command opens a connection.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

