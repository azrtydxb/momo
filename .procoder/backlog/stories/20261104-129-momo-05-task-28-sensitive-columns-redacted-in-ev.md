# momo-05 Task 28: Sensitive columns redacted in events, errors and dumps (S-42)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 28: Sensitive columns redacted in events, errors and dumps (S-42) (`.procoder/plans/momo-05-writes-queries.md:2317`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-42. Files: `sensitive.go`, `sensitive_test.go`, `db.go`.

## Acceptance criteria

- [ ] `TestSensitiveColumns` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-42] `TestSensitiveColumns`: with argument logging on, hook events, error text and momotest dumps contain a placeholder instead of a sensitive column's value; fails if any path prints it.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

