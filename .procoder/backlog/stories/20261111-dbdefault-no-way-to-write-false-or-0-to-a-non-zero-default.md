# dbdefault: no way to write false or 0 to a non-zero default (#90)

Status: open
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #90. A `dbdefault` field treats its Go zero value as "unset", so a
column such as `active boolean NOT NULL DEFAULT true` or `level int NOT NULL
DEFAULT 3` can never store an explicit `false` or `0` through `Insert.Model`:
the column is left out of the INSERT and the database default replaces the
value. Since #77 (`introspect.dbdefault: all`) tags every defaulted column, the
bug spreads: the default now applies, but an explicit false or 0 is silently
replaced.

Reproduce: model `Active bool` tagged `default:true,dbdefault`; insert
`Account{Active: false}`; the row stores true.

Expected: a way to tell "unset" from "false/0". Decision of the user
(2026-11-11): allow `dbdefault` on a pointer field of a NOT NULL column. nil
means "use the database default" (column omitted from INSERT, read back through
RETURNING); a non-nil value is always written, false and 0 included.

Root cause: the generator refused `dbdefault` on any pointer field (the field
was always treated as nullable, so nil had to mean NULL), leaving no way to
express "unset" other than the zero value. The runtime already treats a nil
pointer as unset and a pointer to a zero value as set, and copies the stored
value back into the pointer, so the change is in the generator: a `dbdefault`
pointer keeps the column NOT NULL and the descriptor non-nullable.

## Acceptance criteria

- [ ] a regression test pins the fix: red before the change, green after
- [ ] `dbdefault` on a pointer field of a NOT NULL column is accepted by the generator; the column stays NOT NULL and the descriptor flags carry no nullable bit; `sql.Null*` types and the `nullable` tag are still refused
- [ ] PostgreSQL integration: a nil pointer takes the database default and the stored value is read back into the pointer; an explicit false and an explicit 0 are written (also in a mixed `Models` batch)
- [ ] SQLite and MySQL: explicit false and 0 are written; a nil pointer is read back on SQLite where RETURNING exists, and refused with ErrUnsupported on MySQL (no RETURNING), which is documented
- [ ] `momo generate --from-db` with `introspect.dbdefault: all` gives a NOT NULL bool or number column with a non-zero constant default a pointer type; zero, text and computed defaults keep the plain type
- [ ] `momo generate` warns when a non-pointer dbdefault field is a bool or number whose default is a non-zero constant
- [ ] docs/ai-guide.md no longer says false always stores true without the pointer escape; docs/upgrading.md has a 0.8.0 section; docs/reference.md is regenerated from CI
- [ ] the full ci-agent run is green

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
