# momo-06 Task 18: Graph writes: CreateGraph, FindOrCreate and Upsert

Status: open
Created: 2026-11-04
Epic: momo-06-relations
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-06-relations Task 18: Graph writes: CreateGraph, FindOrCreate and Upsert (`.procoder/plans/momo-06-relations.md:4337`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-84. Files: `graph.go`, `internal/relfixture/tag/momo_gen.go`, `graph_test.go`.

## Acceptance criteria

- [x] `TestCreateGraphAndFindOrCreate` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestUpsertByPK` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-84] `TestCreateGraphAndFindOrCreate` asserts only listed relations are written, FindOrCreate is race-safe under concurrency (ON CONFLICT based), and Upsert updates all columns; fails if an unlisted relation is saved.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: with `graph.go` removed the tests fail to compile (`undefined: momo.CreateGraph`). Green: `go test -tags integration -race -count=1 -run 'CreateGraph|FindOrCreate|UpsertByPK|Assoc' .` against PostgreSQL passes.
- Tests as written in the plan, except `Upsert` is `UpsertByPK` (momo-05's `Upsert` builder already owns that name; see the plan's Task 18 reconciliation note). Extra tests cover belongs-to first, many-to-many, polymorphic, composite, self-reference, cycle rejection, rollback (DB and savepoint), field hooks on children and generated-identity propagation.
- Mutations that fail the tests: dropping `SetChildKey`, dropping the RETURNING read-back, FindOrCreate always reporting created, skipping the belongs-to pass.
- Doc nits from the story 154 review applied to the `assoc.go` godoc (missing parent row, Link/Unlink take no parent lock, reflowed `*Tx` lock sentence).
- The full suite, repo-wide race and lint gates were not run locally (shared disk); CI runs them on push. The last criterion stays open until CI is green.
