# momo-01 Task 16: Cursor closing and empty-result semantics

Status: open
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 16: Cursor closing and empty-result semantics (`.procoder/plans/momo-01-foundation.md:1365`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4, S-8. Files: `cursor_test.go`.

## Acceptance criteria

- [ ] `TestIterClosesCursor` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestEmptyResultNotNotFound` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-4, part] `TestIterClosesCursor` passes for the part of S-4 this task delivers; the full criterion is verified in story 067.
- [ ] [S-8, part] `TestEmptyResultNotNotFound` passes for the part of S-8 this task delivers; the full criterion is verified in story 200.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

