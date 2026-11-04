# momo-02 Task 6: Package loading with positioned diagnostics

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 6: Package loading with positioned diagnostics (`.procoder/plans/momo-02-generator.md:491`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-2, S-3. Files: `gen/types.go`, `gen/load.go`, `gen/load_test.go`, `gen/testdata/fixtures/brokenpkg/broken.go`. Contributes to S-2, S-3; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestLoadPackagesDiagnostics` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

