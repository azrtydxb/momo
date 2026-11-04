# momo-05 Task 26: Error redaction with Detail accessor (S-41)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 26: Error redaction with Detail accessor (S-41) (`.procoder/plans/momo-05-writes-queries.md:2172`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-41. Files: `errors.go`, `errors_redaction_test.go`.

## Acceptance criteria

- [ ] `TestErrorRedaction` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-41] `TestErrorRedaction`: a unique violation on an email column returns an error whose Error() lacks the email while the detail accessor and errors.As expose it; fails if driver detail is concatenated into the message.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

