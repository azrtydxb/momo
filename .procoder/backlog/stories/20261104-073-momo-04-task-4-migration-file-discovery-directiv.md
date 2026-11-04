# momo-04 Task 4: Migration file discovery, directives, statement splitter

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 4: Migration file discovery, directives, statement splitter (`.procoder/plans/momo-04-migrations.md:578`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-11, S-94. Files: `migrate/files.go`.

## Acceptance criteria

- [ ] `TestSplitStatements` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSplitDollarQuotes` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSplitSQLiteTrigger` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestDirectives` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestDiscoverRejectsMalformedNames` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-11] `TestStagedChange` on a populated DB: add nullable column → data migration backfill → SET NOT NULL applies and all rows satisfy the constraint; fails if diff emits NOT NULL directly for a populated table without a default.
- [ ] [S-94] `TestGoMigrations` asserts a registered Go migration runs once under the lock with history recorded, the split directive splits statements, and group rollback runs authored downs in reverse; fails if a Go migration bypasses history.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

