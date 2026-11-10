# Insert.Model keeps database defaults: the dbdefault tag (issue #69)

Status: open
Created: 2026-11-10
Epic: momo-05-writes-queries
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

`Insert.Model(&m)` writes every writable column, so `DEFAULT gen_random_uuid()` or `DEFAULT now()` is overwritten by the Go zero value unless every call site uses `Omit`. Most of Kuvryn Orbit's 253 tables default ids and timestamps in the database. The ask is in issue #69.

## Acceptance criteria

- [ ] a `dbdefault` tag: when the field holds its zero value the column is left out of the INSERT, and its value is read back through RETURNING into the model (unit test pins SQL; PostgreSQL integration test with `gen_random_uuid()` and `now()`)
- [ ] a non-zero value is written as given; bulk `Insert.Models` with mixed zero/non-zero rows is handled correctly (split or refused with a clear error) (test)
- [ ] MySQL and SQLite behave correctly (RETURNING where supported, otherwise documented re-read or refusal) (tests); the generator and `--from-db` emit the tag for columns with a default where appropriate
- [ ] docs describe it
