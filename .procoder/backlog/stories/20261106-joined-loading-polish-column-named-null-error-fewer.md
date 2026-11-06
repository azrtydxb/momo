# Joined loading polish: column-named NULL error, fewer allocations, coverage

Status: open
Created: 2026-11-06
Epic: momo-06-relations
Sprint: -

## Description

Follow-up from the story 152 re-review (single-query join loading). A
developer using `Joined()` should get the same error quality and roughly
the same cost as a plain `Load`, and the edge cases the review found
untested should be pinned. Today a matched child row with NULL in a
non-nullable column fails with "NULL into a non-nullable field" without
naming the column, each joined row costs about 38 allocations against
about 25 for a plain parent plus child row, and several shapes have no
test.

## Acceptance criteria

- [ ] A matched joined child with NULL in a non-nullable column returns an
      error naming the child column (`joinBinding.assign` wraps with
      `ChildMeta.Columns[i].Name`); an integration test asserts the name.
- [ ] Per-row `**T` holders are allocated once per binder and reset per
      row without sharing pointees; a benchmark shows fewer allocations
      per joined row than before (record before/after numbers).
- [ ] Integration test: two different `Joined()` relations in one query
      load correctly.
- [ ] Integration test: non-pointer `types.UUID` and `types.Date` child
      fields and a value-receiver `sql.Scanner` field load through
      `Joined()` identically to plain `Load`.
- [ ] Unit test for `scanNull` with named slice, map and interface kinds.

## Evidence
