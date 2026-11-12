# VIRTUAL generated columns read as STORED on MySQL and SQLite

Status: done 2026-11-11
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #111. Reproduce: create a table with a `VIRTUAL` generated column
on MySQL or SQLite, inspect it, then `migrate diff` against the same schema.

Observed: the MySQL inspector read `GENERATION_EXPRESSION` without checking
`EXTRA`, and the SQLite inspector accepted `hidden` 2 (virtual) and 3 (stored)
alike. `schema.Column` had no kind, so migrate always rendered
`GENERATED ALWAYS AS (...) STORED`. A schema baselined from a live database
created a STORED column on a fresh install while diff and drift reported
nothing.

Expected: the kind round-trips: it is inspected, diffed and rendered.

Root cause: the schema model had no field for the generated kind.

## Acceptance criteria

- [x] `TestMySQLVirtualGeneratedRoundTrips` and `TestSQLiteVirtualGeneratedRoundTrips` are red before the fix and green after
- [x] both tests assert the inspected column has `GeneratedVirtual` set and that `migrate.Diff` of the inspected table against its declaration is empty
- [x] both tests assert a fresh migrate creates a VIRTUAL column (`information_schema.COLUMNS.EXTRA` on MySQL, `pragma_table_xinfo.hidden` = 2 on SQLite)
- [x] both tests assert a VIRTUAL to STORED declaration produces a diff change
- [x] `docs/migrations.md` and the 0.8.0 section of `docs/upgrading.md` describe the kind, the `virtual` tag, the MySQL and PostgreSQL kind-change refusal, and the PostgreSQL `momo.ErrUnsupported`

## Evidence

- Red: run 42993372557 on dabedb0 (test only plus the schema field). Failing
  lines: `virtual_integration_test.go:73: inspected kinds: slug virtual=false
(want true)` on mysql84, mysql97 and mariadb; `virtual_test.go:57: inspected
kinds: slug virtual=false (want true)` for SQLite. Branch deleted.
- Green: ci-agent run 43004865591 on 85c80723c183d35e0d426dde15b7a82ee0903309,
  all 9 jobs success, including integration-mysql (mysql84, mysql97, mariadb),
  integration (postgres), apidiff-docs.
- No regenerated-docs artifact was produced; apidiff-docs passed.
- Docs: docs/migrations.md, docs/upgrading.md.

### Review fix round (items 1 to 5)

- Red: run 43012011060 on d086e1f3 (branch worktree-agent-red-111b, deleted).
  Failing tests: `TestTagGrammar` and `TestVirtualTag` (`unknown tag option
"virtual"`), `TestGenerateFromDBVirtualGenerated`,
  `TestRenderGeneratedKindChangeIsRefused` (`err = <nil>`, want the custom
  migration error) and `TestVirtualRefusedOnPostgres` (`diagnostics = ""`).
- Green: ci-agent run 43174315311 on 9a9244a933986ff3d4b4c9c867eb2575c887b6cb,
  all 9 jobs success. `docs/reference.md` came from the regenerated-docs
  artifact of run 43012090587 (headSha 1451691b).
- New test files: `gen/virtual_test.go`, `migrate/mysql/virtual_render_test.go`,
  `migrate/postgres/virtual_render_test.go`, `migrate/virtual_diff_test.go`,
  plus additions to `gen/tags/tags_test.go`, `gen/fromdb/fromdb_test.go`,
  `migrate/mysql/virtual_integration_test.go` and `migrate/sqlite/virtual_test.go`
  (`TestSQLiteVirtualToStoredRebuildKeepsData`).
- `--from-db` supports PostgreSQL only, so it cannot read a MySQL or SQLite
  table. `TestVirtualModelDiffsCleanAgainstInspected` replaces the end-to-end
  check: a model with `virtual` diffs clean against a column as the MySQL
  inspector reports it.
