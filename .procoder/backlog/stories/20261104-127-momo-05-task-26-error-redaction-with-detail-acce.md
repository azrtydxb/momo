# momo-05 Task 26: Error redaction with Detail accessor (S-41)

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 26: Error redaction with Detail accessor (S-41) (`.procoder/plans/momo-05-writes-queries.md:2172`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-41. Files: `errors.go`, `errors_redaction_test.go`.

## Acceptance criteria

- [x] `TestErrorRedaction` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-41, part] `TestErrorRedaction` passes for the part of S-41 this task delivers; the full criterion is verified in story 200.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Green: ci-agent run 42453190263 on d8088d5 (unit, lint, fuzz-smoke, integration postgres).
- Gate: the commit hook ran the procoder format and lint gate on every commit of this story with 0 blocking findings.
- Red: run 42453213159 on throwaway branch worktree-agent-a1e9f39ea609a17b8-red (tests kept, implementation reverted) failed the build with undefined symbols for this story's API.
