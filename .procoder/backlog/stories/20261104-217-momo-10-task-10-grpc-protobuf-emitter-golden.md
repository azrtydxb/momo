# momo-10 Task 10: gRPC protobuf emitter (golden)

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: -

## Description

Implements plan momo-10-ecosystem-generators Task 10: gRPC protobuf emitter (golden) (`.procoder/plans/momo-10-ecosystem-generators.md:874`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `gen/grpc/proto.go`, `gen/grpc/proto_test.go`, `gen/grpc/testdata/example.proto`.

## Acceptance criteria

- [x] `TestProtoEmitGolden` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Files: gen/grpc/proto.go, proto_test.go, testdata/example.proto.
- TestProtoEmitGolden (and TestProtoRules) red: CI run 42455374066 (tests alone on sprint-5) failed with "undefined: Proto, Lock". Green: CI run 42456595252 (commit 5f4ffe6). The golden was written from the first run's output (run 42455728306) after review; the test also regenerates over the filled lock and requires identical bytes.
- Reserved numbers of retired columns are written as a reserved statement; sensitive columns are not in the API; an unmapped type is the diagnostic "no protobuf mapping for <type>"; an invalid package name is refused.
- Review fixes (red: CI run 42458914148): model and column names must be ASCII protobuf identifiers; Filter and derived message names (XGetRequest and the rest) that collide with another model are diagnostics and the model is left out; columns hidden by a sensitive tag or a diagnostic keep their lock numbers, and retired columns are reserved by name as well as by number (TestProtoHostileNames, TestProtoHiddenColumnsKeepNumbers).
- Not done here: go_package, the plugin and its output wiring, protoc compilation (stories 219 and 222).

