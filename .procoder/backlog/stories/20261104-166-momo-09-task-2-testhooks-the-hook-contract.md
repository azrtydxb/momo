# momo-09 Task 2: TestHooks, the hook contract

Status: open
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: -

## Description

Implements plan momo-09-observability-testing Task 2: TestHooks, the hook contract (`.procoder/plans/momo-09-observability-testing.md:438`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-18. Files: `obs_hooks_test.go`, `db.go`.

## Acceptance criteria

- [ ] `TestHooks` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-18, part] `TestHooks` passes for the part of S-18 this task delivers; the full criterion is verified in story 169.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

- `TestHooks` (obs_hooks_test.go) passes on CI (run 42410680387). Mutations run 42411352506: logging args unconditionally failed with `Args = [Ada] NumArgs = 1, want nil and 1`; dropping the row counter failed with `Rows = 0, want 4`. Criteria left open: the plan's test passes at once, so red-first cannot be shown, and the FlagSensitive redaction subtest is omitted because momo.Redacted does not exist yet (sensitive-arg masking is owned by another agent).
