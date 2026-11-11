# fromdb models stored generated columns as read-only fields

Status: done 2026-11-11
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

- [x] a regression test pins the fix: red before the change, green after
- [x] `--from-db` emits a stored generated column as a `read_only` field with
      `generated:<expr>`, or `c.Generated` when the expression has a comma, with no
      warning, and the snapshot carries the expression
- [x] a table with `normalized text GENERATED ALWAYS AS (lower(name)) STORED`
      round-trips through `--from-db` with an empty `migrate diff` (PostgreSQL
      integration test)
- [x] `Insert.Model` and `Update.Model` do not write the column, and a typed select
      filters by it, on PostgreSQL, MySQL and SQLite (integration test)
- [x] the generator refuses a generated field that is not `read_only` or that has
      a default
- [x] docs: `docs/ai-guide.md`, `docs/migrations.md`, `docs/upgrading.md` and
      `docs/reference.md` describe the tag and `Config.Generated`

## Evidence

- Red: ci-agent run 42978208418 (sha 727ec1c, tests only). Failing lines:
  `TestTagGrammar: "generated:lower(name)" rejected: unknown tag option "generated"`,
  `TestGenerateFromDBStoredGenerated: unexpected warning: users.normalized: generated column is not modeled`,
  `TestGeneratedTagReachesColumn: column = {... Generated: ...}, want the generated expression`.
- Red (review round, IDGen and Serial refusals): ci-agent run 42993215775 (sha 0ed2818); `TestGeneratedTagRefusals/serial`
  and `/identity` failed on the old combined message that named neither conflict.
- Green (final): ci-agent run 43001013480, headSha de1def7f9cedc2f4f15e90e67fc3a4f9ddfeef68, all 9 jobs success,
  including integration-mysql 84, 97 and mariadb running `TestStoredGeneratedColumnLive` (Insert, Upsert, chunked
  insert, Update; plus Unnest and Copy on PostgreSQL).
- Earlier green: ci-agent run 42988137043, headSha bf70aea8ef97db46bc764228c1d17d32a6e023dc, all 9 jobs success
  (unit; integration postgres, which runs the `--from-db` round trip with an empty `migrate diff` after baseline and
  `TestStoredGeneratedColumnLive`; integration-mysql 84, 97 and mariadb, which run `TestStoredGeneratedColumnLive`;
  lint; apidiff-docs; fuzz-smoke; bench-module).
- Docs changed: docs/ai-guide.md, docs/migrations.md, docs/upgrading.md (Upgrading to 0.8.0), docs/reference.md
  (from the regenerated-docs artifact of run 42985005399), cli/generate.go help text.
