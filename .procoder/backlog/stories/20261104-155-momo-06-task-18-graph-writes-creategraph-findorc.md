# momo-06 Task 18: Graph writes: CreateGraph, FindOrCreate and Upsert

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: -

## Description

Implements plan momo-06-relations Task 18: Graph writes: CreateGraph, FindOrCreate and Upsert (`.procoder/plans/momo-06-relations.md:4337`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-84. Files: `graph.go`, `internal/relfixture/tag/momo_gen.go`, `graph_test.go`.

## Acceptance criteria

- [ ] `TestCreateGraphAndFindOrCreate` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestUpsertByPK` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-84] `TestCreateGraphAndFindOrCreate` asserts only listed relations are written, FindOrCreate is race-safe under concurrency (ON CONFLICT based), and Upsert updates all columns; fails if an unlisted relation is saved.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

