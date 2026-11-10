# momo-10 Task 12: protoc compile, generated Go stubs and an in-process round trip

Status: open
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-10-ecosystem-generators Task 12: protoc compile, generated Go stubs and an in-process round trip (`.procoder/plans/momo-10-ecosystem-generators.md:1010`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-64. Files: `gen/grpc/server.go`, `gen/grpc/plugin.go`, `internal/integration/grpc/grpc_test.go`, `internal/integration/grpc/testdata/rename/`.

## Acceptance criteria

- [ ] `TestProtoGenerator` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-64] `TestProtoGenerator` asserts generated .proto compiles with protoc, a CRUD round trip over an in-process gRPC server succeeds, and renaming a field keeps its number; fails if field numbers shift.
- [ ] protoc verifies the JSON-name collision rule: `TestProtoJSONNameCollision` gains a `foo_bar` vs `foobar` (and `Foo` vs `foo`) case. If protoc compares JSON names case-insensitively in proto3, `fieldsOf` (gen/grpc/proto.go:~153) keys its check on the lower-cased name. Found in the 2026-11-08 review of the emitter follow-ups.
- [ ] On a JSON-name collision, the column the lock already numbers keeps its field and the newcomer is the one skipped, so adding a column can never silently drop an existing field from the wire. A test with a lock holding `fooBar` and a new `foo_bar` placed before it covers this.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

