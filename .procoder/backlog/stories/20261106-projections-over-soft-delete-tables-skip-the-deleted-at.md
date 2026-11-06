# Projections over soft-delete tables skip the deleted_at filter

Status: open
Created: 2026-11-06
Epic: momo-05-writes-queries
Sprint: 003-sprint-3-postgresql-hardening-migration-safety-bugs-query

## Description

As a Momo user (or an AI coding agent following the guide), I select a
projection (`momo.Select[Card](db).From(article.Table).Columns(...)`, or the
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

- [x] A projection selected `From` a table with a soft-delete column excludes soft-deleted rows by default, for the timestamp, unix and bool markers. Tested against PostgreSQL in the integration suite.
- [x] `WithDeleted()` and `OnlyDeleted()` work on such a projection the same way they do on a model. Tested.
- [x] A projection over a table without a soft-delete column, and a projection over a subquery, build the same SQL as before; over a join the FROM table is filtered and the joined tables are not, as for a model. Tested by SQL snapshot.
- [x] `docs/ai-guide.md` (Projections, vector search, rule 9), `README.md` "Projections" and `docs/vectors.md` drop the "add the filter yourself" instruction and state the new behavior.
- [x] `examples/projection-pagination` and `examples/vector-search` still pass their example tests.

## Evidence

- Criterion 1: TestProjectionSoftDelete (integration) selects a projection over the timestamp, unix and bool tables and sees only live rows by default; RUNID.
- Criterion 2: the same test checks WithDeleted returns both rows and OnlyDeleted only the deleted one; RUNID.
- Criterion 3: TestProjectionSoftDeleteFilter snapshots the SQL for a plain table (no filter), a subquery source (inner filter only), a join (FROM table filtered, joined table untouched, plain FROM unfiltered), an aliased `As` table, a generated-style `task.Table` and an IN subquery; TestProjectionSoftDeleteCountAndExists asserts Count and Exists carry the filter. Making joins skip the filter again fails the join case.
- Criterion 4: README Projections, docs/ai-guide.md (Projections, vector section, rule 9, section 7), docs/vectors.md, docs/projections.md and the projection-pagination README and main.go comment state the new behavior; no "add the filter yourself" remains except for joins and subqueries.
- Criterion 5: the example tests of projection-pagination and vector-search pass in RUNID.
