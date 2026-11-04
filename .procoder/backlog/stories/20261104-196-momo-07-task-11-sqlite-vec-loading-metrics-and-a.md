# momo-07 Task 11: sqlite-vec loading, metrics and ANN rejection

Status: open
Created: 2026-11-04
Epic: momo-07-vectors-sqlite-vec
Sprint: -

## Description

Implements plan momo-07-vectors Task 11: sqlite-vec loading, metrics and ANN rejection (`.procoder/plans/momo-07-vectors.md:1819`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-16. Files: `driver/sqlitedriver/vec.go`, `driver/sqlitedriver/vec_test.go`, `driver/sqlitedriver/parity_integration_test.go`.

## Acceptance criteria

- [ ] `TestSQLiteVecLoaded` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSQLiteVecMetrics` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSQLiteANNIndexUnsupported` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSQLiteVectorParity` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-16] `TestSQLiteVectorParity`: sqlite-vec distance queries return the same top-k ids as pgvector exact search on the same data; requesting an ANN index on SQLite returns ErrUnsupported; fails if SQLite silently accepts an index option.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

