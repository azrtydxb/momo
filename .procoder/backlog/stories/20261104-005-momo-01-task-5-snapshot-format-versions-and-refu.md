# momo-01 Task 5: Snapshot format versions and refusal of newer formats

Status: open
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 5: Snapshot format versions and refusal of newer formats (`.procoder/plans/momo-01-foundation.md:503`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-22. Files: `schema/load.go`, `schema/load_test.go`.

## Acceptance criteria

- [x] `TestFormatVersions` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-22] `TestReleaseConfig` parses release-please-config.json and asserts the pre-1.0 minor-bump setting and the changelog path; `TestFormatVersions` asserts that a snapshot, checksum file and history table from format N-1 are read by the current version and that a future format version fails with an error naming the required Momo version; fails if a persisted format lacks its version field.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence
- Criterion 1: `go test ./schema/ -run '^TestFormatVersions$' -count=1` before implementation failed with `undefined: Load`; after, `go test ./schema/ -count=1` printed `ok  github.com/azrtydxb/momo/schema`.
- Criterion 2 (NOT verified, open): the `TestFormatVersions` snapshot part passes (current format read, format_version 2 gives ErrNewerFormat naming `format_version 2`, missing field gives ErrMissingFormatVersion); `TestReleaseConfig` is written by plan Task 21 and the checksum-file and history-table parts by their own plans, so the criterion stays unchecked.
- Criterion 3: `launcher.sh check` printed `procoder gate: 2 clean, 0 unformatted, 0 unchecked, 0 out of scope, 11 hygiene finding(s) (0 blocking)`; commit 5de8c39 made.
