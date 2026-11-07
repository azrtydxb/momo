# momo-09 Task 7: Relation loading is reported to hooks

Status: open
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: -

## Description

Implements plan momo-09-observability-testing Task 7: Relation loading is reported to hooks (`.procoder/plans/momo-09-observability-testing.md:1231`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-18. Files: `obs_relevents_test.go`, `db.go`. Contributes to S-18; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestRelationEventsReported` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `TestRelationEventsReported` passes on first run because momo-06 emits the events (plan says so, so no red-first); mutation run 42411352506 (2b503e3, reverted) failed with `want 2 relation events, got 0` after relation hooks were no longer registered; green run 42410680387.
