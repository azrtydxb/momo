# momo-04 Task 1: Schema types for snapshots and managed objects

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 1: Schema types for snapshots and managed objects (`.procoder/plans/momo-04-migrations.md:73`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-1, S-22, S-36, S-37, S-38, S-39. Files: `schema/schema.go`, `schema/types.go`, `schema/format_test.go`. Contributes to S-1, S-22, S-36, S-37, S-38, S-39; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestSchemaFormatVersion` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

