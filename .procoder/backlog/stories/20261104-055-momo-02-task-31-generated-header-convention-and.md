# momo-02 Task 31: Generated header convention and CLI exit codes

Status: open
Created: 2026-11-04
Epic: momo-02-generator
Sprint: -

## Description

Implements plan momo-02-generator Task 31: Generated header convention and CLI exit codes (`.procoder/plans/momo-02-generator.md:3591`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `gen/header.go`, `gen/header_test.go`, `cmd/momo/cli_contract_test.go`, `cmd/momo/cmd.go`.

## Acceptance criteria

- [ ] `TestGeneratedHeaderRegexp` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestCLIExitCodes` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

