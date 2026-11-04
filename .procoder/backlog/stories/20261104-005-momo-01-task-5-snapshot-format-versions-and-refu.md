# momo-01 Task 5: Snapshot format versions and refusal of newer formats

Status: done 2026-11-04
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 5: Snapshot format versions and refusal of newer formats (`.procoder/plans/momo-01-foundation.md:503`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-22. Files: `schema/load.go`, `schema/load_test.go`.

## Acceptance criteria

- [x] `TestFormatVersions` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-22, part] `TestFormatVersions` passes for the part of S-22 this task delivers; the full criterion is verified in story 099.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Criterion 1: `go test ./schema/ -run '^TestFormatVersions$' -count=1` before implementation failed with `undefined: Load`; after, `go test ./schema/ -count=1` printed `ok  github.com/azrtydxb/momo/schema`.
- Criterion 2 (S-22 snapshot part): `go test -count=1 -run TestFormatVersions -v ./schema` → `--- PASS: TestFormatVersions`, `ok`. the `TestFormatVersions` snapshot part passes (current format read, format_version 2 gives ErrNewerFormat naming `format_version 2`, missing field gives ErrMissingFormatVersion); `TestReleaseConfig` is written by plan Task 21 and the checksum-file and history-table parts by their own plans, so the criterion stays unchecked. The checksum-file, history-table and release-config parts are verified in story 099.
- Criterion 3: `launcher.sh check` printed `procoder gate: 2 clean, 0 unformatted, 0 unchecked, 0 out of scope, 11 hygiene finding(s) (0 blocking)`; commit 5de8c39 made.
