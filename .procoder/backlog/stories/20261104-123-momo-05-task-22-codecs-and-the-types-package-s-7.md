# momo-05 Task 22: Codecs and the types package (S-73)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 22: Codecs and the types package (S-73) (`.procoder/plans/momo-05-writes-queries.md:1803`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-73. Files: `types/codec.go`, `types/json.go`, `types/date.go`, `types/uuid.go`, `types/hstore.go`, `types/codec_test.go`.

## Acceptance criteria

- [ ] `TestCodecsRoundTrip` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-73] `TestCodecsRoundTrip` round-trips each type on every dialect; fails if a codec's SQL type and encoding disagree.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

