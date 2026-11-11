# momo-11 Task 13: Go-function migrations

Status: done 2026-11-11
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Carried: 007-sprint-7-finish-every-open-backlog-story-auditing-each — Plan task 13 specified Go migrations with Down functions dispatched by group rollback; the shipped design is forward-only (GoMigration has no Down, a rollback reaching one is refused with ErrGoMigrationNotReversible, TestRefuseGoRollback). No TestGoMigrations exists. Whether Go migrations should gain a Down is a product decision; not invented here.

## Description

Implements plan momo-11-adoption Task 13: Go-function migrations (`.procoder/plans/momo-11-adoption.md:1458`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-94. Files: `migrate/gomigration.go`, `migrate/runner.go`.

## Acceptance criteria

- [x] `TestRefuseGoRollback` (migrate/gomigration_diff_test.go:28) replaces `TestGoMigrations`: `RefuseGoRollback` returns `ErrGoMigrationNotReversible` naming the version of a history row of kind "go", and nothing for SQL rows. Go migrations stay forward-only, by decision.
- [x] [S-94] A registered Go migration runs once with its history recorded (a second apply and a second engine do not re-run it; the lock itself is not asserted by this test), in order with SQL migrations (`TestGoMigrationOrderedWithSQLRunsOnce`, migrate/gomigration_integration_test.go:45); the split directive splits statements (`TestSplitStatements` and the `momo:split` case of `TestSplitHostile`, migrate/split_test.go); a rollback that reaches a Go migration is refused with `ErrGoMigrationNotReversible` (`TestRefuseGoRollback`) and a SQL group rolls back in reverse order (`TestRollbackGroup`, migrate/rollback_test.go:19).
- [x] `procoder check` gate is clean and the task's commit is made (the task's code is on main at 4781864; the lint job 130341061844 of ci.yml run 42965315635 is success; the commit gate ran on the sprint 9 bookkeeping commit)

## Evidence

Carry reason: superseded by a different shipped design. Plan task 13 specified `GoMigration{Up, Down func(ctx, momo.Querier) error, NoTx}` with Go downs dispatched by group rollback. What shipped (sprint 3 and later) is forward-only: `migrate/gomigration.go:47` `GoMigration{Version, Name, Up func(ctx, *momo.Tx), UpNoTx func(ctx, *momo.DB), Checksum}`, registered with `WithGoMigrations` (`migrate/gomigration.go:88`), kind "go" in history, run once under the migration lock (`migrate/gomigration_integration_test.go:45` `TestGoMigrationOrderedWithSQLRunsOnce`), failure/dirty/retry paths (lines 100, 141, 197, 262, 280). A rollback that reaches a Go migration is refused with `ErrGoMigrationNotReversible` (`migrate/errors.go:29`, `migrate/gomigration_diff_test.go:28` `TestRefuseGoRollback`), so "group rollback runs authored downs in reverse" for Go migrations contradicts the shipped contract. SQL group rollback in reverse order is covered by `migrate/rollback_test.go:19` `TestRollbackGroup`. No test named `TestGoMigrations` exists and none was invented; criteria are left unticked for the integrator to carry or rewrite (whether Go migrations should gain a Down is a product decision, not made here).

## Reworded 2026-11-11 (user decision)

User decision of 2026-11-11, recorded in .procoder/ask/decisions.md, section "Carried plan stories": reword the criteria to the design that shipped, cite CI evidence, and close the story. The `Carried:` lines above stay as the history.

- Original: `TestGoMigrations` exists as written in the plan, failed before the implementation, and passes now
- Original: [S-94] `TestGoMigrations` asserts a registered Go migration runs once under the lock with history recorded, the split directive splits statements, and group rollback runs authored downs in reverse; fails if a Go migration bypasses history.
  - Reason: Go-function migrations stay forward-only. `GoMigration` has no `Down` (`Up func(ctx, *momo.Tx)`, `UpNoTx func(ctx, *momo.DB)`, registered with `WithGoMigrations`), and a rollback that reaches one is refused with `ErrGoMigrationNotReversible` (migrate/errors.go), so "group rollback runs authored downs in reverse" for Go migrations contradicts the shipped contract. The rollback clause now applies to SQL migrations only. No test named `TestGoMigrations` exists and none is invented.

### Sprint 9 CI evidence (2026-11-11)

Jobs of ci.yml run 42965315635 on main (headSha 47818647165f0231b1117bb6ebe2bce67dbed444, conclusion success). The job logs carry package-level `ok` lines, not `-v` output: unit (1.26) 130341061373 (`make test`), race (1.26) 130341061707 (`make test-race`), integration (1.27, 0.8.7-pg17, postgres) 130341061675 (`make test-integration-postgres`: MOMO_REQUIRE_INTEGRATION=1 and `-tags integration -race`, so a skipped database test fails), windows 130341061640, bench-module 130341061885 (`make test-bench-module`), lint 130341061844.

- Unit job 130341061373: `ok` momo/migrate (0.168s), which holds `TestRefuseGoRollback` and the split tests (no build tag).
- PostgreSQL integration job 130341061675: `ok` momo/migrate (27.600s) with MOMO_REQUIRE_INTEGRATION=1; `TestGoMigrationOrderedWithSQLRunsOnce` and `TestRollbackGroup` are `//go:build integration` and run only there.
