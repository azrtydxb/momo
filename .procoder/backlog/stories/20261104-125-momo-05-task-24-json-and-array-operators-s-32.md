# momo-05 Task 24: JSON and array operators (S-32)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-05-writes-queries Task 24: JSON and array operators (S-32) (`.procoder/plans/momo-05-writes-queries.md:1997`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-32. Files: `json_array.go`, `json_array_test.go`.

## Scope note

The array half shipped for issue #31 and is closed as its own story, `20261108-array-columns-and-operators-issue-31.md`. What remains here is the JSON operators (`JSONGet`, `JSONText`, `JSONContains`, `JSONHasKey`), tracked in issue #38. The criteria below stay open until the JSON half lands.

## Acceptance criteria

- [ ] `TestJSONAndArrayOperators` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-32] `TestJSONAndArrayOperators` asserts each operator's SQL and result rows on PostgreSQL and the SQLite equivalent or ErrUnsupported; fails if a value is interpolated rather than bound.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

### Sprint 7 audit

- Criterion 2: jsonop/jsonop_test.go TestJSONOperatorsGolden asserts SQL and bound args for PostgreSQL, SQLite and MySQL (values bound, not interpolated); TestJSONOperatorsAgainstDatabase (jsonop_integration_test.go) asserts rows on PostgreSQL and SQLite; Contains is ErrUnsupported on SQLite; arrayop/arrayop_test.go TestArrayOperatorsGolden and TestArrayOperatorsUnsupportedOffPostgres plus TestArrayOperatorsLive cover arrays. Run 42769037273 (ci-agent, headSha 4d04a3d, all jobs green).
- Criterion 1 stays unticked. Carry reason: TestJSONAndArrayOperators was superseded by the jsonop and arrayop packages and the tests above (jsonop/jsonop.go, arrayop/arrayop.go); no test of that name will exist.
