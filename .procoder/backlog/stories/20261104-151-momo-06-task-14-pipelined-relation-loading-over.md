# momo-06 Task 14: Pipelined relation loading over pgx batching

Status: done 2026-11-06
Created: 2026-11-04
Epic: momo-06-relations
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-06-relations Task 14: Pipelined relation loading over pgx batching (`.procoder/plans/momo-06-relations.md:2923`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-51. Files: `pending`, `exec.go`, `internal/relfixture/count.go`, `pipeline_test.go`.

## Acceptance criteria

- [x] `TestPipelinedEagerLoad` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-51] `TestPipelinedEagerLoad` asserts two relations load in one round trip (driver round-trip counter) with identical results to sequential loading; fails if results differ.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Targeted run (integration tag, PG + PgBouncer): `go test -tags integration -count=1 -run 'Pipelin|Nested|TopN' .` passed (TestPipelined* x8 incl. Tx, hooks, errors, cancel, fallback, PgBouncer; Nested*; TopN*). `go vet` clean for both tag sets, `gofmt -l` empty.
- Full suite, lint gate (`procoder check`) and mutation sweeps are not run locally; CI runs them. The gate criterion stays unticked until CI is green.
- Review fixes (2026-11-06): consume-phase failure now ends every relation and statement once with the error (`TestPipelinedConsumeFailure`; mutation skipping `abortRows` fails it); pipelined load on a finished pgx transaction returned nil before the fix and now returns `ErrTxDone` (`TestPipelinedOnFinishedTx`); the reversed batch result order mutation is caught by the pipelined tests. Event order and memory behaviour are documented in `pipeline.go` and `LoadAll`.

- 2026-11-06 gate: commits 0a89d3c and 9a94027; CI run 42202839194 (18 jobs: lint, race, unit, integration pg16/pg17, windows, macos, govulncheck) concluded success; fresh re-review approved (mutation: dropping the tx-done flag on Rollback killed by TestPipelinedOnFinishedTx); merged into sprint-2.
