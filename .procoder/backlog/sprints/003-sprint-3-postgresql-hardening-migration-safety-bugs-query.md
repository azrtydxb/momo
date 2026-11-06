# Sprint 3: PostgreSQL hardening - migration safety bugs, query and relations polish

Status: active
Created: 2026-11-06

## Goal

An application team can run Momo migrations against a production PostgreSQL
database without the known v0.2.0 traps: staged NOT NULL is enforced,
`Concurrently()` and the destructive directive work, apply refuses broken
chains, and history never leaks credentials. Projections respect soft
delete, joined and graph loading cover the edge cases the reviews found, and
conditional upserts work on SQLite partial indexes. Every fix ships with a
regression test and the docs' known-issues lists shrink to match.
