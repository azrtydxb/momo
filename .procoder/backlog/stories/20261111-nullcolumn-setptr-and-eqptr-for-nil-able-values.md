# NullColumn SetPtr and EqPtr for nil-able values

Status: open
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

- [ ] a regression test pins the fix: red before the change, green after
- [ ] `SetPtr(nil)` renders NULL and `SetPtr(&v)` binds v, on PostgreSQL, SQLite and MySQL
- [ ] `EqPtr(nil)` renders `IS NULL` (never `= NULL`) and `EqPtr(&v)` binds v
- [ ] a PostgreSQL integration test round-trips insert, update and filter with nil and non-nil values
- [ ] docs/ai-guide.md and docs/upgrading.md (0.8.0) describe the new methods

## Evidence

<!-- Filled at close time -->
