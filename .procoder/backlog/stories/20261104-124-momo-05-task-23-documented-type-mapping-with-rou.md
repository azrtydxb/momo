# momo-05 Task 23: Documented type mapping with round-trip test per mapping (S-31)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 23: Documented type mapping with round-trip test per mapping (S-31) (`.procoder/plans/momo-05-writes-queries.md:1896`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-31. Files: `docs/TYPE_MAPPING.md`, `type_mapping_test.go`.

## Acceptance criteria

- [ ] `TestTypeMapping` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-31] `TestTypeMapping` round-trips one value (including edge values: max int64, NaN where legal, pre-1970 time, non-UTC zone) per mapping in the documented table on both dialects; fails if a documented mapping is untested.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

