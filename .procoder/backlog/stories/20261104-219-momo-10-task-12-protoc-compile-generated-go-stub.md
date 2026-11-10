# momo-10 Task 12: protoc compile, generated Go stubs and an in-process round trip

Status: open
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-10-ecosystem-generators Task 12: protoc compile, generated Go stubs and an in-process round trip (`.procoder/plans/momo-10-ecosystem-generators.md:1010`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-64. Files: `gen/grpc/server.go`, `gen/grpc/plugin.go`, `internal/integration/grpc/grpc_test.go`, `internal/integration/grpc/testdata/rename/`.

## Acceptance criteria

- [ ] `TestProtoGenerator` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-64] `TestProtoGenerator` asserts generated .proto compiles with protoc, a CRUD round trip over an in-process gRPC server succeeds, and renaming a field keeps its number; fails if field numbers shift.
- [x] protoc verifies the JSON-name collision rule: `TestProtoJSONNameCollision` gains a `foo_bar` vs `foobar` (and `Foo` vs `foo`) case. If protoc compares JSON names case-insensitively in proto3, `fieldsOf` (gen/grpc/proto.go:~153) keys its check on the lower-cased name. Found in the 2026-11-08 review of the emitter follow-ups.
- [x] On a JSON-name collision, the column the lock already numbers keeps its field and the newcomer is the one skipped, so adding a column can never silently drop an existing field from the wire. A test with a lock holding `fooBar` and a new `foo_bar` placed before it covers this.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence
- Built: gen/grpc/server.go (ServerSource), plugin.go (Plugin writes momo.proto and momo.proto.lock via w.Text, server_gen.go via w.File; the lock file carries the generated header, which LoadLock skips), proto.go (go_package option, JSON-name rule), internal/integration/grpc/grpc_test.go, testdata/rename, committed example and protoc stubs.
- [S-64] TestProtoGenerator: committed .proto, lock, server and stubs equal a fresh generation; protoc v36.2 compiles; CRUD, filter, keyset page and NotFound over bufconn on PostgreSQL; the rename fixture keeps `title = 3`. CI run 42779257792 (headSha b6b30a8): unit, integration (postgres), integration-mysql (mysql84, mysql97, mariadb), fuzz-smoke, bench-module and apidiff-docs green; lint red only on gosec of the protoc stubs (fixed in the next commit). TestGraphQLGenerator, TestProtoGenerator and TestOpenAPIGenerator PASS in run 42774579231 (regen-tmp, commit 41a3405, PostgreSQL with MOMO_REQUIRE_INTEGRATION=1 and the pinned protoc).
- JSON-name rule, checked against real protoc 36.2 (checkJSONNameRule in TestProtoGenerator): protoc accepts `foo_bar` with `foobar` and `Foo` with `foo` and rejects `foo_bar` with `fooBar`. So fieldsOf compares JSON names exactly, not lower-cased; the criterion said to key on the lower-cased name only "if protoc compares case-insensitively", and it does not. TestProtoJSONNameCollision (gen/grpc/lock_compat_test.go) covers both accepted pairs.
- Lock wins: TestProtoJSONNameCollisionKeepsLockedColumn (lock holds `fooBar`, new `foo_bar` placed before it): the locked column keeps field 2, the newcomer is skipped with a diagnostic, nothing is numbered or reserved.
- Open: criterion 1 not ticked. Red proof: the tests were written with the code, so no run failed on `undefined` alone; per the 2026-11-08 decision a throwaway red branch (implementation reverted) is still needed to tick this criterion.
