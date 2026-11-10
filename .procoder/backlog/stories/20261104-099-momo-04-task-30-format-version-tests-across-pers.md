# momo-04 Task 30: Format-version tests across persisted artifacts

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-04-migrations Task 30: Format-version tests across persisted artifacts (`.procoder/plans/momo-04-migrations.md:3134`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-22. Files: `migrate/testdata/format0/20260101000000_init.up.sql`.

## Acceptance criteria

- [ ] `TestFormatVersions` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestHistoryFormat0Upgrade` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-22] `TestReleaseConfig` parses release-please-config.json and asserts the pre-1.0 minor-bump setting and the changelog path; `TestFormatVersions` asserts that a snapshot, checksum file and history table from format N-1 are read by the current version and that a future format version fails with an error naming the required Momo version; fails if a persisted format lacks its version field.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

### Sprint 7 (2026-11-10)

Carry reason: the plan's "format 0" does not exist in the product. The first release already wrote format 1 and the shipped design refuses a snapshot with no `format_version` on purpose (`ErrMissingFormatVersion`, schema/load.go:15; migrate/snapshot.go:60) and a `momo.sum` with no header (migrate/sum.go:117), so there is no N-1 artifact to read and no `migrate/testdata/format0` fixture to write. The refusal half of S-22 is covered: `TestFormatVersions` (schema/load_test.go), `TestHistoryFormatVersion` (migrate/history_test.go, newer history refused with "upgrade momo"), `TestHistoryFormatCheckedWithoutWrites`, `TestReleaseConfig` (internal/repocheck/release_test.go). Criteria stay unticked until a format 2 exists to make a format 1 fixture meaningful.
