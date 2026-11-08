# momo-05 Task 28: Sensitive columns redacted in events, errors and dumps (S-42)

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 28: Sensitive columns redacted in events, errors and dumps (S-42) (`.procoder/plans/momo-05-writes-queries.md:2317`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-42. Files: `sensitive.go`, `sensitive_test.go`, `db.go`.

## Acceptance criteria

- [x] `TestSensitiveColumns` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-42] `TestSensitiveColumns`: with argument logging on, hook events, error text and momotest dumps contain a placeholder instead of a sensitive column's value; fails if any path prints it.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: run 42455602122 on the throwaway branch worktree-agent-aecd38e0a7c8adf10-red-sens (maskArgs sensitive loop removed, branch deleted): TestSensitiveColumns/hook_events and TestSensitiveColumns/failed_statement failed in unit and integration, together with the existing masking tests.
- Green: green run 42455574669 on a5e569e0712fc7d785b63df4afac3ecfac87054e (ci-agent: lint including tidy-check, unit, integration postgres and fuzz-smoke all success).
- The masking itself (maskArgs, RedactedArg, per-position sens indexes, Compiled.String printing no values) pre-existed on sprint-5; this story adds TestSensitiveColumns covering hook events (before, after, failed statement), error text and the compiled-statement dump, also through the v and +v verbs. Deviation: the placeholder is RedactedArg replacing only the sensitive positions, not a one-element Args slice. A momotest dump was not exercised: dumps are Compiled.String, which prints no values.
