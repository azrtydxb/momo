# Raw SQL: Query[T] and QueryRows for multi-column results (issue #63)

Status: open
Created: 2026-11-10
Epic: momo-05-writes-queries
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

A hand-written statement that returns several columns has no momo path. `Exec` returns a count, `Scalars` returns one column, and `Querier` is sealed. Kuvryn Orbit's OQL compiler builds parameterized SQL from an AST, and Orbit has about 1,700 inline multi-column `SELECT`/`RETURNING` calls. Issue #63 asks for a raw query API that scans into a struct or `RowN`, with the same placeholder rules and error classification as `Exec`.

## Acceptance criteria

- [x] `momo.Query[T]`, called with `ctx, q, sql, args...`, returns `[]T` for T a model, a projection struct or `Row1`..`Row10`; columns are matched like typed selects, and an unknown or missing column is a classified error (unit and PostgreSQL integration tests)
- [x] a streaming form (`QueryRows` or an iterator) reads rows without buffering them all, and closes on early break and on error (integration test)
- [x] `$n` and `?` placeholders behave as in `Exec`, errors are classified (unique violation, canceled) and redacted as in `Exec`, and the query goes through hooks; like Exec it does not run interceptors (db_interceptor.go: raw SQL never runs interceptors) (tests: TestQueryRunsHooksAndTx, TestQueryTupleRow, TestQueryErrorsClassifiedLikeExec)
- [ ] works on `*DB`, `*Tx` and a pinned handle; MySQL and SQLite run the same integration test; docs (ai-guide, raw SQL page) describe it

## Evidence

All evidence is ci-agent run 42785541466 (headSha 539e69ae3af28902555959e238b46d8ec08813af), all 9 jobs green: unit, integration postgres, mysql84, mysql97, mariadb.

- Query[T] on models, projections and Row1..Row10, unknown/missing/duplicate column and tuple arity errors: raw_query_test.go (TestQueryTupleRow, TestQueryModelMatchesByName, TestQueryTupleArityMismatch, TestQueryModelWithoutColumnNames), raw_query_integration_test.go (TestQueryMultiColumn, TestQueryColumnErrors).
- Streaming, no buffering, close on early break and on error and cancel: TestQueryRows (unit), TestQueryRowsStreaming (integration).
- Placeholders, classification (unique violation, canceled) and redaction parity with Exec, hooks (op raw): TestQueryTupleRow, TestQueryRunsHooksAndTx, TestQueryErrorsClassifiedLikeExec. Interceptors: not run, like Exec (db_interceptor.go).
- *DB, *Tx, pinned handle (where the driver is a Pinner): TestQueryMultiColumn. The integration tests (rqEach) run testopen.Each (PostgreSQL and SQLite) plus MySQL 8.4, MySQL 9.7 and MariaDB in their jobs; the MySQL-family jobs failed before the mysqldriver Columns() fix and pass now. Caveat: the CI logs do not use go test -v, so the SQLite subtests are not individually visible; whether SQLite ran in the postgres job (MOMO_INTEGRATION_DIALECTS=postgres) is unconfirmed.
- Docs: docs/ai-guide.md (raw SQL section), docs/capabilities.md, docs/momovet.md, docs/PRIVACY_RULES.md, docs/agents.md, docs/ARCHITECTURE.md; there is no separate raw SQL page, so ai-guide is the raw SQL reference. apidiff-docs and docscheck green.
