# momo-01 Task 12: Raw SQL placeholders

Status: open
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 12: Raw SQL placeholders (`.procoder/plans/momo-01-foundation.md:1106`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4, S-77. Files: `raw_test.go`.

## Acceptance criteria

- [ ] `TestRawPlaceholders` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-4] `TestIterClosesCursor`: Each/Iter close the cursor on early break, callback error and context cancellation, asserted via pool stats (no acquired connections afterwards); fails if a return path skips Close.
- [ ] [S-77] `TestRawArgs` and `FuzzRawArgs` assert named and expanded placeholders bind correctly per dialect and identifiers are quoted; fails if an identifier argument is bound as a value or a value is interpolated.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

