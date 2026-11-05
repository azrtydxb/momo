# momo-04 Task 1: Schema types for snapshots and managed objects

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 1: Schema types for snapshots and managed objects (`.procoder/plans/momo-04-migrations.md:73`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-1, S-22, S-36, S-37, S-38, S-39. Files: `schema/schema.go`, `schema/types.go`, `schema/format_test.go`. Contributes to S-1, S-22, S-36, S-37, S-38, S-39; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestSchemaFormatVersion` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: `go test ./schema/ -run TestSchemaFormatVersion -count=1` FAIL (`s.Write undefined`, build failed); after the implementation PASS.
- `go test ./schema -count=1` ok. TestSchemaJSONRoundTrip, TestSchemaEveryFieldSerialized (now dereferences pointer fields; removing the json tag of RLS.Forced makes it fail), TestFormatVersions, and the new TestSortCoversManagedObjects (fails when set sorting is disabled) and TestEqualNormalizesManagedObjects pass.
- `go test ./schema -run='^$' -fuzz=FuzzSchemaSort -fuzztime=20s` PASS (1.16M execs).
- `go build ./... && go vet ./... && go test -race -count=1 ./...` ok; `procoder check` clean (0 blocking). Commit 2f36716.

