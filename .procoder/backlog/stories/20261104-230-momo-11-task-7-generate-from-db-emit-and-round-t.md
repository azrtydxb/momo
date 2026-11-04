# momo-11 Task 7: generate --from-db emit and round trip

Status: open
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: -

## Description

Implements plan momo-11-adoption Task 7: generate --from-db emit and round trip (`.procoder/plans/momo-11-adoption.md:681`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-92. Files: `gen/fromdb/emit.go`, `cmd/momo/generate.go`.

## Acceptance criteria

- [ ] `TestGenerateFromDB` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-92] `TestGenerateFromDB` inspects the example database, generates models, and asserts they diff to zero against it; fails if a column type is unmapped silently.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

