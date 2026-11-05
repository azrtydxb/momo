# momo-04 Task 14: PostgreSQL migration lock and session timeouts

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 14: PostgreSQL migration lock and session timeouts (`.procoder/plans/momo-04-migrations.md:1620`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-11, S-28. Files: `migrate/postgres/lock.go`. Contributes to S-11; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestLockHolderNamed` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestLockKeyPerSchema` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestLockTimeoutApplied` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-28, part] `TestLockTimeoutApplied` passes for the part of S-28 this task delivers; the full criterion is verified in story 095.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Failing first: `go test -tags integration -race ./migrate/postgres/` failed to build with `mpg.New().Lock undefined` and `mpg.New().Setup undefined`.
- Passing now: `MOMO_REQUIRE_INTEGRATION=1 ... go test -tags integration -race -count=1 ./migrate/...` gives `ok migrate` and `ok migrate/postgres`.
- Extra live-DB tests: lock released after panic, unlock after context cancel, second locker refused with holder named then succeeds after unlock, and timeouts reset (no leak) on the pooled connection.
- Mutation check: replacing pg_advisory_unlock with a no-op made TestLockReleasedAfterPanic and TestUnlockAfterContextCancel fail with the lock still held; restored afterwards.
- Gate: `procoder check` 0 blocking findings, `make staticcheck` clean, `go build ./...` and `go vet ./...` pass.
- Commit: f2a8312 feat(migrate/postgres): advisory migration lock with named holder and session timeouts.

