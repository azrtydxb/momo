# Several FROM sources and source RETURNING columns on UPDATE and DELETE

Status: open
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #93. `UpdateQuery.From` and `DeleteQuery.Using` took exactly one
source, and `Returning` listed only the target model's columns, rendered
unqualified. Claim-and-return queues (`WITH due AS (... FOR UPDATE SKIP LOCKED)
UPDATE t SET ... FROM due, subscription s WHERE ... RETURNING t.id, s.url`)
could not be written and stayed raw SQL.

Reproduce: build an update with a CTE named due and a second table as sources,
`From(momo.CTE("due"), otherTable)`, and `Returning` the target id and the
other table's url. Observed: `From` takes one
argument, so the second source does not compile; a source column in `Returning`
was matched by name against the target's columns (a wrong column when the names
match, `ErrValidation` otherwise); RETURNING was unqualified, so a column that the
source and the target share was ambiguous on PostgreSQL. Expected: several sources
render as a comma list, `Returning` reads source and CTE columns qualified and the
rows scan into a projection or plain struct, and the unfiltered-mutation guard
keeps refusing a source that nothing constrains.

The issue also notes that a data-modifying CTE body without `Returning` was
accepted although `docs/capabilities.md` requires `Returning`: `writeWithClause`
only checked the dialect. It now fails the build with `ErrValidation`.

Root cause: the builders stored one `TableSource` and `renderReturning` /
`returningIndexes` knew only the target's `Meta` columns.

## Acceptance criteria

- [ ] a regression test pins the fix: red before the change, green after
- [ ] `From` and `Using` take several sources and render them as a comma list on PostgreSQL, SQLite (`From`) and MySQL (multi-table form)
- [ ] `Returning` accepts source and CTE columns, rendered qualified on PostgreSQL, and `ScanUpdate` / `ScanDelete` scan them into a plain struct
- [ ] with two or more sources the guard refuses a source that no `Where` predicate references (`ErrUnfilteredMutation`) and `AllRows` still confirms it
- [ ] SQLite and MySQL refuse a source column in RETURNING with `ErrUnsupported`; `Rows` refuses it with `ErrValidation`
- [ ] a DML CTE body without `Returning` fails the build with `ErrValidation`; with `Returning` or `ReturningAll` it builds
- [ ] docs updated: `docs/ai-guide.md`, `docs/capabilities.md`, `docs/upgrading.md`

## Evidence

(filled when CI is green)
