# momo-03 Task 22: Pure-Go SQLite adapter (modernc, no cgo, no vectors)

Status: open
Created: 2026-11-04
Epic: momo-03-drivers-sqlite
Sprint: -

## Description

Implements plan momo-03-drivers Task 22: Pure-Go SQLite adapter (modernc, no cgo, no vectors) (`.procoder/plans/momo-03-drivers.md:2179`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `driver/sqlitepure/open.go`, `driver/sqlitepure/open_test.go`, `go.mod`.

## Acceptance criteria

- [ ] `TestPureSQLiteCapabilitiesAndErrors` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

