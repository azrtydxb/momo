# momo-04 Task 8: Pure diff engine with explicit renames, destructive gating, staged NOT NULL, FK actions

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 8: Pure diff engine with explicit renames, destructive gating, staged NOT NULL, FK actions (`.procoder/plans/momo-04-migrations.md:934`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-10, S-11, S-74. Files: `migrate/diff.go`. Contributes to S-10, S-11; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestNoRenameGuessing` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestDestructiveRequiresFlag` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestStagedChangeDiff` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestFKActions` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestDiffNoChangeEmpty` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-74] `TestFKActions` asserts each action's effect on every dialect and that the no-constraint option produces no FK in inspection; fails if actions are lost in a round trip.
- [x] `procoder check` gate is clean and the task's commit is made
- [x] `Column.Collate` and `Column.Comment` are diffed: a change to either yields a column change (momo-02 Task 10 builds them from `collate` and `comment` tags and MomoConfig), with a round-trip test

## Evidence

- Tests (plan's literal five plus explicit renames, destructive type change/drop table, normalized types, staged NULL to NOT NULL, collate/comment round trip, FK action change, create/function/view changes): FAIL before (`undefined: Diff`), PASS after.
- Mutation: removing the OnUpdate comparison in the FK equality made TestFKActions fail (no drop_fk/add_fk); reverted.
- go build, go vet, make staticcheck, go test -race ./... pass; procoder check: 0 blocking findings.
- Commit: feat(migrate): pure diff with explicit renames, destructive and staged gating (migrate/diff.go, migrate/diff_test.go, ChangeKind constants in migrate/types.go).
- S-74 criterion evidence: the diff part is done (`TestFKActions`: add_fk, drop_fk+add_fk on an OnDelete/OnUpdate change, NO ACTION equals default). The rest (every dialect, no-constraint option absent from inspection) is carried by Task 13 (story 082) and the SQLite render. Not done here: the S-74 criterion (FK actions on every dialect, no-constraint option absent from inspection) needs the PostgreSQL/SQLite inspect and render tasks; only the diff part is covered. Left unchecked.
- Deviation: an added NOT NULL column that is a rename candidate (same table and type as a dropped column) does not raise ErrStagedRequired, because the plan's TestNoRenameGuessing expects no error there; the rename hint is always emitted.
- Review fixes: column renames now remap PK, unique, index, FK (local and referenced) and check constraints (`TestRenamedColumnRemapsConstraints`; removing the remap fails it); table-rename lookup is schema-qualified and sorted; dropped tables emit their own drop_fk; added determinism, enum-removal, partition, WITHOUT ROWID and hint-only removal tests.
