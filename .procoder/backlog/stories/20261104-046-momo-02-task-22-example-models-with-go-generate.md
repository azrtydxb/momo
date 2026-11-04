# momo-02 Task 22: Example models with go:generate, and generated output that builds

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 22: Example models with go:generate, and generated output that builds (`.procoder/plans/momo-02-generator.md:2464`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-2, S-3. Files: `internal/testmodels/example/models.go`, `internal/testmodels/example/generate.go`, `internal/testmodels/example/example_test.go`, `internal/testmodels/example/momo_gen.go`.

## Acceptance criteria

- [ ] `TestGenerateCompanion` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-2] [S-3] `TestGenerateCompanion` (generator golden test): a model package with tags and a MomoConfig companion generates code that `go build` accepts; `TestGenerateDiagnostics` asserts an unsupported companion expression yields file:line:col and writes no files; fails if the generator guesses or emits partial output.
- [ ] [S-3] `TestGenerateCrossPackage`, `TestGenerateRefusesHandwritten` and `TestGenerateNameCollisions` (golden tests): models outside package main build without cycles, a handwritten file at a target path is never overwritten, reserved words and colliding names get distinct identifiers; fails if any golden fixture stops compiling.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

