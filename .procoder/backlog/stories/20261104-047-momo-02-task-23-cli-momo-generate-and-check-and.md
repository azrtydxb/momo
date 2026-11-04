# momo-02 Task 23: CLI `momo generate` and `--check`, and go generate integration

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 23: CLI `momo generate` and `--check`, and go generate integration (`.procoder/plans/momo-02-generator.md:2553`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-3. Files: `cmd/momo/main.go`, `cmd/momo/generate.go`, `cmd/momo/generate_test.go`, `cmd/momo/cmd.go`, `cmd/momo/helpers_test.go`.

## Acceptance criteria

- [ ] `TestGenerateCheck` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestGoGenerateIntegration` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-3] `TestGenerateCrossPackage`, `TestGenerateRefusesHandwritten` and `TestGenerateNameCollisions` (golden tests): models outside package main build without cycles, a handwritten file at a target path is never overwritten, reserved words and colliding names get distinct identifiers; fails if any golden fixture stops compiling.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

