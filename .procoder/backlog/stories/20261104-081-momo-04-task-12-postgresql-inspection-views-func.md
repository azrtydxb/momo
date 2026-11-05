# momo-04 Task 12: PostgreSQL inspection: views, functions, triggers, policies, RLS, partitions

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 12: PostgreSQL inspection: views, functions, triggers, policies, RLS, partitions (`.procoder/plans/momo-04-migrations.md:1320`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-37, S-38, S-39. Files: `migrate/postgres/inspect_objects.go`. Contributes to S-37, S-38, S-39; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestPostgresObjectInspection` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

