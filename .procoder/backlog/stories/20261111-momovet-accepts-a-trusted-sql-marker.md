# momovet accepts a trusted-SQL marker

Status: open
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #106. Reproduce: build a SQL string from constants and an allowlisted identifier (`"UPDATE "+table+" SET x = 1"` with `table` a variable) and pass it to `momo.Exec`, then run `go vet -vettool=momovet ./...`.

Observed: momovet reports `non-constant SQL string passed to momo.Exec` and offers no way to mark the call as vetted, so a project with about 22 such call sites cannot turn momovet on in CI without suppressing whole packages.

Expected: a `//momovet:ok <reason>` line directive, as a trailing comment on the call or on the line above, suppresses the report for that call; a directive without a reason, or one that suppresses nothing, is itself reported.

Root cause: the analyzer had no suppression mechanism; every non-constant argument was reported unconditionally.

## Acceptance criteria

- [ ] a marked call (trailing comment, or comment on the line above) is not reported
- [ ] an unmarked non-constant call is still reported
- [ ] a directive without a reason is reported
- [ ] a directive that suppresses nothing is reported as unused
- [ ] docs/momovet.md and docs/upgrading.md describe the directive
- [ ] a regression test pins the fix: red before the change, green after

## Evidence

<!-- Filled at close time. -->
