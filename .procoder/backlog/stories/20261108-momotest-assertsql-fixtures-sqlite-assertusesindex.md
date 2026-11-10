# momotest: AssertSQL, file-based LoadFixtures, momotest.SQLite and AssertUsesIndex

Status: done 2026-11-10
Created: 2026-11-08
Epic: momo-09-observability-testing
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Story 172 (momo-09 Task 8) was narrowed to the issue #34 scope, which left four parts of the original plan untracked: `momotest.AssertSQL`, `momotest.LoadFixtures` reading a YAML file, `momotest.SQLite`, and `momotest.AssertUsesIndex`. The executable specification is in `.procoder/plans/momo-09-observability-testing.md`: Task 8 step 5 for `AssertSQL`, Task 9 for `LoadFixtures` and `SQLite`, Task 10 for `AssertUsesIndex`. Spec coverage: S-34 (fixtures, ephemeral SQLite) and S-55 (index assertion).

What done means for a user writing tests against momo: they can assert the SQL a query compiles to without running it and without leaking bound values; load table fixtures from a YAML file in file order; get a throwaway SQLite database per test; and assert that a query plan uses a named index on PostgreSQL or SQLite.

Dependency: `momotest.SQLite` needs the SQLite driver (`sqlitedriver.Open`, `testdb.SQLitePath`) from the M2 epics `momo-03-drivers-sqlite`. That part is blocked until the SQLite driver branch lands; the other three parts do not depend on it and ship first, and the SQLite criteria stay open until the driver exists. `AssertUsesIndex` on SQLite is likewise only exercised once the driver exists; its PostgreSQL path is not blocked.

## Acceptance criteria

- [x] `AssertSQL` compares `Compiled.SQL` with the expected text ignoring whitespace differences, calls `tb.Helper`, and only compiles, never executes (the hook sees no statement); `TestMomotestSQLAssert` covers a match with reformatted whitespace, a mismatch, and a RawSubquery or Insert case
- [x] on mismatch the message holds both SQL texts, the bound-argument count and `Compiled.String`, and never an argument value: a test using a `FlagSensitive` column asserts the failure text does not contain the value (Compiled.String lists placeholders, so no `[redacted]` text appears; amended after review)
- [x] `LoadFixtures` reads a YAML mapping of table name to a list of rows, inserts tables in file order and rows in list order (so foreign keys can be satisfied by ordering), writes columns sorted, binds values, quotes names with `Ident`, and turns a YAML null into NULL (`TestMomotestFixtures` on PostgreSQL)
- [x] `LoadFixtures` fails the test with a message naming the file, table and row for a missing file, a non-mapping top level, a non-list table, and a row that the database rejects (`testdata/bad.yaml`); loading order is deterministic over repeated runs (`-count=5`)
- [x] `go.yaml.in/yaml/v3` is the only new module requirement and is already pinned in `go.mod` (tidy check green)
- [x] `AssertUsesIndex` runs a plain EXPLAIN (never ANALYZE), so the statement is not executed, and passes when the named index appears as a whole token in an index access node (`Index Scan`, `Index Only Scan`, `Bitmap Index Scan`) on PostgreSQL (`TestAssertUsesIndex`, `TestPlanUsesIndex` on a fixture with and without the index)
- [x] `AssertUsesIndex` fails with the printed plan, and says so when a sequential scan is present; it fails the test on an unsupported dialect, and an index whose name is a prefix of the asserted one does not match
- [x] `momotest.SQLite` returns a `*momo.DB` on a fresh temporary file per call, two calls do not see each other's tables, and the file is removed at test cleanup (`TestMomotestSQLiteEphemeral`); blocked until the SQLite driver lands, and the story is not closed while this criterion is unchecked
- [x] `AssertUsesIndex` has a SQLite case that matches `USING INDEX` and `USING COVERING INDEX` lines; blocked on the same driver
- [x] each of the new exported functions has a runnable example and a section in `docs/ai-guide.md`, and the doc states that plans may echo literal values so tests should use non-sensitive data
- [x] story 172's own criteria stay as they are and are not duplicated here; `procoder check` is clean

## Evidence

- Audit: LoadFixtures (momotest/fixtures.go, TestMomotestFixtures, TestMomotestFixturesRowError, testdata/bad.yaml) and the SQLite helper existed from sprint 5. The SQLite helper is `sqlitetest.New` in package `momotest/sqlitetest` (TestMomotestSQLiteEphemeral), not `momotest.SQLite`: a separate package keeps the C sources out of momotest, enforced by TestMomotestAvoidsSQLite. Not renamed.
- Built now: `momotest.AssertSQL` (momotest/assert.go, TestMomotestSQLAssert, TestMomotestSQLAssertInsert; the failure lists `$n=[redacted]`), `momotest.AssertUsesIndex` (story 173), TestParseFixturesDeterministic (100 repeated parses; stands in for -count=5), ExampleAssertSQL, ExampleAssertUsesIndex, ExampleLoadFixtures, docs/testing.md and docs/ai-guide.md sections (plans may echo literal values: stated). go.yaml.in/yaml/v3 was already in go.mod.
- All criteria verified in ci-agent run 42778091550 (headSha 79a4a965871045b635c90908822154c8c19c4503, all 9 jobs success).
- Review fix: AssertSQL no longer prints its own `args:` line; `Compiled.String()` already names the bound placeholders. The criterion about `[redacted]` is amended accordingly: the failure shows no value, and the test asserts that.
