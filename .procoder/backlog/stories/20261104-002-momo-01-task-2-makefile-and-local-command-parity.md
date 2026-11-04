# momo-01 Task 2: Makefile and local command parity

Status: open
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 2: Makefile and local command parity (`.procoder/plans/momo-01-foundation.md:183`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-20, S-25. Files: `Makefile`, `internal/repocheck/makefile_test.go`, `CONTRIBUTING.md`.

## Acceptance criteria

- [ ] `TestMakefileTargets` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-20] `TestCIWorkflowCoverage` parses .github/workflows/ci.yml and asserts it runs unit, generator, compile-fail, race and fuzz-smoke tests, plus integration tests against pgvector/pgvector:pg17 and SQLite+sqlite-vec, on Go 1.26 and 1.27; fails if any job is skipped.
- [ ] [S-25] `TestWorkflowsPinned` parses .github/workflows and asserts every third-party `uses:` is pinned by a 40-character SHA and every workflow declares `permissions:`; CI runs govulncheck and the Scorecard workflow; fails if an action is referenced by tag.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

