# Using or From guard accepts an Or with one branch off the target

Status: open
Created: 2026-11-10
Epic: -
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each
Type: bug
Severity: s3

## Description

Found while reviewing the #64 fix (merge 1a3a95e). `DeleteQuery.Using` and
`UpdateQuery.From` refuse a statement whose Where never references the target
table, because the result is a cross join that touches every target row
(`referencesTable` in delete.go and update.go). The check passes when any
predicate references the target, so an `Or` with only one branch on the target
slips through.

Reproduction: a delete of `Doc` rows with `.Using(src).Where(momo.Or(doc.ID.Eq(1), src.Title.Eq("x")))`
renders `DELETE FROM docs USING src WHERE docs.id = $1 OR src.title = $2`.

Observed: the statement runs. When one `src` row has title "x", every `docs`
row is deleted.

Expected: `ErrUnfilteredMutation` unless `AllRows()` is set, because one branch
of the Or does not constrain the target. The rule should require every branch
of a top-level Or to reference the target; an And is satisfied when any of its
terms does.

## Acceptance criteria

- [ ] a regression test pins the fix: red before the change, green after
- [ ] Delete with Using and Update with From both refuse an Or whose branches do not all reference the target, unless AllRows is set (unit tests)
- [ ] an And with one target term, and an Or whose branches all reference the target, are still accepted (unit tests)
- [ ] the method docs and `docs/ai-guide.md` state the Or rule

## Evidence
