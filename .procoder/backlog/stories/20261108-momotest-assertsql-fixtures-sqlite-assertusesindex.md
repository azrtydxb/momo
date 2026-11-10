# momotest: AssertSQL, file-based LoadFixtures, momotest.SQLite and AssertUsesIndex

Status: open
Created: 2026-11-08
Epic: momo-09-observability-testing
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Story 172 (momo-09 Task 8) was narrowed to the issue #34 scope, which left four parts of the original plan untracked: `momotest.AssertSQL`, `momotest.LoadFixtures` reading a YAML file, `momotest.SQLite`, and `momotest.AssertUsesIndex`. The executable specification is in `.procoder/plans/momo-09-observability-testing.md`: Task 8 step 5 for `AssertSQL`, Task 9 for `LoadFixtures` and `SQLite`, Task 10 for `AssertUsesIndex`. Spec coverage: S-34 (fixtures, ephemeral SQLite) and S-55 (index assertion).

What done means for a user writing tests against momo: they can assert the SQL a query compiles to without running it and without leaking bound values; load table fixtures from a YAML file in file order; get a throwaway SQLite database per test; and assert that a query plan uses a named index on PostgreSQL or SQLite.

Dependency: `momotest.SQLite` needs the SQLite driver (`sqlitedriver.Open`, `testdb.SQLitePath`) from the M2 epics `momo-03-drivers-sqlite`. That part is blocked until the SQLite driver branch lands; the other three parts do not depend on it and ship first, and the SQLite criteria stay open until the driver exists. `AssertUsesIndex` on SQLite is likewise only exercised once the driver exists; its PostgreSQL path is not blocked.

## Acceptance criteria

- [ ] `AssertSQL` compares `Compiled.SQL` with the expected text ignoring whitespace differences, calls `tb.Helper`, and only compiles, never executes (the hook sees no statement); `TestMomotestSQLAssert` covers a match with reformatted whitespace, a mismatch, and a RawSubquery or Insert case
- [ ] on mismatch the message holds both SQL texts, the bound-argument count and `Compiled.String`, and never an argument value: a test using a `FlagSensitive` column asserts the failure text contains `[redacted]` and not the value
- [ ] `LoadFixtures` reads a YAML mapping of table name to a list of rows, inserts tables in file order and rows in list order (so foreign keys can be satisfied by ordering), writes columns sorted, binds values, quotes names with `Ident`, and turns a YAML null into NULL (`TestMomotestFixtures` on PostgreSQL)
- [ ] `LoadFixtures` fails the test with a message naming the file, table and row for a missing file, a non-mapping top level, a non-list table, and a row that the database rejects (`testdata/bad.yaml`); loading order is deterministic over repeated runs (`-count=5`)
- [ ] `go.yaml.in/yaml/v3` is the only new module requirement and is already pinned in `go.mod` (tidy check green)
- [ ] `AssertUsesIndex` runs a plain EXPLAIN (never ANALYZE), so the statement is not executed, and passes when the named index appears as a whole token in an index access node (`Index Scan`, `Index Only Scan`, `Bitmap Index Scan`) on PostgreSQL (`TestAssertUsesIndex`, `TestPlanUsesIndex` on a fixture with and without the index)
- [ ] `AssertUsesIndex` fails with the printed plan, and says so when a sequential scan is present; it fails the test on an unsupported dialect, and an index whose name is a prefix of the asserted one does not match
- [ ] `momotest.SQLite` returns a `*momo.DB` on a fresh temporary file per call, two calls do not see each other's tables, and the file is removed at test cleanup (`TestMomotestSQLiteEphemeral`); blocked until the SQLite driver lands, and the story is not closed while this criterion is unchecked
- [ ] `AssertUsesIndex` has a SQLite case that matches `USING INDEX` and `USING COVERING INDEX` lines; blocked on the same driver
- [ ] each of the new exported functions has a runnable example and a section in `docs/ai-guide.md`, and the doc states that plans may echo literal values so tests should use non-sensitive data
- [ ] story 172's own criteria stay as they are and are not duplicated here; `procoder check` is clean

## Evidence
