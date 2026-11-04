# momo-02 Task 6: Package loading with positioned diagnostics

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 6: Package loading with positioned diagnostics (`.procoder/plans/momo-02-generator.md:491`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-2, S-3. Files: `gen/types.go`, `gen/load.go`, `gen/load_test.go`, `gen/testdata/fixtures/brokenpkg/broken.go`.

## Acceptance criteria

- [ ] `TestLoadPackagesDiagnostics` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-2] [S-3] `TestGenerateCompanion` (generator golden test): a model package with tags and a MomoConfig companion generates code that `go build` accepts; `TestGenerateDiagnostics` asserts an unsupported companion expression yields file:line:col and writes no files; fails if the generator guesses or emits partial output.
- [ ] [S-3] `TestGenerateCrossPackage`, `TestGenerateRefusesHandwritten` and `TestGenerateNameCollisions` (golden tests): models outside package main build without cycles, a handwritten file at a target path is never overwritten, reserved words and colliding names get distinct identifiers; fails if any golden fixture stops compiling.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

