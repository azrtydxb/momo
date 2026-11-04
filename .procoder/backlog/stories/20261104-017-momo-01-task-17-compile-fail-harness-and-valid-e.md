# momo-01 Task 17: Compile-fail harness and valid-example build

Status: open
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: -

## Description

Implements plan momo-01-foundation Task 17: Compile-fail harness and valid-example build (`.procoder/plans/momo-01-foundation.md:1456`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4. Files: `testdata/compilefail/wrongtype/main.go`, `testdata/compilefail/wrongtype_any/main.go`, `testdata/valid/basic/main.go`, `compilefail_test.go`.

## Acceptance criteria

- [ ] `TestCompileFail` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestValidExamplesCompile` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-4] `TestIterClosesCursor`: Each/Iter close the cursor on early break, callback error and context cancellation, asserted via pool stats (no acquired connections afterwards); fails if a return path skips Close.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

