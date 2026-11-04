# momo-06 Task 14: Pipelined relation loading over pgx batching

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: -

## Description

Implements plan momo-06-relations Task 14: Pipelined relation loading over pgx batching (`.procoder/plans/momo-06-relations.md:2923`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-51. Files: `pending`, `exec.go`, `internal/relfixture/count.go`, `pipeline_test.go`.

## Acceptance criteria

- [ ] `TestPipelinedEagerLoad` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-51] `TestPipelinedEagerLoad` asserts two relations load in one round trip (driver round-trip counter) with identical results to sequential loading; fails if results differ.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

