# momo-04 Task 3: Migrate core types, errors, options, Dialect interface, test helpers

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 3: Migrate core types, errors, options, Dialect interface, test helpers (`.procoder/plans/momo-04-migrations.md:359`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-8, S-10. Files: `migrate/types.go`. Contributes to S-8, S-10; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestSentinelsAreDistinct` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

