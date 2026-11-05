# momo-02 Task 7: Model discovery from the momo.Model marker

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 7: Model discovery from the momo.Model marker (`.procoder/plans/momo-02-generator.md:566`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-2, S-3. Files: `gen/model.go`, `gen/model_test.go`, `gen/testdata/fixtures/models/models.go`, `gen/load.go`, `gen/types.go`. Contributes to S-2, S-3; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestModelDiscovery` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

