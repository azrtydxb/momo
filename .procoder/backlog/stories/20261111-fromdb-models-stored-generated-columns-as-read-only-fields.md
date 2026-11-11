# fromdb models stored generated columns as read-only fields

Status: open
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue 103. `momo generate --from-db` skipped every
`GENERATED ALWAYS AS (...) STORED` column with the warning "generated column is
not modeled", so typed builders could not select or filter by it and the
baseline reported it.

Reproduce: create a table with
`normalized text GENERATED ALWAYS AS (lower(name)) STORED`, run
`momo generate --from-db`. Observed: the column is missing from the model and
the snapshot, and a warning is printed. Expected: a `read_only` field carrying
the expression, an unchanged `migrate diff`, no write of the column by Insert or
Update, and typed Select and Where on it.

Root cause: `gen/fromdb/emit.go` dropped any column with `Generated != ""`, and
the Go-struct path had no tag for the expression (`schema.Column.Generated` was
only reachable from the database inspectors), so a model could not declare one.

Fix: a new `generated:<expr>` tag and `Config.Generated` companion method (for an
expression with a comma, which a tag cannot hold) map to `schema.Column.Generated`;
the generator requires `read_only` and refuses a default, identity or similar on
the same field; `--from-db` writes the field with `read_only` and the expression.
Migrate already rendered `GENERATED ALWAYS AS (...) STORED` on PostgreSQL, MySQL
and SQLite. A VIRTUAL column is not stored in the schema model (PostgreSQL
inspection refuses it, MySQL and SQLite inspection read it as stored), so there is
no VIRTUAL case left in `--from-db`; this is a decision for the integrator.

## Acceptance criteria

- [ ] a regression test pins the fix: red before the change, green after
- [ ] `--from-db` emits a stored generated column as a `read_only` field with
      `generated:<expr>`, or `c.Generated` when the expression has a comma, with no
      warning, and the snapshot carries the expression
- [ ] a table with `normalized text GENERATED ALWAYS AS (lower(name)) STORED`
      round-trips through `--from-db` with an empty `migrate diff` (PostgreSQL
      integration test)
- [ ] `Insert.Model` and `Update.Model` do not write the column, and a typed select
      filters by it, on PostgreSQL, MySQL and SQLite (integration test)
- [ ] the generator refuses a generated field that is not `read_only` or that has
      a default
- [ ] docs: `docs/ai-guide.md`, `docs/migrations.md`, `docs/upgrading.md` and
      `docs/reference.md` describe the tag and `Config.Generated`

## Evidence

<!-- Filled at close time. -->
