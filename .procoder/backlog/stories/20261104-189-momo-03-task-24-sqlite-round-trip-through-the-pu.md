# momo-03 Task 24: SQLite round trip through the public driver interface

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-03-drivers-sqlite
Sprint: -

## Description

Implements plan momo-03-drivers Task 24: SQLite round trip through the public driver interface (`.procoder/plans/momo-03-drivers.md:2279`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `driver/driver_roundtrip_test.go`.

## Acceptance criteria

- [x] `TestRoundTripSQLite` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

Red: the plan says this test needs no new code (its expected red is a missing Task 17). On the red head 0261a54 the driver_test package did not build at all (driver/config_identity_test.go imports the missing sqlitepure package), so TestRoundTripSQLite failed to build in red run 42466012379 on 0261a54d9b5091ec6d4053b47d088c198aae0867 (tests only on top of sprint-5, throwaway branch worktree-agent-aa450d3e0da3c4a3b-red, deleted afterwards; all seven jobs failed); against sprint-5 alone it would have passed, because sqlitedriver.Open already exists, so this red is a compile failure and not a behavioural one. Green: ci-agent run 42466819006 on 7d7faa67e3d4b835a4b7800b03ce5d0b86b5186c (lint, fuzz-smoke, unit, integration postgres, mysql84, mysql97, mariadb all success); the postgres integration job log has `ok github.com/azrtydxb/momo/driver` under -race. The gate was clean at the commit.

Deviations: the test has the `integration && cgo && !windows` tags (sqlitedriver.Open is a stub otherwise), closes the database in t.Cleanup and checks momo.New(d) is non-nil instead of discarding it. Added driver/config_identity_test.go, which asserts sqlitepure.Config is sqlitedriver.Config.
