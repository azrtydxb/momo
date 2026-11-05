# momo-07 Task 10: Pinned pgvector versions and version assertion

Status: open
Created: 2026-11-04
Epic: momo-07-vectors-pgvector
Sprint: -

## Description

Implements plan momo-07-vectors Task 10: Pinned pgvector versions and version assertion (`.procoder/plans/momo-07-vectors.md:1782`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-15, S-20. Files: `internal/testdb/compose.yaml`, `internal/testdb/pgvector_version_test.go`. Contributes to S-15, S-20; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestPgvectorVersionPinned` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

