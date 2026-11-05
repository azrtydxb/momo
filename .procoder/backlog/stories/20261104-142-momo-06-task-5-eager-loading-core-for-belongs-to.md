# momo-06 Task 5: Eager loading core for belongs-to and has-one, relation hook events

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: -

## Description

Implements plan momo-06-relations Task 5: Eager loading core for belongs-to and has-one, relation hook events (`.procoder/plans/momo-06-relations.md:1538`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-6, S-18. Files: `loader.go`, `load.go`, `relhook.go`, `db.go`, `load_test.go`. Contributes to S-18; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestEagerLoadBelongsTo` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestEagerLoadHasOne` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestRelationHookEvents` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-6, part] this story's tests pass for the part of S-6 this task delivers; the full criterion is verified in story 146.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

