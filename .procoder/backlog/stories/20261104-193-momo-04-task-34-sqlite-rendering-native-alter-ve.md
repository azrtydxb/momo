# momo-04 Task 34: SQLite rendering: native ALTER versus rebuild decision

Status: closed
Created: 2026-11-04
Epic: momo-04-migrations-sqlite
Sprint: -

## Description

Implements plan momo-04-migrations Task 34: SQLite rendering: native ALTER versus rebuild decision (`.procoder/plans/momo-04-migrations.md:3606`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-13. Files: `migrate/sqlite/render.go`. Contributes to S-13; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestSQLiteAlterDecision` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSQLiteUnsupportedKindsError` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Tests: migrate/sqlite/render_test.go `TestSQLiteAlterDecision` and `TestSQLiteUnsupportedKindsError` (as written in the plan), plus `TestSQLiteUnsupportedKindsAreTyped`, `TestSQLiteRenderNativeStatements`, `TestSQLiteCreateTableText`, `TestSQLiteCreateTableRefusals`, `TestSQLiteDefaultSQL`, `TestSQLiteNativeAddRestrictions`, `TestSQLiteRenderSchemaDropColumn`, `TestSQLiteRenderSchemaEnums`; the round trip render, apply, inspect, `migrate.Diff` finds nothing and `schema.Equal` holds: `TestSQLiteRenderApplyInspectDiffIsZero`, `TestSQLiteNativeAddColumnNeedsNoRebuild`, `TestSQLiteDropAndRenameNative` (roundtrip_test.go).
- Red: ci-agent run 42467369597 (headSha 374d20a44bafd45aaeb802b85aca4de0ad0b87ab) on the throwaway branch worktree-agent-a2047783134c30958-red, which held only the tests on top of sprint-5 (deleted afterwards): unit, lint and every integration job failed to build (`undefined: tokenize`, `undefined: parseTableSQL`, `undefined: enumCheck`, `undefined: msqlite.New`, `migrate.SessionOptions` has no field `ForeignKeysOff`).
- Green: ci-agent run 42481107465 (headSha e0ccf4868ffb93980c85e07e1d394a7cefbd8800) on worktree-agent-a2047783134c30958, after three review rounds (round 3 added TestSQLiteRebuildWithoutRowIDTable and the fail-closed pragma_foreign_key_check checks in TestForeignKeyCheckFailsClosed): lint, unit (make test, test-compilefail, test-race), fuzz-smoke, bench-module, PostgreSQL integration and MySQL 8.4, MySQL 9.7 and MariaDB integration all success. The integration jobs run `go test -tags integration -race ./...`, which includes migrate/sqlite against a real SQLite. The per-test PASS listing (run 42470163697) predates the review fixes; the tests added since are named in the next bullet.
- Per-test output: ci-agent run 42470163697 (headSha e98500bce10e87834c069617267450b55309d36f) on the throwaway branch worktree-agent-a2047783134c30958-verbose (deleted afterwards), the same code with the PostgreSQL integration target narrowed to `go test -v -tags integration -race ./migrate/sqlite/`: 51 PASS lines, 0 FAIL, 0 SKIP; every test named below shows PASS.
- Review fixes proven by tests in that run: TestSQLiteAutoMigrateSkipsRebuilds and TestSQLiteAutoMigrateWithStrictTable (AutoMigrate skips rebuilds and is not stopped by a STRICT, AUTOINCREMENT or WITHOUT ROWID table), TestSQLiteRollbackRebuildKeepsChildRows, TestSQLiteStrictAutoincrementRoundTrip (options and sqlite_sequence survive a rebuild), TestSQLiteForeignKeyCycleBetweenNewTables, TestSQLiteCreatedTableForeignKeyCycle, TestSQLiteRebuildCarriesTableOptions, TestDiffSQLiteTableOptions, TestForeignKeyCheckStatement, TestForeignKeyCheckFailsClosed, TestIsUndefinedTable, TestNormalizeSQLiteIdentity, TestNormalizeDefinition.
- Gate: the commit hook ran `procoder check` with 0 blocking findings.

### Deviations from the plan

- The plan's `Render(plan)` receives only the change list, but a rebuild needs the whole desired table, the starting columns and the views and triggers that mention it. Added `migrate.SchemaRenderer` (`RenderSchema(from, to, plan)`) in migrate/dialect.go; `Engine.Diff` and `AutoMigrate` call it in place of `Render` when the dialect has it. `Render` itself follows the plan (one marker statement with `Hint: "rebuild"` per table, no SQL, not executable).
- With schemas, `drop_column` is native only when the starting table text uses the column nowhere (primary key, UNIQUE, index key or WHERE, CHECK, foreign key on either side, generated expression, trigger, view). Plain `Render` cannot know and always rebuilds a `drop_column`.
- `add_column` also rebuilds for an enum column, an identity column and a column with a comment (ALTER TABLE cannot add them), and a parenthesised-expression or `CURRENT_*` default is refused natively for any added column, not only NOT NULL ones, as the SQLite documentation says.
- `create_enum`, `drop_enum` and `alter_enum_add_value` are `ErrUnsupported` in plain `Render` as the plan says. In `RenderSchema` the first two produce nothing (an enum is only the CHECK written with its columns) and `alter_enum_add_value` rebuilds the tables that use the enum; otherwise an enum column could never be created through `Engine.Diff`.
- Every unsupported kind returns an error wrapping `momo.ErrUnsupported`; so do materialized views, views with indexes, index storage parameters, non-btree methods, operator classes, NULLS ordering, column comments, serial identity and partitioning.
- `rename_column` with no `Before` payload (as in the plan's test) renders `RENAME COLUMN "new" TO "new"` instead of failing; `Engine.Diff` always sets `Before`.
- The `drop_fk` and `drop_index` changes of a dropped table render nothing (the table takes them along); `drop_view` and `drop_trigger` render `IF EXISTS`.
