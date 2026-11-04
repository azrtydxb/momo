# momo-01 Task 21: Release automation and PR title check

Status: open
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 21: Release automation and PR title check (`.procoder/plans/momo-01-foundation.md:2096`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-22. Files: `release-please-config.json`, `.github/workflows/release-please.yml`, `internal/repocheck/release_test.go`.

## Acceptance criteria

- [ ] `TestReleaseConfig` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-22, part] `TestReleaseConfig` passes for the part of S-22 this task delivers; the full criterion is verified in story 099.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

