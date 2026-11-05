# momo-02 Task 16: Identifier collisions and reserved words

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 16: Identifier collisions and reserved words (`.procoder/plans/momo-02-generator.md:1690`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-3. Files: `gen/ident.go`, `gen/ident_test.go`, `gen/testdata/fixtures/collide/collide.go`, `gen/testdata/fixtures/collide/sub/sub.go`.

## Acceptance criteria

- [x] `TestGenerateNameCollisions` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-3, part] `TestGenerateNameCollisions` passes for the part of S-3 this task delivers; the full criterion is verified in story 043.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Commit 42f7b26 feat(gen): identifier collisions and reserved words; go build, vet and test -race ./... pass; procoder check: 0 blocking.
- Failing first: go test ./gen reported undefined: Identifier before gen/ident.go existed.
- Passing: TestIdentifierReservedWords, TestIdentifierEdgeCases, TestCheckModelIdentifiers, TestCheckColumnCollisions.
- TestGenerateNameCollisions: unit part passes; the Load and column-collision part SKIPs until the model builder (task 7) lands and calls CheckColumnCollisions and CheckModelIdentifiers. The first two criteria stay unchecked.

- Story 043 (commit 515a3ad): TestGenerateNameCollisions passes with no skip, asserts len(g.Models) > 0 and the user_id column collision; checks wired in gen/load.go. Full suite passes with -race. The first criterion was left unticked here because of the earlier skip; its test now runs fully and passes.
