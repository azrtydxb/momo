# momo-04 Task 5: momo.sum checksum file

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 5: momo.sum checksum file (`.procoder/plans/momo-04-migrations.md:707`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-11, S-22. Files: `migrate/sum.go`. Contributes to S-11, S-22; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestSumFormatVersion` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestChecksumMismatch` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

