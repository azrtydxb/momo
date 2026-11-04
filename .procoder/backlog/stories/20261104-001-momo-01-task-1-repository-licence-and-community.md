# momo-01 Task 1: Repository, licence and community files

Status: open
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: -

## Description

Implements plan momo-01-foundation Task 1: Repository, licence and community files (`.procoder/plans/momo-01-foundation.md:65`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-24. Files: `internal/repocheck/repo_test.go`, `LICENSE`, `.github/ISSUE_TEMPLATE/bug_report.yml`, `.gitignore`, `*.go`.

## Acceptance criteria

- [ ] `TestRepoFiles` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-24] `TestRepoFiles` asserts LICENSE (Apache-2.0 text), SECURITY.md, CONTRIBUTING.md, CODE_OF_CONDUCT.md and .github issue/PR templates exist and SPDX headers are present in Go files; fails if a file is missing.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

