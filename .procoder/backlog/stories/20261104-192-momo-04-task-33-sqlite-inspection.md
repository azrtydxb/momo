# momo-04 Task 33: SQLite inspection

Status: closed
Created: 2026-11-04
Epic: momo-04-migrations-sqlite
Sprint: -

## Description

Implements plan momo-04-migrations Task 33: SQLite inspection (`.procoder/plans/momo-04-migrations.md:3516`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-13, S-39. Files: `migrate/sqlite/inspect.go`. Contributes to S-13, S-39; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestSQLiteInspectRoundTrip` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Test: migrate/sqlite/inspect_test.go `TestSQLiteInspectRoundTrip` (as written in the plan) and `TestSQLiteInspectDetails` (composite and WITHOUT ROWID keys, named and unnamed UNIQUE, enum CHECK, generated and collated columns, implicit parent key, views, triggers, bookkeeping tables left out); parser unit tests and `FuzzParseDDL` (listed in `FUZZ_TARGETS`) in parse_test.go.
- Red: ci-agent run 42467369597 (headSha 374d20a44bafd45aaeb802b85aca4de0ad0b87ab) on the throwaway branch worktree-agent-a2047783134c30958-red, which held only the tests on top of sprint-5 (deleted afterwards): unit, lint and every integration job failed to build (`undefined: tokenize`, `undefined: parseTableSQL`, `undefined: enumCheck`, `undefined: msqlite.New`, `migrate.SessionOptions` has no field `ForeignKeysOff`).
- Green: ci-agent run 42481107465 (headSha e0ccf4868ffb93980c85e07e1d394a7cefbd8800) on worktree-agent-a2047783134c30958, after three review rounds (round 3 added TestSQLiteRebuildWithoutRowIDTable and the fail-closed pragma_foreign_key_check checks in TestForeignKeyCheckFailsClosed): lint, unit (make test, test-compilefail, test-race), fuzz-smoke, bench-module, PostgreSQL integration and MySQL 8.4, MySQL 9.7 and MariaDB integration all success. The integration jobs run `go test -tags integration -race ./...`, which includes migrate/sqlite against a real SQLite. The per-test PASS listing (run 42470163697) predates the review fixes; the tests added since are named in the next bullet.
- Per-test output: ci-agent run 42470163697 (headSha e98500bce10e87834c069617267450b55309d36f) on the throwaway branch worktree-agent-a2047783134c30958-verbose (deleted afterwards), the same code with the PostgreSQL integration target narrowed to `go test -v -tags integration -race ./migrate/sqlite/`: 51 PASS lines, 0 FAIL, 0 SKIP; every test named below shows PASS.
- Review fixes proven by tests in that run: TestSQLiteAutoMigrateSkipsRebuilds and TestSQLiteAutoMigrateWithStrictTable (AutoMigrate skips rebuilds and is not stopped by a STRICT, AUTOINCREMENT or WITHOUT ROWID table), TestSQLiteRollbackRebuildKeepsChildRows, TestSQLiteStrictAutoincrementRoundTrip (options and sqlite_sequence survive a rebuild), TestSQLiteForeignKeyCycleBetweenNewTables, TestSQLiteCreatedTableForeignKeyCycle, TestSQLiteRebuildCarriesTableOptions, TestDiffSQLiteTableOptions, TestForeignKeyCheckStatement, TestForeignKeyCheckFailsClosed, TestIsUndefinedTable, TestNormalizeSQLiteIdentity, TestNormalizeDefinition.
- Gate: the commit hook ran `procoder check` with 0 blocking findings.

### Deviations from the plan

- Catalog reads use the table-valued pragma functions (`pragma_table_xinfo(?)`, `pragma_index_list(?)`, `pragma_index_xinfo(?)`, `pragma_foreign_key_list(?)`) with a bound table name, because `PRAGMA table_xinfo(?)` takes no bind parameter. Same data.
- The table text is read with a small tokenizer (migrate/sqlite/tokens.go, parse.go) instead of regular expressions, so quotes, comments and nested parentheses are never split. The plan's `topLevelChecks(sql)` is `parseTableSQL(...).checks`.
- Constraint names: a named UNIQUE, CHECK or FOREIGN KEY keeps its name from the table text; an unnamed one gets `<table>_<cols>_key`, `<table>_check[N]` or `<table>_<cols>_fkey`.
- Index key collation is not reported: `schema.IndexPart` has no collation field. Column collation is.
- A REFERENCES with no column list resolves to the parent's primary key.
- Virtual tables and shadow tables (`pragma_table_list`, SQLite 3.37; an older library only loses the shadow-table filter) are left out; `View.Depends` is filled from the table and view names the query mentions.
- `Dialect.Render` and `Dialect.Lock` are implemented (stories 193 and 195) rather than stubbed, because the branch carries all four stories.
- schema/normalize.go gained two idempotent SQLite normalizations that the round trip needs: an INTEGER column that is the whole primary key of a rowid table is `by_default` on both sides (`normalizeSQLiteIdentity`), and `Trigger.Definition` is canonicalised like an expression with a trailing semicolon and IF NOT EXISTS removed (`normalizeDefinition`). `FuzzNormalize` now feeds both (a PRIMARY KEY column with a fuzzed identity, a fuzzed Definition).
- An enum column is inspected with the synthetic enum name `<table>_<column>` as the plan says, so a model enum round-trips only when it carries that name (documented in docs/migrations.md).
