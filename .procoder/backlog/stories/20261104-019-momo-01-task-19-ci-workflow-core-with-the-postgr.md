# momo-01 Task 19: CI workflow core with the PostgreSQL integration job

Status: done 2026-11-04
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 19: CI workflow core with the PostgreSQL integration job (`.procoder/plans/momo-01-foundation.md:1805`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-20, S-25, S-26. Files: `.github/workflows/ci.yml`, `internal/repocheck/ci_test.go`, `go.mod`. Contributes to S-25, S-26; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestCIWorkflowCoverage` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-20] `TestCIWorkflowCoverage` parses .github/workflows/ci.yml and asserts it runs unit, generator, compile-fail, race and fuzz-smoke tests, plus integration tests against pgvector/pgvector:pg17 and SQLite+sqlite-vec, on Go 1.26 and 1.27; fails if any job is skipped.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `TestCIWorkflowCoverage`: `go test ./internal/repocheck/ -run '^TestCIWorkflowCoverage$' -count=1` before ci.yml existed: `FAIL ... ci_test.go:86: open .../.github/workflows/ci.yml: no such file or directory`; after: `ok  github.com/azrtydxb/momo/internal/repocheck`.
- [S-20]: the test asserts jobs unit, compile-fail, race, fuzz-smoke and integration have no `if` or continue-on-error and test Go 1.26 and 1.27, the pgvector service and pg17/pg16 matrix, and (via `make -n test-integration-postgres`) `-tags integration`, `-race` and `MOMO_REQUIRE_INTEGRATION=1`; it passes. SQLite integration is not enabled because M1 lists postgres only (plan milestone rule; SQLite is added in M2).
- `actionlint` (installed): no output, exit 0.
- Gate: `go build ./... && go vet ./... && go test -race ./...` all `ok`; `launcher.sh check`: `procoder gate: 1 clean, 0 unformatted, 0 unchecked, 3 out of scope, 48 hygiene finding(s) (0 blocking)`; commit 20fdf0e.

