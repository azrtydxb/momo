# momo-10 Task 12: protoc compile, generated Go stubs and an in-process round trip

Status: open
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: -

## Description

Implements plan momo-10-ecosystem-generators Task 12: protoc compile, generated Go stubs and an in-process round trip (`.procoder/plans/momo-10-ecosystem-generators.md:1010`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-64. Files: `gen/grpc/server.go`, `gen/grpc/plugin.go`, `internal/integration/grpc/grpc_test.go`, `internal/integration/grpc/testdata/rename/`.

## Acceptance criteria

- [ ] `TestProtoGenerator` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-64] `TestProtoGenerator` asserts generated .proto compiles with protoc, a CRUD round trip over an in-process gRPC server succeeds, and renaming a field keeps its number; fails if field numbers shift.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

