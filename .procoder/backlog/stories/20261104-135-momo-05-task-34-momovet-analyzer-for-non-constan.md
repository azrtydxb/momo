# momo-05 Task 34: momovet analyzer for non-constant raw SQL (S-44)

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 34: momovet analyzer for non-constant raw SQL (S-44) (`.procoder/plans/momo-05-writes-queries.md:2828`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-44. Files: `momovet/analyzer.go`, `momovet/analyzer_test.go`, `momovet/testdata/src/raw/raw.go`, `cmd/momovet/main.go`.

## Acceptance criteria

- [x] `TestMomovet` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-44] `TestMomovet` (analysistest fixtures) reports non-constant SQL arguments and accepts constants; fails if a concatenated string passes.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: run 42455587059 on the throwaway branch worktree-agent-aecd38e0a7c8adf10-red (tests and fixtures only, branch deleted): vet reported momovet/analyzer_test.go undefined Analyzer.
- Green: green run 42455574669 on a5e569e0712fc7d785b63df4afac3ecfac87054e (ci-agent: lint including tidy-check, unit, integration postgres and fuzz-smoke all success).
- TestMomovet runs analysistest over testdata/src/raw with a stub momo package under testdata: constants and constant concatenation pass; concatenation, a var and a format call are reported for Exec, Scalars, SQL, TypedSQL and RawSubquery; a same-named local function is not flagged. cmd/momovet uses singlechecker; tidy-check passed with no go.mod change; docs/momovet.md added.
- Deviation: the plan names momo.Raw and momo.Subquery, which do not exist; the real raw-SQL functions are checked instead (Scalars, RawSubquery).
