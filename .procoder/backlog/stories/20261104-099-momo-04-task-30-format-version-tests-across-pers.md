# momo-04 Task 30: Format-version tests across persisted artifacts

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 30: Format-version tests across persisted artifacts (`.procoder/plans/momo-04-migrations.md:3134`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-22. Files: `migrate/testdata/format0/20260101000000_init.up.sql`.

## Acceptance criteria

- [ ] `TestFormatVersions` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestHistoryFormat0Upgrade` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-22] `TestReleaseConfig` parses release-please-config.json and asserts the pre-1.0 minor-bump setting and the changelog path; `TestFormatVersions` asserts that a snapshot, checksum file and history table from format N-1 are read by the current version and that a future format version fails with an error naming the required Momo version; fails if a persisted format lacks its version field.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

