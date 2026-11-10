# Raw SQL: Query[T] and QueryRows for multi-column results (issue #63)

Status: open
Created: 2026-11-10
Epic: momo-05-writes-queries
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

A hand-written statement that returns several columns has no momo path. `Exec` returns a count, `Scalars` returns one column, and `Querier` is sealed. Kuvryn Orbit's OQL compiler builds parameterized SQL from an AST, and Orbit has about 1,700 inline multi-column `SELECT`/`RETURNING` calls. Issue #63 asks for a raw query API that scans into a struct or `RowN`, with the same placeholder rules and error classification as `Exec`.

## Acceptance criteria

- [ ] `momo.Query[T]`, called with `ctx, q, sql, args...`, returns `[]T` for T a model, a projection struct or `Row1`..`Row10`; columns are matched like typed selects, and an unknown or missing column is a classified error (unit and PostgreSQL integration tests)
- [ ] a streaming form (`QueryRows` or an iterator) reads rows without buffering them all, and closes on early break and on error (integration test)
- [ ] `$n` and `?` placeholders behave as in `Exec`, errors are classified (unique violation, canceled) and redacted as in `Exec`, and the query goes through hooks and interceptors (tests)
- [ ] works on `*DB`, `*Tx` and a pinned handle; MySQL and SQLite run the same integration test; docs (ai-guide, raw SQL page) describe it
