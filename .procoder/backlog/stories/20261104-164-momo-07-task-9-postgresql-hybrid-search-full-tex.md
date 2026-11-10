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
- [ ] [S-17] `TestHybridSearch`: a fixed corpus yields the asserted ordering with documented candidate counts and RRF k; fails if fusion is order-dependent on ties.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

