# momo-03 Task 2: Optional driver capability interfaces in the root package

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-03-drivers Task 2: Optional driver capability interfaces in the root package (`.procoder/plans/momo-03-drivers.md:164`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `driver_optional.go`, `driver_optional_test.go`.

## Acceptance criteria

- [x] `TestOptionalInterfacesAreSatisfiedByAdapters` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- RED: `go test -run TestOptionalInterfacesAreSatisfiedByAdapters .` failed to build: `undefined: PoolStats`, `Capabilities`, `PinnedConn`, `Pinner`, `Copier`, `Batcher`, `PoolStater`, `ServerVersioner`, `CapabilityReporter`.
- GREEN: `go test -race -count=1 -run TestOptionalInterfacesAreSatisfiedByAdapters .` -> `ok`.
- Mutation: changing `Pinner.Pin` to take an extra argument broke the build (`optionalFull does not implement Pinner (wrong type for method Pin)`); restored.
- Files: `driver_optional.go`, `driver_optional_test.go` (compile-time `var _ momo.X` checks for all seven interfaces plus the plan's literal test).
