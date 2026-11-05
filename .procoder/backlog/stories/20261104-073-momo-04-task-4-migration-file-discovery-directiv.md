# momo-04 Task 4: Migration file discovery, directives, statement splitter

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 4: Migration file discovery, directives, statement splitter (`.procoder/plans/momo-04-migrations.md:578`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-11, S-94. Files: `migrate/files.go`. Contributes to S-11, S-94; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestSplitStatements` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSplitDollarQuotes` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSplitSQLiteTrigger` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestDirectives` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestDiscoverRejectsMalformedNames` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

