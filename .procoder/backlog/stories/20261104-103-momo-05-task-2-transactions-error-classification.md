# momo-05 Task 2: Transactions, error classification and the Scalars helper (S-7, S-8)

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 2: Transactions, error classification and the Scalars helper (S-7, S-8) (`.procoder/plans/momo-05-writes-queries.md:160`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-7, S-8. Files: `internal/suite/transactions.go`, `internal/suite/errors.go`, `scalars.go`, `transactions_test.go`.

## Acceptance criteria

- [x] `TestTransactions` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestErrorClassification` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestScalarsRawPlaceholders` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-7, part] `TestTransactions` passes for the part of S-7 this task delivers; the full criterion is verified in story 202.
- [x] [S-8, part] `TestErrorClassification` passes for the part of S-8 this task delivers; the full criterion is verified in story 200.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Failing first: `go test -tags integration -run 'TestTransactions|TestErrorClassification|TestScalars' .` failed to build: `undefined: suite.Errors`, `undefined: momo.Scalars`, `undefined: suite.Transactions`.
- Passing: same command with -race on PostgreSQL: TestTransactions (commit, rollback, panic, savepoint, isolation), TestErrorClassification (unique, not null, check, foreign key) and TestScalarsRawPlaceholders all PASS. `go build`, `go vet`, `go test -race ./...` and `make staticcheck` are clean.
- Mutation: removing the re-raise in `DB.Tx` made `TestTransactions/postgres/panic_rolls_back_and_re-raises` fail with `panic was swallowed`; restored.
- Gate: `procoder check` reported 0 blocking findings. Commit 4cfbaca.
- Note: TestErrorClassification was added to the existing errors_test.go. SQLite and MySQL runs happen through testopen.Each once those openers are registered; full S-7 and S-8 verification is in stories 202 and 200.

