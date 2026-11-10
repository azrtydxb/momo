# momo-07 Task 9: PostgreSQL hybrid search (full-text plus vector, RRF)

Status: open
Created: 2026-11-04
Epic: momo-07-vectors-pgvector
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-07-vectors Task 9: PostgreSQL hybrid search (full-text plus vector, RRF) (`.procoder/plans/momo-07-vectors.md:1518`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-17. Files: `hybrid.go`, `hybrid_test.go`, `hybrid_integration_test.go`.

## Acceptance criteria

- [ ] `TestHybridSQL` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestHybridSearch` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-17] `TestHybridSearch`: a fixed corpus yields the asserted ordering with documented candidate counts and RRF k; fails if fusion is order-dependent on ties.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Built in sprint 7 as `momo.HybridSearch` in `select_hybrid.go` (root files need the `select` prefix, so not `hybrid.go`), tests in `select_hybrid_test.go` and `select_hybrid_integration_test.go`. The tests were written after the implementation, so no red run of `TestHybridSQL` or `TestHybridSearch` was recorded; criteria 1 and 2 stay unticked for that reason only (both tests pass). The first CI run on 0fff70e failed on test-string, file-prefix, fragment-ceiling and generated-docs problems (run 42769174243); the fixes are in ad7b82b.
- Green: ci-agent run 42784116091, headSha 2eda85d2041b0d0f6ec5200f96facc8aeddc8f96, conclusion success. Earlier green on the first version: run 42773205945 (ad7b82b). Review fixes since: NULL and NaN embeddings skipped, no type parameter on HybridSpec, Query and column-table validation, vector leg ordered by distance only, with live tests for single-leg hits, TextCandidates, NULL/zero vectors, soft delete plus schema, and an HNSW EXPLAIN check.
- Criteria 1 and 2: `TestHybridSQL` (SQL shape, bind order cfg/query/text candidates/vector/vector candidates/k/limit, defaults), `TestHybridSearch` (PostgreSQL integration job).
- Criterion 3 (S-17): `TestHybridSearch` asserts the order [1 3 2 4] with RRF k=60 and 100 candidates per leg, scores 2/61, 1/62, 1/63, 1/64, reversed insertion order giving the same result, a primary-key tie-break (ids 5 and 9 tie on 1/61+1/62, 5 first), VectorCandidates=2 bounding the vector leg, and RRFK=1 with Limit=1. Defaults and ranges are documented in docs/vectors.md "Hybrid search".
- Differences from the plan: the bind order follows first appearance in the SQL (text candidates third), and placement is the root package with the `select_` file prefix. Soft-deleted rows are excluded and a schema-qualified table is supported (`TestHybridSQLSoftDeleteAndSchema`).

