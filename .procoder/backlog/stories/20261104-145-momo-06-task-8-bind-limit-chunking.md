# momo-06 Task 8: Bind-limit chunking

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: -

## Description

Implements plan momo-06-relations Task 8: Bind-limit chunking (`.procoder/plans/momo-06-relations.md:2342`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-5, S-6. Files: `options_maxparams.go`, `load_test.go`. Contributes to S-5; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestEagerLoadChunking` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-6] `TestEagerLoad*` (one per relation kind incl. composite keys): a counting hook asserts the exact query count, chunking over the bind limit, parent pagination and order preserved without duplicates, NULL-FK parents left empty; fails if loading issues per-parent queries or joins that duplicate parents.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

