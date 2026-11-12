# NullColumn SetPtr and EqPtr for nil-able values

Status: done 2026-11-12
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #109. A `NullColumn[T]` takes `T` in `Set` and `Eq`, but models
carry nullable columns as `*T`. Every call site that may hold nil must branch:
`if p.X != nil { Set(*p.X) } else { SetNull() }`, and the same for filters
(`Eq(*f.X)` guarded by a nil check). Expected: `SetPtr(v *T)` writes NULL for
nil and the value otherwise, and `EqPtr(v *T)` renders `IS NULL` for nil and
`= $n` otherwise. Root cause: the API had no pointer-taking variants, so the
only nil-safe route was a hand-written branch (or `Insert.Model`).

## Acceptance criteria

<!-- Each criterion is testable. The first is non-negotiable — a bug is
     only fixed when a test would catch its return. -->

- [x] a regression test pins the fix: red before the change, green after
- [x] `SetPtr(nil)` renders NULL and `SetPtr(&v)` binds v, on PostgreSQL, SQLite and MySQL
- [x] `EqPtr(nil)` renders `IS NULL` (never `= NULL`) and `EqPtr(&v)` binds v
- [x] a PostgreSQL integration test round-trips insert, update and filter with nil and non-nil values
- [x] docs/ai-guide.md and docs/upgrading.md (0.8.0) describe the new methods

## Evidence

- Red: ci-agent run 42993189784 on 1b85f6ba (test-only commit) failed: `nullcolumn_ptr_test.go:21:65: suite.DocBody.EqPtr undefined` and `nullcolumn_ptr_integration_test.go:25:79: suite.DocBody.SetPtr undefined`.
- Green: ci-agent run 43007735238 and docs run 43007735132, both success on SHA 61ecb6c52bdb7042d3f9c9a675987db6ee1196fa (TestNullColumnPtrSQL for postgres, sqlite and mysql; TestNullColumnPtrRoundTrip in the postgres integration job).
- Docs changed: docs/ai-guide.md, docs/upgrading.md (0.8.0), docs/reference.md (regenerated from the regenerated-docs artifact of run 43004147929, whose tree matched).
