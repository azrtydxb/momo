# momo-04 Task 35: SQLite 12-step rebuild

Status: closed
Created: 2026-11-04
Epic: momo-04-migrations-sqlite
Sprint: -

## Description

Implements plan momo-04-migrations Task 35: SQLite 12-step rebuild (`.procoder/plans/momo-04-migrations.md:3679`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-13. Files: `migrate/sqlite/rebuild.go`.

## Acceptance criteria

- [x] `TestSQLiteRebuild` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSQLiteRebuildFKStillEnforced` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSQLiteRebuildOrphanRollsBack` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestSQLiteRebuildBlocked` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-13] `TestSQLiteRebuild`: a rebuild-requiring change preserves row count and content, indexes and triggers, and `PRAGMA foreign_key_check` returns no rows; fails if the rebuild drops a trigger or index.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Tests (migrate/sqlite/rebuild_test.go, integration): `TestSQLiteRebuild` (3 rows and sum(qty) kept, index `items_name`, triggers `items_audit` and `links_touch`, view `items_v` present and readable, new default in place, `pragma_foreign_key_check` empty, `foreign_keys` back to 1), `TestSQLiteRebuildFKStillEnforced`, `TestSQLiteRebuildOrphanRollsBack` (error names foreign_key_check and rollback, is `momo.ErrForeignKeyViolation`, original table text, rows, triggers, view and index restored), `TestSQLiteRebuildBlocked`, `TestSQLiteCaptureFollowsViews`; unit tests `TestSQLiteRenderSchemaRebuildOrder` (normative statement order), `TestSQLiteRebuildFoldsTableChanges`, `TestSQLiteRebuildStatementsPure`; end to end `TestSQLiteRebuildKeepsDataAndObjects` and, through `Engine.Diff` and `Engine.Apply`, `TestSQLiteEngineLifecycle` and `TestSQLiteEngineRebuildOrphanFailsAndRestores` (foreign keys restored and lock released after a failed rebuild).
- Red: ci-agent run 42467369597 (headSha 374d20a44bafd45aaeb802b85aca4de0ad0b87ab) on the throwaway branch worktree-agent-a2047783134c30958-red, which held only the tests on top of sprint-5 (deleted afterwards): unit, lint and every integration job failed to build (`undefined: tokenize`, `undefined: parseTableSQL`, `undefined: enumCheck`, `undefined: msqlite.New`, `migrate.SessionOptions` has no field `ForeignKeysOff`).
- Green: ci-agent run 42481107465 (headSha e0ccf4868ffb93980c85e07e1d394a7cefbd8800) on worktree-agent-a2047783134c30958, after three review rounds (round 3 added TestSQLiteRebuildWithoutRowIDTable and the fail-closed pragma_foreign_key_check checks in TestForeignKeyCheckFailsClosed): lint, unit (make test, test-compilefail, test-race), fuzz-smoke, bench-module, PostgreSQL integration and MySQL 8.4, MySQL 9.7 and MariaDB integration all success. The integration jobs run `go test -tags integration -race ./...`, which includes migrate/sqlite against a real SQLite. The per-test PASS listing (run 42470163697) predates the review fixes; the tests added since are named in the next bullet.
- Per-test output: ci-agent run 42470163697 (headSha e98500bce10e87834c069617267450b55309d36f) on the throwaway branch worktree-agent-a2047783134c30958-verbose (deleted afterwards), the same code with the PostgreSQL integration target narrowed to `go test -v -tags integration -race ./migrate/sqlite/`: 51 PASS lines, 0 FAIL, 0 SKIP; every test named below shows PASS.
- Review fixes proven by tests in that run: TestSQLiteAutoMigrateSkipsRebuilds and TestSQLiteAutoMigrateWithStrictTable (AutoMigrate skips rebuilds and is not stopped by a STRICT, AUTOINCREMENT or WITHOUT ROWID table), TestSQLiteRollbackRebuildKeepsChildRows, TestSQLiteStrictAutoincrementRoundTrip (options and sqlite_sequence survive a rebuild), TestSQLiteForeignKeyCycleBetweenNewTables, TestSQLiteCreatedTableForeignKeyCycle, TestSQLiteRebuildCarriesTableOptions, TestDiffSQLiteTableOptions, TestForeignKeyCheckStatement, TestForeignKeyCheckFailsClosed, TestIsUndefinedTable, TestNormalizeSQLiteIdentity, TestNormalizeDefinition.
- Gate: the commit hook ran `procoder check` with 0 blocking findings.

### Deviations from the plan

- The plan's `Render(plan)` receives only the change list, but a rebuild needs the whole desired table, the starting columns and the views and triggers that mention it. Added `migrate.SchemaRenderer` (`RenderSchema(from, to, plan)`) in migrate/dialect.go; `Engine.Diff` and `AutoMigrate` call it in place of `Render` when the dialect has it. `Render` itself follows the plan (one marker statement with `Hint: "rebuild"` per table, no SQL, not executable).
- A migration file cannot hold statement hints, so the engine path recognises the rebuild by text: migrate/runner.go `execStatement` treats `PRAGMA foreign_key_check` (or Hint `fk-check`) as the check and fails on a returned row with a `momo.ErrForeignKeyViolation` error; `Engine.Diff` writes the existing directive `-- momo:sqlite-foreign-keys-off` into any up file that contains the check; `loadPending` reads it into the new `Pending.ForeignKeysOff`; `Apply` (and `AutoMigrate`) call `Dialect.Setup(ForeignKeysOff)` around that migration on SQLite and restore afterwards. The runner change is limited to `execAll` calling `execStatement`.
- The `dependent:<type>:<name>` hint (`migrate.ErrRebuildBlocked`) exists only on statements returned in memory by `sqlite.Rebuild` / `RenderSchema`; a migration read back from a file reports the failing statement number instead.
- `CREATE VIEW` does not check the names in its query, so a recreated view would never fail. After each recreated view the script runs `SELECT 1 FROM "view" LIMIT 0` (same hint), which makes SQLite resolve the names; that is what makes `TestSQLiteRebuildBlocked` fail as the plan wants. A trigger body is resolved only when it fires and cannot be checked this way (documented).
- `Capture` matches table names with the tokenizer (quoted identifiers count, string literals do not) instead of the regexp `(?i)\b<table>\b`, and also captures views and triggers that mention an already captured view, because RENAME fails while a view names a missing view.
- `RebuildStatements(table, oldColumns, desired, desiredIndexes, deps)` returns `([]migrate.Statement, error)` (an enum column without values, or a dependent that is neither view nor trigger, is refused); `Rebuild` is `Capture` plus the column read.
- The engine path (`RenderSchema`) builds the dependents from the starting and desired schemas, not from the live database, and recreates only those the desired schema keeps unchanged; a changed or removed one has its own plan change.
