# migrate: Diff review follow-ups from story 086

Status: open
Created: 2026-11-05

## Description

The fresh review of story 086 (Engine.Diff writes migration files) found four
non-blocking gaps. Done means each one is fixed or pinned by a test, so a later
change to the history table name, the SQLite renderer or the sum format cannot
silently break Diff.

## Acceptance criteria

- [ ] One shared constant names the history table; history.go, runner.go and the Engine.Inspect filter all use it, and a test fails if the filter and the DDL disagree
- [ ] renderUp puts the statement terminator on its own line when the last line is a `--` comment, and a unit test covers a statement ending in a comment
- [ ] The Engine.Diff doc comment states that a NoTx statement makes the whole generated file run without a transaction
- [ ] Unit tests cover Diff with momo.sum absent (ComputeSum fallback) and with a name already recorded in momo.sum (refused)

## Evidence

