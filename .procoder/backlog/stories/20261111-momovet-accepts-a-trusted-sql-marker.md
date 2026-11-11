# momovet accepts a trusted-SQL marker

Status: done 2026-11-11
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

- [x] a marked call (trailing comment, or comment on the line above) is not reported
- [x] an unmarked non-constant call is still reported
- [x] a directive without a reason is reported
- [x] a directive that suppresses nothing is reported as unused
- [x] docs/momovet.md and docs/upgrading.md describe the directive
- [x] a regression test pins the fix: red before the change, green after

## Evidence

- Red: ci-agent run 42977704719 (sha ed548082), unit job, TestMomovet: raw/raw.go:58, 61 and 69 "unexpected diagnostic: non-constant SQL string passed to momo.Exec/Query"; raw.go:68 "no diagnostic was reported matching `needs a reason`"; raw.go:72 "no diagnostic was reported matching `unused //momovet:ok directive`".
- Green: ci-agent run 42986311612, headSha f1fdd75d242a7d636865facf658d66abc69167a1, conclusion success. TestMomovet covers the marked (trailing and line above), unmarked, no-reason and unused cases.
- Docs changed: docs/momovet.md, docs/upgrading.md (Upgrading to 0.8.0).
