# momo-01 Task 11: Placeholders, quoting and the builder fuzz target

Status: done 2026-11-04
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 11: Placeholders, quoting and the builder fuzz target (`.procoder/plans/momo-01-foundation.md:1017`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4. Files: `builder_test.go`. Contributes to S-4; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestPlaceholdersAndQuoting` exists as written in the plan, fails under a deliberate mutation of `Dialect.AppendPlaceholder` (the spike's code predates the test), and passes now
- [x] `FuzzBuilder` exists as written in the plan, fails under a deliberate mutation of `Dialect.AppendIdent` quote doubling and of `Builder.Arg` value binding (the spike's code predates the test), and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `go test . -run '^TestPlaceholdersAndQuoting$' -count=1` on the unchanged spike: `ok` (guard PASS).
- Mutation 1: `AppendPlaceholder` never emits `$n`: FAIL `postgres numbering wrong`; file restored, PASS.
- Mutation 2: `AppendIdent` stops doubling quotes: `FuzzBuilder/seed#1` FAIL `identifier not quoted`; file restored.
- Mutation 3: `Builder.Arg` interpolates strings: `FuzzBuilder` FAIL `value interpolated`; file restored.
- `go test . -run NONE -fuzz FuzzBuilder -fuzztime 30s`: first run found a crash on name `'\x10'` with value `\x10`. Root cause was the test oracle (the quoted identifier itself spelled `'value'`), not production code; the oracle now strips the quoted identifier before the interpolation check, entry kept in `testdata/fuzz/FuzzBuilder`. Rerun 30s: PASS (5.9M execs).
- Mutation 3 still FAILs with the corrected oracle.
- `go build ./... && go vet ./... && go test -race ./...` pass.
