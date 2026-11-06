# Conditional upsert follow-ups: SQLite target literals, Excluded in target

Status: open
Created: 2026-11-06
Epic: momo-05-writes-queries
Sprint: 003-sprint-3-postgresql-hardening-migration-safety-bugs-query

## Description

Follow-up from the review of issue #8 (conditional upsert). A developer using
`OnConflict(cols...).Where(preds...)` on SQLite gets "ON CONFLICT clause does
not match any PRIMARY KEY or UNIQUE constraint", because SQLite matches the
target predicate against the index text and a bound value (`"live" = ?`) never
matches the literal in the index (`live = 1`). v0.2.0 documents the workaround
(`momo.SQL("live = 1")`). A developer who puts `col.Excluded()` in the target
predicate gets a database error instead of `ErrValidation`.

## Acceptance criteria

- [ ] On SQLite, the target predicate renders literal values (bool, integer,
      string) inline instead of binding them, and a SQLite integration test
      upserts against a partial unique index with a typed predicate.
- [x] `Excluded()` inside the target predicate is `ErrValidation` at build
      time; a unit test covers it.
- [x] Golden tests cover nested `Not`/`And`/`Or`/`In`/`Between`/`IsNull` in the
      action predicate, a chunked `Models` upsert with both predicates (argument
      numbering), and that `base.Where(a)` and `base.Where(b)` do not alias.
- [x] The README, `docs/capabilities.md` and `docs/ai-guide.md` drop the SQLite
      workaround once the first criterion is done.

## Evidence

Open: there is no SQLite driver in go.mod and no SQLite integration opener, so the first criterion's integration test cannot be written yet.

- Criterion 1 (open): `TestConflictTargetSQLiteLiterals` and `TestConflictActionStaysBoundOnSQLite` prove the rendering (bool 1/0, integer, string with quote doubling, In/Between/IsNull/Not; PostgreSQL still binds; VALUES and the action predicate stay bound). No SQLite integration opener exists and go.mod has no SQLite driver (adding one needs a dependency decision), so the integration test is not written and this criterion stays unticked.
- Criterion 2: `TestConflictTargetExcludedIsValidation` (postgres and sqlite, direct and nested) returns ErrValidation; mutation `if false` in `excludedExpr.AppendSQL` fails it.
- Criterion 3: `TestConflictActionWhereNestedGolden` (Not/And/Or/In/NotIn/Between/IsNull), `TestConflictWhereChunkedArgNumbering` (MaxParams 10: two chunks, args $9/$10 then $5/$6) and `TestConflictWhereDoesNotAlias` (mutating either `slices.Clip` fails it).
- Criterion 4: README, docs/capabilities.md and docs/ai-guide.md no longer mention `momo.SQL("live = 1")`; they describe the inline rendering and the Excluded rule.
