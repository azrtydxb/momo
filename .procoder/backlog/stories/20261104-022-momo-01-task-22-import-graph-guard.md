# momo-01 Task 22: Import-graph guard

Status: open
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 22: Import-graph guard (`.procoder/plans/momo-01-foundation.md:2183`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-20. Files: `internal/repocheck/importgraph_test.go`.

## Acceptance criteria

- [ ] `TestImportGraph` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-20] `TestCIWorkflowCoverage` parses .github/workflows/ci.yml and asserts it runs unit, generator, compile-fail, race and fuzz-smoke tests, plus integration tests against pgvector/pgvector:pg17 and SQLite+sqlite-vec, on Go 1.26 and 1.27; fails if any job is skipped.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

