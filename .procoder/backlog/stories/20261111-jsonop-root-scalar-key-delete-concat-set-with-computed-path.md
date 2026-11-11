# jsonop: root scalar, key delete, concat, set with computed path, array constructors

Status: open
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #96. Owl's audit snapshots, settings reads and retention queries
need jsonb operations momo cannot express, so they fall back to raw SQL.

Reproduce: try to build `value #>> '{}'`, `to_jsonb(x) - 'password_hash'`,
`release || jsonb_build_object(...)`, `jsonb_set(skipped, ARRAY['at_' || $1::text], ...)`
or `ARRAY(SELECT ... UNION SELECT ...)` with momo. `jsonop.Path` with no keys
fails with "JSON path has no elements" (ErrValidation), there is no `-`, `||`
or computed-path `jsonb_set` in jsonop, and no `ARRAY[...]` or `ARRAY(subquery)`
expression in momo.

Observed: none of these can be written as a typed expression; the caller drops
to raw SQL, which loses the sensitive-flag masking and, for a key delete,
forces a column list that silently picks up a future secret column.

Expected: `jsonop.Root`, `jsonop.DeleteKey`, `jsonop.Concat`, `jsonop.Set`,
`momo.ArrayOf` and `momo.ArraySubquery` render the PostgreSQL operators, with
MySQL and SQLite renderings where an equivalent exists and ErrUnsupported
where none does.

Root cause: the jsonop package only had extraction, containment and key tests,
and the root package had no array-constructor expression. Root extraction was
blocked by the non-empty-path check, which is correct for Path.

## Acceptance criteria

- [ ] `jsonop.Root(col)` renders `#>> '{}'` (PostgreSQL), `json_extract(col, '$')` (SQLite) and `JSON_UNQUOTE(JSON_EXTRACT(col, '$'))` (MySQL); `Path` with no keys stays ErrValidation
- [ ] `jsonop.DeleteKey` renders `-` / `json_remove` / `JSON_REMOVE` with the key bound, and keeps every other member when run against PostgreSQL and SQLite
- [ ] `jsonop.Concat` and `jsonop.Set` run on PostgreSQL and return ErrUnsupported on SQLite and MySQL
- [ ] `momo.ArrayOf` and `momo.ArraySubquery` run on PostgreSQL (including a UNION subquery), refuse an empty `ArrayOf`, and return ErrUnsupported elsewhere
- [ ] values bound through the new operators from a sensitive JSON column are masked in hook events
- [ ] reader docs (ai-guide, capabilities, TYPE_MAPPING, upgrading 0.8.0) and docs/reference.md describe the new API
- [ ] a regression test pins the fix: red before the change, green after

## Evidence

<!-- Filled at close time. -->
