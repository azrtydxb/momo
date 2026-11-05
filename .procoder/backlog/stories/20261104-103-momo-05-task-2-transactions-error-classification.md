# momo-05 Task 2: Transactions, error classification and the Scalars helper (S-7, S-8)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 2: Transactions, error classification and the Scalars helper (S-7, S-8) (`.procoder/plans/momo-05-writes-queries.md:160`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-7, S-8. Files: `internal/suite/transactions.go`, `internal/suite/errors.go`, `scalars.go`, `transactions_test.go`.

## Acceptance criteria

- [ ] `TestTransactions` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestErrorClassification` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestScalarsRawPlaceholders` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-7, part] `TestTransactions` passes for the part of S-7 this task delivers; the full criterion is verified in story 202.
- [ ] [S-8, part] `TestErrorClassification` passes for the part of S-8 this task delivers; the full criterion is verified in story 200.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

