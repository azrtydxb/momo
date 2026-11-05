# momo-01 Task 12: Raw SQL placeholders

Status: done 2026-11-04
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 12: Raw SQL placeholders (`.procoder/plans/momo-01-foundation.md:1106`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4, S-77. Files: `raw_test.go`. Contributes to S-4, S-77; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestRawPlaceholders` exists as written in the plan, fails under a deliberate mutation of `Builder.appendRaw` (the spike's code predates the test), and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `go test . -run '^TestRawPlaceholders$' -count=1` on the unchanged spike: `ok` (guard PASS).
- Mutation 1: the `??` literal-question-mark branch of `appendRaw` disabled: FAIL `has more placeholders than the 1 arguments given`; file restored, PASS.
- Mutation 2: the too-few-arguments guard disabled: FAIL (index out of range panic instead of a build error); file restored.
- The too-many-arguments check (`n != len(args)`) was not exercised by the plan's original test. The lead added a case (`momo.SQL("a = ?", 1, 2)` must fail to build) to raw_test.go and the plan; with the check disabled → `--- FAIL: TestRawPlaceholders` / `more arguments than placeholders must fail to build`; restored → `ok`.
- `go build ./... && go vet ./... && go test -race ./...` pass.

