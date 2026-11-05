# momo-01 Task 8: Error sentinels, structured Error and sibling sentinels

Status: done 2026-11-04
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 8: Error sentinels, structured Error and sibling sentinels (`.procoder/plans/momo-01-foundation.md:758`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-8. Files: `errors.go`, `errors_test.go`, `vector/errors.go`, `migrate/errors.go`. Contributes to S-8; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestErrorSentinels` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestErrorIsAs` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestInvalidVectorSentinel` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestMigrateSentinelMessages` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `TestErrorSentinels`: `go test . -run 'Sentinel|IsAs' -count=1` before implementing: `FAIL github.com/azrtydxb/momo [build failed]` (undefined ErrValidation, vector has no non-test files); after: `ok  github.com/azrtydxb/momo`.
- `TestErrorIsAs`: same failing run before; `go test ./... -run 'Sentinel|IsAs|InvalidVector' -count=1` after: `ok  github.com/azrtydxb/momo`.
- `TestInvalidVectorSentinel`: before: `vector/errors_test.go:8:5: undefined: ErrInvalidVector`; after: `ok  github.com/azrtydxb/momo/vector`.
- `TestMigrateSentinelMessages`: before: `migrate/errors_test.go:11:30: undefined: ErrChecksumMismatch`; after: `ok  github.com/azrtydxb/momo/migrate`.
- Gate: `go build ./... && go vet ./... && go test -race ./... -count=1` all `ok`; `launcher.sh check`: `procoder gate: 6 clean, 0 unformatted, 0 unchecked, 0 out of scope, 33 hygiene finding(s) (0 blocking)`; commit made.

