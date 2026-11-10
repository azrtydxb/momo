# momo-11 Task 13: Go-function migrations

Status: open
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-11-adoption Task 13: Go-function migrations (`.procoder/plans/momo-11-adoption.md:1458`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-94. Files: `migrate/gomigration.go`, `migrate/runner.go`.

## Acceptance criteria

- [ ] `TestGoMigrations` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-94] `TestGoMigrations` asserts a registered Go migration runs once under the lock with history recorded, the split directive splits statements, and group rollback runs authored downs in reverse; fails if a Go migration bypasses history.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

Carry reason: superseded by a different shipped design. Plan task 13 specified `GoMigration{Up, Down func(ctx, momo.Querier) error, NoTx}` with Go downs dispatched by group rollback. What shipped (sprint 3 and later) is forward-only: `migrate/gomigration.go:47` `GoMigration{Version, Name, Up func(ctx, *momo.Tx), UpNoTx func(ctx, *momo.DB), Checksum}`, registered with `WithGoMigrations` (`migrate/gomigration.go:88`), kind "go" in history, run once under the migration lock (`migrate/gomigration_integration_test.go:45` `TestGoMigrationOrderedWithSQLRunsOnce`), failure/dirty/retry paths (lines 100, 141, 197, 262, 280). A rollback that reaches a Go migration is refused with `ErrGoMigrationNotReversible` (`migrate/errors.go:29`, `migrate/gomigration_diff_test.go:28` `TestRefuseGoRollback`), so "group rollback runs authored downs in reverse" for Go migrations contradicts the shipped contract. SQL group rollback in reverse order is covered by `migrate/rollback_test.go:19` `TestRollbackGroup`. No test named `TestGoMigrations` exists and none was invented; criteria are left unticked for the integrator to carry or rewrite (whether Go migrations should gain a Down is a product decision, not made here).

