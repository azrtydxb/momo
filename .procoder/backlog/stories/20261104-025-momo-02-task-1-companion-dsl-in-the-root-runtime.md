# momo-02 Task 1: Companion DSL in the root runtime and the golden helper

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 1: Companion DSL in the root runtime and the golden helper (`.procoder/plans/momo-02-generator.md:45`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-2. Files: `momo/companion.go`, `momo/companion_test.go`, `internal/golden/golden.go`, `internal/golden/golden_test.go`. Contributes to S-2; those criteria are verified in the stories that define their named tests. Depends on momo-04 Task 1 (story 070): the seven managed-object companion methods return schema types that task adds.

## Acceptance criteria

- [ ] `TestCompanionDSLCompiles` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestGoldenDiff` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

