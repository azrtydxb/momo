# Graph writes polish: soft-delete FindOrCreate, bulk children, docs

Status: open
Created: 2026-11-06
Epic: momo-06-relations
Sprint: -

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

- [ ] An integration test with a soft-deletable model pins what
      `FindOrCreate` does when the matching row is soft-deleted, and the
      godoc states it (decide: return it, restore it, or error).
- [ ] `CreateGraph` inserts each relation's children with the chunked bulk
      insert path, or its godoc states the per-row cost; a test asserts
      the statement count for a 1000-child graph.
- [ ] `CreateGraph` and `FindOrCreate` on a dialect without RETURNING or
      ON CONFLICT return `ErrUnsupported` before opening a transaction; a
      unit test covers it.
- [ ] The `CreateGraph` godoc says already-persisted children are
      re-inserted (and fail with `ErrUniqueViolation`) and points to
      `Link` for existing children; a test covers the many-to-many case.

## Evidence
