# momo-02 Task 15: Generated ColumnByName allowlist (S-43)

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 15: Generated ColumnByName allowlist (S-43) (`.procoder/plans/momo-02-generator.md:1561`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-43. Files: `gen/emit_lookup.go`, `gen/emit_lookup_test.go`, `gen/testdata/fixtures/sensitive/s.go`, `gen/testdata/golden/descriptors/document/lookup_test.go`.

## Acceptance criteria

- [x] `TestColumnByName` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestColumnByNameRuntime` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-43, part] `TestColumnByName` passes for the part of S-43 this task delivers; the full criterion is verified in story 104.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: `go test ./gen -run TestColumnByName` failed with `undefined: EmitLookup`; the document golden package failed with `undefined: ColumnByName`.
- Green: `go test ./gen` and `go test ./gen/testdata/golden/descriptors/document` pass; `go test -race -count=1 ./...`, `go vet`, `make staticcheck` clean.
- Hostile strings (case variants, whitespace, quotes, dots, SQL fragments, empty) are all rejected in `TestColumnByNameHostile`.
- Fuzz: `FuzzColumnByName` ran 15s, 566976 execs, no failure; registered in the Makefile.
- Mutation: making the generated switch case-insensitive failed `TestColumnByNameHostile` and the fuzz seeds.
- Gate: `procoder check` reports 0 blocking findings; commit feat(gen): generated allowlisted ColumnByName lookup.
