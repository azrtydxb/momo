# momo-07 Task 10: Pinned pgvector versions and version assertion

Status: open
Created: 2026-11-04
Epic: momo-07-vectors-pgvector
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each
Carried: 002-generator-from-go-structs-with-its-type-prerequisites — Criterion 1 unmet as written: TestPgvectorVersionPinned exists and passes but never failed before the implementation, since compose.yaml was already pinned by momo-01; no honest red exists, only a mutation check.

## Description

Implements plan momo-07-vectors Task 10: Pinned pgvector versions and version assertion (`.procoder/plans/momo-07-vectors.md:1782`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-15, S-20. Files: `internal/testdb/compose.yaml`, `internal/testdb/pgvector_version_test.go`. Contributes to S-15, S-20; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestPgvectorVersionPinned` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Pins: internal/testdb/compose.yaml (0.8.7-pg17, 0.8.7-pg16) and the .github/workflows/ci.yml matrix (0.8.7-pg17, 0.8.7-pg16) agree; TestPgvectorPinsAgree (untagged, part of the normal suite) enforces it.
- Integration: TestPgvectorVersionPinned (build tag integration, as in the plan) PASS against pgvector/pgvector:0.8.7-pg17 and 0.8.7-pg16 containers (extversion 0.8.7).
- Mutation: setting pinnedPgvector to 0.8.6 made TestPgvectorVersionPinned and TestPgvectorPinsAgree fail; restored.
- NOT met: criterion 1, "failed before the implementation". compose.yaml was already pinned by momo-01 and the unpinned pgvector/pgvector:pg17 image also reports 0.8.7, so no honest red existed. Red was shown only through the mutation above.
- Gate: `procoder check` 0 blocking findings. Commit 71845d6.

