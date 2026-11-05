# momo-02 Task 17: The Writer: collision-safe, deterministic, refuses handwritten files

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-02-generator Task 17: The Writer: collision-safe, deterministic, refuses handwritten files (`.procoder/plans/momo-02-generator.md:1782`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-3. Files: `gen/writer.go`, `gen/writer_test.go`.

## Acceptance criteria

- [x] `TestGenerateRefusesHandwritten` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-3, part] `TestGenerateRefusesHandwritten` passes for the part of S-3 this task delivers; the full criterion is verified in story 043.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

Failing run before implementation: `go test ./gen -run Writer -count=1` gave `undefined: NewWriter`. After: `go test -race -count=1 ./...` all ok, including TestGenerateRefusesHandwritten, TestWriterIdempotentAndCheck, TestWriterFailedWriteLeavesNothing, TestWriterOrphans, TestWriterCollisionSameBytesOK. Gate clean; commit 9b263e1 feat(gen): collision-safe deterministic writer. The S-3 part criterion stays unchecked: verified in story 043.

