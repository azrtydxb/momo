# momo-01 Task 14: Keyset pagination with deterministic tie-breaking

Status: open
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 14: Keyset pagination with deterministic tie-breaking (`.procoder/plans/momo-01-foundation.md:1237`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4. Files: `select.go`, `keyset_test.go`. Contributes to S-4; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestKeysetPagination` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

