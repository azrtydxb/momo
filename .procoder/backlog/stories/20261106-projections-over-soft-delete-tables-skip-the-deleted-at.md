# Projections over soft-delete tables skip the deleted_at filter

Status: open
Created: 2026-11-06
Epic: momo-05-writes-queries
Sprint: -

## Description

As a Momo user (or an AI coding agent following the guide), I select a
projection (`momo.Select[Card](q).From(article.Table).Columns(...)`, or the
vector `Hit`) over a table that has a soft-delete column and expect soft-deleted
rows to stay hidden, as they are for a model. They are not: the filter is chosen
from the result type's metadata (`sq.meta.SoftDelete >= 0` in `select.go`), and a
projection's metadata has `SoftDelete: -1`, so the SQL is
`SELECT id, title, score FROM articles` with no `deleted_at IS NULL`. The docs
now tell users to add the predicate by hand. Done means the library does it, so
a projection cannot leak deleted rows by omission.

Two designs are acceptable: apply the filter automatically when the projection's
`From` is a single table that has a soft-delete column (the table's metadata is
known from the generated column set), or have the generator emit projection
metadata that carries the soft-delete column. The second ties in with
[Generate Meta for projection structs](20261106-generate-meta-for-projection-structs.md).

## Acceptance criteria

- [ ] A projection selected `From` a table with a soft-delete column excludes soft-deleted rows by default, for the timestamp, unix and bool markers. Tested against PostgreSQL in the integration suite.
- [ ] `WithDeleted()` and `OnlyDeleted()` work on such a projection the same way they do on a model. Tested.
- [ ] A projection over a table without a soft-delete column, and a projection over joins or a subquery, build the same SQL as before. Tested by SQL snapshot.
- [ ] `docs/ai-guide.md` (Projections, vector search, rule 9), `README.md` "Projections" and `docs/vectors.md` drop the "add the filter yourself" instruction and state the new behavior.
- [ ] `examples/projection-pagination` and `examples/vector-search` still pass their example tests.

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
