# momo-01 Task 2: Makefile and local command parity

Status: done 2026-11-04
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 2: Makefile and local command parity (`.procoder/plans/momo-01-foundation.md:183`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-20, S-25. Files: `Makefile`, `internal/repocheck/makefile_test.go`, `CONTRIBUTING.md`. Contributes to S-20, S-25; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestMakefileTargets` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- S-20 and S-25 criteria (TestCIWorkflowCoverage, TestWorkflowsPinned) moved to stories 019 and 020, where the plan defines those tests.
- AC1: `go test ./internal/repocheck/ -run '^TestMakefileTargets$' -count=1` before the Makefile: `--- FAIL: TestMakefileTargets` / `make -n test: exit status 2` / `make: *** No rule to make target 'test'.  Stop.`; after: `ok  	github.com/azrtydxb/momo/internal/repocheck	0.370s`; `make fmt-check vet` exits 0.
- AC4: `launcher.sh check` -> `procoder gate: 2 clean, 0 unformatted, 0 unchecked, 1 out of scope, 11 hygiene finding(s) (0 blocking)`; commit d4863a8 `build: add Makefile with CI-parity targets`.
