# momo-10 Task 11: Field-number lock file

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: -

## Description

Implements plan momo-10-ecosystem-generators Task 11: Field-number lock file (`.procoder/plans/momo-10-ecosystem-generators.md:932`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `gen/grpc/lock.go`, `gen/grpc/lock_test.go`.

## Acceptance criteria

- [x] `TestProtoLockStableNumbers` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Files: gen/grpc/lock.go, lock_test.go.
- TestProtoLockStableNumbers (also TestLoadLockFailsClosed and TestAssignSkipsProtobufRangeAndFlagsRepeats) red: CI run 42455374066 (tests alone on sprint-5) failed with "undefined: Lock, LoadLock". Green: CI run 42456595252 (commit 5f4ffe6).
- Deviation from the plan text: there is no Lock.Save. The repo test TestPluginsUseWriter (run 42455728306) forbids generator packages from writing files, so Lock.Marshal returns the bytes for the gen.Writer, and the test writes them itself. The assertions of TestProtoLockStableNumbers are the plan's.
- LoadLock fails closed: a missing file is an empty lock, but invalid JSON, a missing or newer version, a number outside 1 to 536870911 or in the protobuf-reserved 19000 to 19999 range, or a repeated number is an error. Assign skips the reserved range.
- Open for the plugin story (222): the lock is JSON and cannot carry the generated header, so it needs a Writer method that writes an owned, headerless file; Writer.Text refuses it.

