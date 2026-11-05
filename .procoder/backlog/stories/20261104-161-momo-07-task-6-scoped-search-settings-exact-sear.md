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

### Review fixes

- ExactSearch plan proof: `TestExactSearchBypassesVectorIndex` runs EXPLAIN of a vector `ORDER BY ... LIMIT` on a 20000-row hnsw-indexed table. Control (`enable_seqscan` off via `SetLocal`) shows the hnsw index in the plan; under `ExactSearch` the plan has a Seq Scan and no index. The `ExactSearch` field doc now says it disables all index scans for the rest of the transaction, primary-key and join lookups included.
- pgvector version gate: `SetSearch` reads `extversion` from `pg_extension` when an iterative-scan, max_scan_tuples or max_probes field is set and returns `ErrUnsupported` below 0.8.0 (this is reliable where checking `current_setting` is not, because an older server accepts any dotted name as a placeholder). `TestPgvectorAtLeast08` covers the comparison, `TestSearchIterativeNeedsPgvector08` proves no `set_config` on 0.7.x, and `TestSearchIterativeSettingsOnInstalledPgvector` passes on the pinned 0.8.7.
- `SetLocal` doc states the name must never come from untrusted input (role, session_authorization, search_path).
- Upper bounds: ivfflat.probes and ivfflat.max_probes at most 32768, hnsw.max_scan_tuples at most 2147483647, ef_search 1..1000, all `ErrValidation`, unit tested at the boundary and one past it.
- PgBouncer: `TestSearchSettingsPgBouncer` (skips without `MOMO_TEST_PGBOUNCER_URL`) passed against PgBouncer on port 6432. Evidence is honest rather than absolute: inside the transaction the setting is visible (one backend is pinned for the transaction), and 20 later statements through a 4-connection pool never saw it. Transaction pooling does not let the test pin the same backend afterwards, so absence after commit is supporting evidence only.
- MySQL vs SQLite: documented on `SetSearch`; SQLite treats `ExactSearch` alone as a no-op (always exact), MySQL returns `ErrUnsupported` (`TestSearchMySQLExactSearchUnsupported`).
- `Tx.SetLocal` is also specified by momo-04 Task 23; that plan now notes it already exists in `search.go` so the implementer reuses it. `hnsw.scan_mem_multiplier` is out of plan and tracked in the follow-up story `20261105-momo-07-support-hnsw-scan-mem-multiplier-search-setting`.
- Second fresh review: 0 Critical/0 Important; its 3 Minor items (error hint for ALTER EXTENSION vector UPDATE, softened placeholder wording, plan bound comments) applied on merge.
