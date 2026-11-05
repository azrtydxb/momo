# momo-04 Task 2: Schema normalization and FuzzNormalize

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 2: Schema normalization and FuzzNormalize (`.procoder/plans/momo-04-migrations.md:233`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-1, S-11. Files: `schema/normalize.go`. Contributes to S-1, S-11; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestNormalizeTypes` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestNormalizeExprDBRewrites` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestEqualIgnoresOrder` exists as written in the plan, failed before the implementation, and passes now
- [ ] `FuzzNormalize` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

