# momo-01 Task 4: Schema base types, with JSON round-trip

Status: done 2026-11-04
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 4: Schema base types, with JSON round-trip (`.procoder/plans/momo-01-foundation.md:381`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-1. Files: `schema/schema.go`, `schema/schema_test.go`.

## Acceptance criteria

- [x] `TestSchemaJSONRoundTrip` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSchemaEveryFieldSerialized` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSchemaFieldSet` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-1, JSON part] `TestSchemaJSONRoundTrip` (schema package): JSON round-trips losslessly; fails if a field is dropped from serialization. The `FuzzSchemaSort` half of S-1 is verified in story 007, which carries the full criterion.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- TestSchemaJSONRoundTrip: before implementation `go test ./schema/ -count=1` failed to build (`unknown field FormatVersion in struct literal of type Schema`, `undefined: FormatVersion`); after: `--- PASS: TestSchemaJSONRoundTrip`, `ok github.com/azrtydxb/momo/schema`
- TestSchemaEveryFieldSerialized: same red run; `go test ./schema/ -count=1 -v` prints `--- PASS: TestSchemaEveryFieldSerialized`
- TestSchemaFieldSet: same red run; `go test ./schema/ -count=1 -v` prints `--- PASS: TestSchemaFieldSet`
- [S-1, JSON part] `go test -count=1 -run 'TestSchemaJSONRoundTrip|TestSchemaEveryFieldSerialized' -v ./schema` → both `--- PASS`, `ok`. The FuzzSchemaSort half stays open in story 007.
- Gate and commit: `launcher.sh check` clean (2 clean, 0 unformatted, 0 blocking); commit 81950d4 `feat(schema): base snapshot types with format_version and JSON round-trip`; `go build ./... && go vet ./... && go test -race ./...` all ok
