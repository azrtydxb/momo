# Joined loading polish: column-named NULL error, fewer allocations, coverage

Status: done 2026-11-06
Created: 2026-11-06
Epic: momo-06-relations
Sprint: 003-sprint-3-postgresql-hardening-migration-safety-bugs-query

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

- [x] A matched joined child with NULL in a non-nullable column returns an
      error naming the child column (`joinBinding.assign` wraps with
      `ChildMeta.Columns[i].Name`); an integration test asserts the name.
- [x] Per-row `**T` holders are allocated once per binder and reset per
      row without sharing pointees; a benchmark shows fewer allocations
      per joined row than before (record before/after numbers).
- [x] Integration test: two different `Joined()` relations in one query
      load correctly.
- [x] Integration test: non-pointer `types.UUID` and `types.Date` child
      fields and a value-receiver `sql.Scanner` field load through
      `Joined()` identically to plain `Load`.
- [x] Unit test for `scanNull` with named slice, map and interface kinds.

## Evidence

- Criterion 1: `TestJoinBinderNullColumnIsNamed` (unit) proved the error is ErrValidation with Column "count" and Table set; integration `TestJoinLoadNullColumnErrorNamesColumn` asserts `column=name` (CI run 42304152615, green).
- Criterion 2: `BenchmarkJoinBinderRow` went from 13 to 8 allocs/op (1165 to 714 ns/op, 192 to 152 B/op) with the holders and scratch child built once per binder; `TestJoinBinderRowsDoNotShare` proved consecutive rows share no child and no pointee (mutation: handing out the scratch child fails it).
- Criterion 3: `TestJoinLoadTwoRelationsInOneQuery` (CI run 42304152615).
- Criterion 4: `TestJoinLoadValueTypesMatchPlainLoad` loads non-pointer `types.UUID`, `types.Date` and a value-receiver Scanner through `Joined()` and compares with plain load (CI run 42304152615).
- Criterion 5: `TestScanNullNamedKinds` covers named slice, map and interface kinds (plus the existing unnamed cases).
