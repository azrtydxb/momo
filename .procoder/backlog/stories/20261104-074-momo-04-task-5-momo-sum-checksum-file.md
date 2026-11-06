# momo-04 Task 5: momo.sum checksum file

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 5: momo.sum checksum file (`.procoder/plans/momo-04-migrations.md:707`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-11, S-22. Files: `migrate/sum.go`. Contributes to S-11, S-22; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestSumFormatVersion` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestChecksumMismatch` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- RED: `go test ./migrate/` failed to build with `undefined: ComputeSum`, `undefined: ParseSum`, `undefined: ErrNewerFormat`.
- GREEN: `go test -race -count=1 ./migrate/` ok. TestSumFormatVersion and TestChecksumMismatch pass, plus tests for deterministic line order over 20 runs, round trip, added/removed/edited detection, tampered entry or directory hash (ErrChecksumMismatch), malformed lines, CRLF and BOM, and append-only AppendSum.
- Mutation check: made VerifySum ignore hash differences; TestChecksumMismatch, TestVerifySumReportsAddedAndRemoved and TestAppendSum failed. Restored.
- Gate: `procoder check` 0 blocking findings; go build, go vet and go test -race pass.
- Deviation: the plan uses `migrate.ErrNewerFormat` but Task 3 does not define it; sum.go defines `ErrNewerFormat = schema.ErrNewerFormat` so one sentinel matches both packages.

