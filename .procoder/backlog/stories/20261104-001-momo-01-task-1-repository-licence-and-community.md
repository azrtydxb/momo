# momo-01 Task 1: Repository, licence and community files

Status: done 2026-11-04
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 1: Repository, licence and community files (`.procoder/plans/momo-01-foundation.md:65`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-24. Files: `internal/repocheck/repo_test.go`, `LICENSE`, `.github/ISSUE_TEMPLATE/bug_report.yml`, `.gitignore`, `*.go`.

## Acceptance criteria

- [x] `TestRepoFiles` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-24] `TestRepoFiles` asserts LICENSE (Apache-2.0 text), SECURITY.md, CONTRIBUTING.md, CODE_OF_CONDUCT.md and .github issue/PR templates exist and SPDX headers are present in Go files; fails if a file is missing.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `TestRepoFiles` as written in the plan: before implementation `go test ./internal/repocheck/ -run '^TestRepoFiles$' -count=1` printed `--- FAIL: TestRepoFiles` with `missing LICENSE: ... no such file or directory`; after implementation the same command printed `ok  	github.com/azrtydxb/momo/internal/repocheck	0.272s`.
- [S-24] Test asserts the files and headers: it checks LICENSE, SECURITY.md, CONTRIBUTING.md, CODE_OF_CONDUCT.md and the four .github templates exist, and LICENSE contains "Version 2.0, January 2004" (`grep -c "Version 2.0, January 2004" LICENSE` printed `1`, 202 lines); before the SPDX step the test failed with `builder.go: missing SPDX header` (and 8 more Go files), so a missing file or header fails it.
- Gate: `launcher.sh check` printed `procoder gate: 40 clean, 0 unformatted, 0 unchecked, 5 out of scope, 137 hygiene finding(s) (0 blocking)`; commit 586d784 `chore: add licence, community files and SPDX headers`.
