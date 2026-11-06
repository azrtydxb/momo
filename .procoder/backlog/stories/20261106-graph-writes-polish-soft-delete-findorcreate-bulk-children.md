# Graph writes polish: soft-delete FindOrCreate, bulk children, docs

Status: done 2026-11-06
Created: 2026-11-06
Epic: momo-06-relations
Sprint: 003-sprint-3-postgresql-hardening-migration-safety-bugs-query

## Description

Follow-up from the story 155 review (graph writes). A developer calling
`FindOrCreate` or `CreateGraph` should get pinned, documented behaviour in
the edge cases the review found untested or undocumented. Today a
soft-deleted row is returned by `FindOrCreate` as "found" with no test,
`CreateGraph` inserts every child with its own statement (10k children
means 10k round trips), the MySQL path fails with `ErrUnsupported` only
after the transaction has started, and the godoc does not say that
already-persisted many-to-many children are re-inserted rather than
linked.

## Acceptance criteria

- [x] An integration test with a soft-deletable model pins what
      `FindOrCreate` does when the matching row is soft-deleted, and the
      godoc states it (decide: return it, restore it, or error).
- [x] `CreateGraph` inserts each relation's children with the chunked bulk
      insert path, or its godoc states the per-row cost; a test asserts
      the statement count for a 1000-child graph.
- [x] `CreateGraph` and `FindOrCreate` on a dialect without RETURNING or
      ON CONFLICT return `ErrUnsupported` before opening a transaction; a
      unit test covers it.
- [x] The `CreateGraph` godoc says already-persisted children are
      re-inserted (and fail with `ErrUniqueViolation`) and points to
      `Link` for existing children; a test covers the many-to-many case.

## Evidence

- Criterion 1: decision is "a soft-deleted row is not found; the insert conflicts and the call fails with ErrUniqueViolation, nothing is returned or restored". `TestFindOrCreateSoftDeletedMatchIsNotFound` (unit, mutation: re-select with WithDeleted fails it) and integration `TestFindOrCreateSoftDeletedRowIntegration` (CI run 42304152615); godoc and docs/relations.md state it.
- Criterion 2: `TestCreateGraphInsertsChildrenInBulk` asserts 1 comment INSERT for 1000 children and 2 for 9000 over the bind limit (mutation: per-row insert fails it); integration `TestCreateGraphThousandChildrenIntegration`.
- Criterion 3: `TestGraphWritesUnsupportedBeforeTransaction` proved ErrUnsupported with zero Begin calls and zero statements on MySQL for both CreateGraph and FindOrCreate (mutation: removing the check fails it).
- Criterion 4: CreateGraph godoc and docs say persisted children are re-inserted and point to Link; integration `TestCreateGraphPersistedManyToManyChildIntegration` asserts ErrUniqueViolation, rollback, and Link working (CI run 42304152615).
