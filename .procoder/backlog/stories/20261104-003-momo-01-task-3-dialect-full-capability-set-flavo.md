# momo-01 Task 3: Dialect: full capability set, flavours and quoting

Status: open
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 3: Dialect: full capability set, flavours and quoting (`.procoder/plans/momo-01-foundation.md:285`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4, S-31. Files: `dialect/dialect.go`, `dialect/dialect_test.go`.

## Acceptance criteria

- [ ] `TestDialectCapabilities` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestAppendIdentQuoting` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestPlaceholderStyle` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-4] `TestIterClosesCursor`: Each/Iter close the cursor on early break, callback error and context cancellation, asserted via pool stats (no acquired connections afterwards); fails if a return path skips Close.
- [ ] [S-31] `TestTypeMapping` round-trips one value (including edge values: max int64, NaN where legal, pre-1970 time, non-UTC zone) per mapping in the documented table on both dialects; fails if a documented mapping is untested.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

