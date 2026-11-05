# momo-03 Task 2: Optional driver capability interfaces in the root package

Status: open
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-03-drivers Task 2: Optional driver capability interfaces in the root package (`.procoder/plans/momo-03-drivers.md:164`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `driver_optional.go`, `driver_optional_test.go`.

## Acceptance criteria

- [ ] `TestOptionalInterfacesAreSatisfiedByAdapters` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

