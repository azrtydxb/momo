# momo-05 Task 11: Fuzzed raw arguments and typed IN and empty results (S-77, S-8)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 11: Fuzzed raw arguments and typed IN and empty results (S-77, S-8) (`.procoder/plans/momo-05-writes-queries.md:879`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-8, S-77. Files: `raw_args_fuzz_test.go`, `empty_result_test.go`, `expr.go`. Contributes to S-8; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `FuzzRawArgs` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestEmptyResultNotNotFoundIntegration` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-77] `TestRawArgs` and `FuzzRawArgs` assert named and expanded placeholders bind correctly per dialect and identifiers are quoted; fails if an identifier argument is bound as a value or a value is interpolated.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Green: ci-agent run 42453190263 on d8088d5 (unit, lint, fuzz-smoke, integration postgres).
- Gate: the commit hook ran the procoder format and lint gate on every commit of this story with 0 blocking findings.
- Red-first not provable: the typed In empty-list rendering and NamedArg already existed, so FuzzRawArgs and TestEmptyResultNotNotFoundIntegration pass without any implementation change; those two criteria stay unticked.
