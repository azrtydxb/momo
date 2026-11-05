# momo-01 Task 21: Release automation and PR title check

Status: done 2026-11-04
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 21: Release automation and PR title check (`.procoder/plans/momo-01-foundation.md:2096`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-22. Files: `release-please-config.json`, `.github/workflows/release-please.yml`, `internal/repocheck/release_test.go`.

## Acceptance criteria

- [x] `TestReleaseConfig` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-22, part] `TestReleaseConfig` passes for the part of S-22 this task delivers; the full criterion is verified in story 099.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- TestReleaseConfig red: with release-please-config.json absent → `--- FAIL: TestReleaseConfig` / `open .../release-please-config.json: no such file or directory`; restored → `--- PASS: TestReleaseConfig`, `ok github.com/azrtydxb/momo/internal/repocheck`.
- S-22 part: TestReleaseConfig asserts the release-please config and manifest this task delivers (passes, above); the full S-22 criterion is verified in story 099.
- Gate and commits: the commit hook's procoder gate passed for 3ca6fbd (workflows and config, actionlint clean) and c1061bb (test); `go test -race -count=1 ./...` all ok after merge.
- Actions pinned via `gh api`: googleapis/release-please-action v5.0.0 → 45996ed1f6d02564a971a2fa1b5860e934307cf7; amannn/action-semantic-pull-request v6.1.1 → 48f256284bd46cdaab1048c3721360e808335d50.
