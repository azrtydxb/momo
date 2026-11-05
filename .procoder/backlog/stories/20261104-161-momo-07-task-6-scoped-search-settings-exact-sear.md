# momo-07 Task 6: Scoped search settings, exact search, and iterative scans

Status: open
Created: 2026-11-04
Epic: momo-07-vectors-pgvector
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-07-vectors Task 6: Scoped search settings, exact search, and iterative scans (`.procoder/plans/momo-07-vectors.md:992`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-15, S-29. Files: `search.go`, `search_test.go`, `search_integration_test.go`.

## Acceptance criteria

- [x] `TestSearchSettingsValidation` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSearchSQL` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSearchSetLocalBinds` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSearchSettingsScoped` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestFilteredANNIterativeScan` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-15, part] `TestSearchSettingsScoped` passes for the part of S-15 this task delivers; the full criterion is verified in story 162.
- [x] [S-29] `TestFilteredANNIterativeScan`: a selective filter with HNSW returns fewer than k rows with iterative scan off and k rows with it on, settings reset after the transaction; fails if the setting leaks to the pool.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red first: `go test . -run TestSearch` failed with `undefined: SearchSettings` before `search.go` existed.
- Green: `go test . -run 'TestSearch|TestFilteredANN' -count=1` passes the unit tests `TestSearchSettingsValidation`, `TestSearchSQL`, `TestSearchSetLocalBinds` and the extra `TestSearchSQLiteSupport`.
- Integration (`-tags integration -race`, live Postgres with pgvector): `TestSearchSettingsScoped` and `TestFilteredANNIterativeScan` pass. Both run on a one-connection pool, so a leaked `SET` would be seen by the follow-up `current_setting` check; none leaked.
- Settings are applied with `set_config($1, $2, true)` (transaction-local), so they are PgBouncer transaction-mode safe.
- Deviation: `Tx.SetLocal` (named in the plan as a momo-04 contract method) did not exist on this branch, so it is added in `search.go`. The plan's `fakedriver.NewRecorder` does not exist either, so `search_fake_test.go` uses `fakedriver.New` and `Calls()`.
