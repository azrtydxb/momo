# momo-05 Task 26: Error redaction with Detail accessor (S-41)

Status: open
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 26: Error redaction with Detail accessor (S-41) (`.procoder/plans/momo-05-writes-queries.md:2172`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-41. Files: `errors.go`, `errors_redaction_test.go`.

## Acceptance criteria

- [ ] `TestErrorRedaction` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-41, part] `TestErrorRedaction` passes for the part of S-41 this task delivers; the full criterion is verified in story 200.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

- CI run 42410267553 (ci-agent, commit d310d41) green: unit, lint, fuzz-smoke and integration (postgres) jobs pass with this story's tests.
- Criteria left unticked: the red-first claim (no pre-implementation run exists; the first CI run only failed on two fixture mistakes fixed in d310d41) and the local procoder check gate (not run on this laptop).
