# momo-05 Task 10: Raw SQL arguments: named, expanded IN and identifiers (S-77)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 10: Raw SQL arguments: named, expanded IN and identifiers (S-77) (`.procoder/plans/momo-05-writes-queries.md:759`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-77. Files: `raw_args.go`, `raw_args_test.go`.

## Acceptance criteria

- [ ] `TestRawArgs` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestRawEmptyInIsBuildError` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestRawNonEmptyInBinds` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-77] `TestRawArgs` and `FuzzRawArgs` assert named and expanded placeholders bind correctly per dialect and identifiers are quoted; fails if an identifier argument is bound as a value or a value is interpolated.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

