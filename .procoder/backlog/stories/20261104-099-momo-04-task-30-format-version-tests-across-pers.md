# momo-04 Task 30: Format-version tests across persisted artifacts

Status: done 2026-11-11
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Carried: 007-sprint-7-finish-every-open-backlog-story-auditing-each — The plan's format 0 never existed: the first release wrote format 1 and the shipped design refuses artifacts without format_version (ErrMissingFormatVersion, momo.sum header), so there is no N-1 artifact and no migrate/testdata/format0 fixture to read. Refusal half is covered (TestFormatVersions in schema/load_test.go, TestHistoryFormatVersion, TestReleaseConfig). Needs the owner to rewrite the criteria or wait for a format 2.

## Description

Implements plan momo-04-migrations Task 30: Format-version tests across persisted artifacts (`.procoder/plans/momo-04-migrations.md:3134`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-22. Files: `migrate/testdata/format0/20260101000000_init.up.sql`.

## Acceptance criteria

- [x] [S-22] `TestFormatVersions` (schema/load_test.go:13) as shipped: the current format loads, a `format_version` 2 snapshot is refused with `ErrNewerFormat` naming the format, and a snapshot without `format_version` is refused with `ErrMissingFormatVersion`.
- [x] [S-22] `TestHistoryFormatVersion` (migrate/history_test.go:17, PostgreSQL) replaces `TestHistoryFormat0Upgrade`: a history table of a newer format is refused with an "upgrade momo" error, and `TestHistoryFormatCheckedWithoutWrites` (migrate/apply_safety_test.go:206) shows the check writes nothing.
- [x] [S-22] `TestReleaseConfig` (internal/repocheck/release_test.go:15) parses release-please-config.json and asserts `bump-minor-pre-major` and the `CHANGELOG.md` changelog path; the persisted formats are covered by `TestFormatVersions` (snapshot), `TestHistoryFormatVersion` (history table) which refuse an artifact that lacks its format version or carries a newer one. The momo.sum header check in migrate/sum.go (missing header, `ErrNewerFormat`) exists but no test is cited for it here. There is no format N-1 to read: format 1 is the first format ever released.
- [x] `procoder check` gate is clean and the task's commit is made (the task's code is on main at 4781864; the lint job 130341061844 of ci.yml run 42965315635 is success; the commit gate ran on the sprint 9 bookkeeping commit)

## Evidence

### Sprint 7 (2026-11-10)

Carry reason: the plan's "format 0" does not exist in the product. The first release already wrote format 1 and the shipped design refuses a snapshot with no `format_version` on purpose (`ErrMissingFormatVersion`, schema/load.go:15; migrate/snapshot.go:60) and a `momo.sum` with no header (migrate/sum.go:117), so there is no N-1 artifact to read and no `migrate/testdata/format0` fixture to write. The refusal half of S-22 is covered: `TestFormatVersions` (schema/load_test.go), `TestHistoryFormatVersion` (migrate/history_test.go, newer history refused with "upgrade momo"), `TestHistoryFormatCheckedWithoutWrites`, `TestReleaseConfig` (internal/repocheck/release_test.go). Criteria stay unticked until a format 2 exists to make a format 1 fixture meaningful.

## Reworded 2026-11-11 (user decision)

User decision of 2026-11-11, recorded in .procoder/ask/decisions.md, section "Carried plan stories": reword the criteria to the design that shipped, cite CI evidence, and close the story. The `Carried:` lines above stay as the history.

- Original: `TestFormatVersions` exists as written in the plan, failed before the implementation, and passes now
- Original: `TestHistoryFormat0Upgrade` exists as written in the plan, failed before the implementation, and passes now
- Original: [S-22] ... `TestFormatVersions` asserts that a snapshot, checksum file and history table from format N-1 are read by the current version and that a future format version fails with an error naming the required Momo version; fails if a persisted format lacks its version field.
  - Reason: the plan's "format 0" never existed. The first release already wrote format 1, and the shipped design refuses an artifact without a version on purpose (`ErrMissingFormatVersion`, schema/load.go; a `momo.sum` without its header, migrate/sum.go), so there is no N-1 artifact to read and no `migrate/testdata/format0` fixture. What is verifiable is the refusal half: missing and newer formats are refused, with the error naming the format (snapshot) or telling the user to upgrade momo (history). A fixture test for N-1 becomes meaningful only when a format 2 exists.

### Sprint 9 CI evidence (2026-11-11)

Jobs of ci.yml run 42965315635 on main (headSha 47818647165f0231b1117bb6ebe2bce67dbed444, conclusion success). The job logs carry package-level `ok` lines, not `-v` output: unit (1.26) 130341061373 (`make test`), race (1.26) 130341061707 (`make test-race`), integration (1.27, 0.8.7-pg17, postgres) 130341061675 (`make test-integration-postgres`: MOMO_REQUIRE_INTEGRATION=1 and `-tags integration -race`, so a skipped database test fails), windows 130341061640, bench-module 130341061885 (`make test-bench-module`), lint 130341061844.

- Unit job 130341061373: `ok` momo/schema (0.046s) and momo/internal/repocheck (15.757s), which hold `TestFormatVersions` (no build tag) and `TestReleaseConfig`; `ok` momo/migrate (0.168s) as well.
- PostgreSQL integration job 130341061675: `ok` momo/migrate (27.600s) and momo/schema (1.211s) with MOMO_REQUIRE_INTEGRATION=1; `TestHistoryFormatVersion` is `//go:build integration`, so it only runs there.
- windows job 130341061640: `ok` momo/schema and momo/internal/repocheck.
