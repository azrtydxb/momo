# Conditional upsert follow-ups: SQLite target literals, Excluded in target

Status: open
Created: 2026-11-06
Epic: momo-05-writes-queries
Sprint: -

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
- [ ] `Excluded()` inside the target predicate is `ErrValidation` at build
      time; a unit test covers it.
- [ ] Golden tests cover nested `Not`/`And`/`Or`/`In`/`Between`/`IsNull` in the
      action predicate, a chunked `Models` upsert with both predicates (argument
      numbering), and that `base.Where(a)` and `base.Where(b)` do not alias.
- [ ] The README, `docs/capabilities.md` and `docs/ai-guide.md` drop the SQLite
      workaround once the first criterion is done.

## Evidence
