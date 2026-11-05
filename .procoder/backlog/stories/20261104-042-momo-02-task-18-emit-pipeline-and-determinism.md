# momo-02 Task 18: Emit pipeline and determinism

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 18: Emit pipeline and determinism (`.procoder/plans/momo-02-generator.md:1857`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-3. Files: `gen/emit.go`, `gen/emit_test.go`, `gen/writer.go`. Contributes to S-3; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestGenerateDeterministic` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

