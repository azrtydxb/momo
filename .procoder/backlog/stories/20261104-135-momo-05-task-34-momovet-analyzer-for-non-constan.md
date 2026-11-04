# momo-05 Task 34: momovet analyzer for non-constant raw SQL (S-44)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 34: momovet analyzer for non-constant raw SQL (S-44) (`.procoder/plans/momo-05-writes-queries.md:2828`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-44. Files: `momovet/analyzer.go`, `momovet/analyzer_test.go`, `momovet/testdata/src/raw/raw.go`, `cmd/momovet/main.go`.

## Acceptance criteria

- [ ] `TestMomovet` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-44] `TestMomovet` (analysistest fixtures) reports non-constant SQL arguments and accepts constants; fails if a concatenated string passes.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

