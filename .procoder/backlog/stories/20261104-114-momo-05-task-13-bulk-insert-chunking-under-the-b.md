# momo-05 Task 13: Bulk insert chunking under the bind-parameter limit — S-5

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 13: Bulk insert chunking under the bind-parameter limit — S-5 (`.procoder/plans/momo-05-writes-queries.md:1025`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-5. Files: `bulk_test.go`, `insert.go`.

## Acceptance criteria

- [x] `TestBulkInsertChunking` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-5, part] `TestBulkInsertChunking` passes for the part of S-5 this task delivers; the full criterion is verified in story 115.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red first: with the unchanged insert.go, the new unit tests failed with `limit exceeded: 55 bind parameters exceed the postgres limit of 50` (limit plus one, two chunks, shared timestamp, upsert identity, RETURNING and early-close tests).
- Green: `go test -tags integration -race -count=1 .` against the running PostgreSQL passes, including `TestBulkInsertChunking` (40000 rows, 200000 parameters, chunked), RETURNING order across chunks, upsert with identity (OVERRIDING SYSTEM VALUE per chunk), rollback of earlier chunks on a failing chunk, and chunking inside a caller Tx.
- Unit tests in `bulk_unit_test.go` cover exactly-at-limit, limit plus one, a single row over the limit (ErrLimitExceeded, no statements run), one shared clock read, one transaction per chunked Exec, and a savepoint on a caller Tx.
- SQLite is not registered in this build, so the SQLite half of `TestBulkInsertChunking` did not run here; the limit comes from the dialect (32766).
- Decisions: all chunks share one clock read (story 121 semantics); a pinned OnConn connection cannot begin a transaction so chunks there are not atomic (documented on Exec).
