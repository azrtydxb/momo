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
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

- CI run 42410267553 (ci-agent, commit d310d41) green: unit, lint, fuzz-smoke and integration (postgres) jobs pass with this story's tests.
- Criteria left unticked: the red-first claim (no pre-implementation run exists; the first CI run only failed on two fixture mistakes fixed in d310d41) and the local procoder check gate (not run on this laptop).
