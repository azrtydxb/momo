# momo-01 Task 22: Import-graph guard

Status: open
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 22: Import-graph guard (`.procoder/plans/momo-01-foundation.md:2183`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-20. Files: `internal/repocheck/importgraph_test.go`. Contributes to S-20; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestImportGraph` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Commit 062a737 "test: guard the core import graph" adds internal/repocheck/importgraph_test.go.
- Criterion 1 is left unchecked: the plan (Task 22 Step 2) says the test passes on first run because no production change exists, so a genuine red-before-implementation run is impossible. `go test ./internal/repocheck/ -run '^TestImportGraph$' -count=1 -v` printed `--- PASS: TestImportGraph` and `ok`.
- Mutation proving the guard bites: adding schema/zz_mutation.go importing github.com/azrtydxb/momo/migrate gave `FAIL: github.com/azrtydxb/momo/schema depends on github.com/azrtydxb/momo/migrate, which the core may not import`; the file was removed afterwards and the tree was clean.
- `go build ./... && go vet ./... && go test -race ./...` all ok.
- `launcher.sh check`: `procoder gate: 1 clean, 0 unformatted, 0 unchecked, 0 out of scope, 7 hygiene finding(s) (0 blocking)`.
