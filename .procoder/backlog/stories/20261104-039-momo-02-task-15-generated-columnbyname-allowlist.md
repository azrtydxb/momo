# momo-02 Task 15: Generated ColumnByName allowlist (S-43)

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 15: Generated ColumnByName allowlist (S-43) (`.procoder/plans/momo-02-generator.md:1561`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-43. Files: `gen/emit_lookup.go`, `gen/emit_lookup_test.go`, `gen/testdata/fixtures/sensitive/s.go`, `gen/testdata/golden/descriptors/document/lookup_test.go`.

## Acceptance criteria

- [ ] `TestColumnByName` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestColumnByNameRuntime` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-43] `TestColumnByName` rejects unknown names and returns the typed column for known ones; `TestEscapeLike` matches literal `%`, `_` and the escape character on both dialects; fails if a name is ever quoted into SQL without the allowlist.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

