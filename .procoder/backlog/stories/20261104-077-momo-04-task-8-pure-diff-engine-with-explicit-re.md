# momo-04 Task 8: Pure diff engine with explicit renames, destructive gating, staged NOT NULL, FK actions

Status: done 2026-11-10
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each
Carried: 002-generator-from-go-structs-with-its-type-prerequisites — Criterion [S-74] TestFKActions on every dialect and no-constraint option absent from inspection is unchecked: TestFKActions covers only the diff part. Also staged NOT NULL is never enforced after diff --stage (bug story 20261106-staged-not-null-is-never-enforced-after-diff-stage).

## Description

Implements plan momo-04-migrations Task 8: Pure diff engine with explicit renames, destructive gating, staged NOT NULL, FK actions (`.procoder/plans/momo-04-migrations.md:934`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-10, S-11, S-74. Files: `migrate/diff.go`. Contributes to S-10, S-11; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestNoRenameGuessing` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestDestructiveRequiresFlag` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestStagedChangeDiff` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestFKActions` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestDiffNoChangeEmpty` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-74] `TestFKActions` asserts each action's effect on every dialect and that the no-constraint option produces no FK in inspection; fails if actions are lost in a round trip.
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

### Sprint 7 (2026-11-10)

- [S-74]: `TestFKActions` now exists per dialect: `migrate/postgres/fkactions_integration_test.go`, `migrate/mysql/fkactions_integration_test.go` (MySQL 8.4, 9.7, MariaDB), `migrate/sqlite/fkactions_test.go`. Each renders and applies every action (NO ACTION, RESTRICT, CASCADE, SET NULL, and SET DEFAULT except on MySQL, where InnoDB rejects it) on delete and on update, asserts the schema round-trips, runs the delete or update on a live row and checks the effect, and asserts the no-constraint table inspects with no foreign key. A second diff is empty.
- Green: ci-agent run 42776939577 on 0dafa15e799e4e052ca9b05bc94eed3b231d421f (PostgreSQL and the three MySQL-family jobs), and ci.yml dispatch run 42779413962 on 0dafa15e799e4e052ca9b05bc94eed3b231d421f, every job success.
- The staged-NOT-NULL part of the Carried note is covered by story 20261106 (done).

### Sprint 7, review round (2026-11-10)

The first evidence above was replaced after review: the tests were rebuilt to run the real path. `internal/fkcase` builds the schemas from models through `gen.BuildSchema` (the on_delete, on_update and no_constraint tags), so the no-constraint table comes from a `no_constraint` relation and every action is spelled as a tag value. `TestFKActions` per dialect checks every action on live rows, the no-constraint table inspecting with no foreign key, and the diff against the inspection being empty; `TestFKActionsNoActionIsNotRestrict` (PostgreSQL) tells NO ACTION from RESTRICT with a deferred constraint; `TestFKActionsChangeOnExistingTable` changes OnDelete on an existing table and applies it on each dialect (a table rebuild on SQLite, with the row kept).

- Green: ci-agent run 42790304107 on b6e2615b0419dbde9268124a8f3efeb3ea1bd517 (PostgreSQL and MySQL 8.4, 9.7, MariaDB jobs), merged to sprint-7 as 06f7936.
- SQLite test named in a CI log: ci-agent run 42793140189 on throwaway branch worktree-agent-ad6b3740d97d2a3ae-v, headSha 1cfd319a9bec4b6e902d7b6a855800d77a370d48 (the final code plus one Makefile line that runs `go test -v -run TestFKActions ./migrate/...`). Its log shows `--- PASS: TestFKActions (0.02s)` and `--- PASS: TestFKActionsChangeOnExistingTable (0.01s)` followed by `ok github.com/azrtydxb/momo/migrate/sqlite 0.041s`; the same run shows the PostgreSQL tests, including `TestFKActionsNoActionIsNotRestrict`, passing. The MySQL variants are skipped in that job and run in the MySQL jobs of the run above.
