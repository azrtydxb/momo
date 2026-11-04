# momo-06 Task 8: Bind-limit chunking

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: -

## Description

Implements plan momo-06-relations Task 8: Bind-limit chunking (`.procoder/plans/momo-06-relations.md:2342`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-5, S-6. Files: `options_maxparams.go`, `load_test.go`. Contributes to S-5; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestEagerLoadChunking` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-6, part] this story's tests pass for the part of S-6 this task delivers; the full criterion is verified in story 146.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

