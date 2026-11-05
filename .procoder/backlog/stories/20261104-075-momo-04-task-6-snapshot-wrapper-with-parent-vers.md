# momo-04 Task 6: Snapshot wrapper with parent version and format_version

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 6: Snapshot wrapper with parent version and format_version (`.procoder/plans/momo-04-migrations.md:792`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-11, S-22. Files: `migrate/snapshot.go`. Contributes to S-11, S-22; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestSnapshotParentRecorded` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSnapshotFormatVersion` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- FAIL before implementation: `go test ./migrate/ -run Snapshot` gave `undefined: SnapshotFile`, `undefined: WriteSnapshot`, `undefined: ReadSnapshot`.
- PASS after: TestSnapshotParentRecorded, TestSnapshotFormatVersion, plus hostile cases (newer schema format, corrupt/truncated JSON, BOM+CRLF, byte-identical write, LatestSnapshot none/greatest/duplicate/version-mismatch/self-parent/newer-format).
- Mutation: removing BOM trimming made TestSnapshotBOMAndCRLF fail. Removing the explicit Sort survived because schema.Load already sorts (kept as explicit guarantee).
- go build, go vet, go test -race -count=1 ./... pass; procoder check 0 blocking. Commit f5d72f5.
