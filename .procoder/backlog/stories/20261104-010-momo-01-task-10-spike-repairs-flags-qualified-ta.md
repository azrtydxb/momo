# momo-01 Task 10: Spike repairs: flags, qualified table references and soft-delete filter

Status: open
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 10: Spike repairs: flags, qualified table references and soft-delete filter (`.procoder/plans/momo-01-foundation.md:948`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4, S-75. Files: `meta.go`, `select_repair_test.go`. Contributes to S-4, S-75; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestSoftDeleteFilterWithColumns` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestTableRefSchemaQualified` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

