# momo-06 Task 1: Normalized key encoding

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-06-relations Task 1: Normalized key encoding (`.procoder/plans/momo-06-relations.md:150`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-6. Files: `keys.go`, `keys_internal_test.go`.

## Acceptance criteria

- [x] `TestKeyStringNormalization` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-6, part] this story's tests pass for the part of S-6 this task delivers; the full criterion is verified in story 146.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red first: before keys.go existed, `go test ./ -run '^TestKeyStringNormalization$'` failed with `undefined: keyString` (build failed). After keys.go it prints `ok  github.com/azrtydxb/momo`.
- Mutation check: removing the 0x1f delimiter initially survived (the type tags alone already separate the plan's composite case), so one case was added to the test, `diff({"a","b"}, {"as:b"})`, and the mutant is now killed (`keys collide: "s:as:b"`). Changing the int64 tag from i: to j: also fails the test. Both mutants restored; test passes.
- Deviation: the added delimiter case is the only change to the test as written in the plan.
