# momo-02 Task 9: Statically read MomoConfig companion with diagnostics

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 9: Statically read MomoConfig companion with diagnostics (`.procoder/plans/momo-02-generator.md:783`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-2. Files: `gen/companion.go`, `gen/companion_test.go`, `gen/testdata/fixtures/companion/ok.go`, `gen/testdata/fixtures/companionbad/bad.go`, `gen/model.go`.

## Acceptance criteria

- [ ] `TestCompanionReaderAccepts` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestGenerateDiagnostics` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-2, part] `TestGenerateDiagnostics` passes for the part of S-2 this task delivers; the full criterion is verified in story 046.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

