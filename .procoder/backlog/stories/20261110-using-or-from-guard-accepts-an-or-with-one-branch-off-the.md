# Using or From guard accepts an Or with one branch off the target

Status: done 2026-11-10
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

- [x] a regression test pins the fix: red before the change, green after
- [x] Delete with Using and Update with From both refuse an Or whose branches do not all reference the target, unless AllRows is set (unit tests)
- [x] an And with one target term, and an Or whose branches all reference the target, are still accepted (unit tests)
- [x] the method docs and `docs/ai-guide.md` state the Or rule

## Evidence

- Red: ci-agent run 42827421622 (throwaway branch worktree-agent-acf2b-red-orguard, sha 2ab5519, the test alone), unit job FAIL TestFromUsingGuardOr: an Or with one branch off the target returned no error on postgres, mysql and sqlite for Using and From.
- Fix 5b76911: `referencesTable` (write_with.go) now recurses: an And is satisfied by any term, an Or only when every branch constrains the target, an empty Or never. The same test also covers the accepted cases (an And with one target term, an Or whose branches all reference the target, an Or of Ands, a target term beside a mixed Or) and AllRows. Method docs on DeleteQuery.Using and UpdateQuery.From and docs/ai-guide.md state the rule.
- Green: ci-agent run 42839158680 (sha 5e86d37, all 9 jobs success), TestFromUsingGuardOr passes.
