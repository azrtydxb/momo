# momo-03 Task 25: Document process-wide effects and hardening of the cgo SQLite driver

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-03-drivers-sqlite
Sprint: -

## Description

Implements plan momo-03-drivers Task 25: Document process-wide effects and hardening of the cgo SQLite driver (`.procoder/plans/momo-03-drivers.md:2334`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `driver/sqlitedriver/doc.go`, `driver/sqlitedriver/doc_test.go`.

## Acceptance criteria

- [x] `TestDocStatesProcessGlobalEffects` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

Red: red run 42466012379 on 0261a54d9b5091ec6d4053b47d088c198aae0867 (tests only on top of sprint-5, throwaway branch worktree-agent-aa450d3e0da3c4a3b-red, deleted afterwards; all seven jobs failed); TestDocStatesProcessGlobalEffects failed (`open doc.go: no such file or directory`) in the unit job log. Green: ci-agent run 42466819006 on 7d7faa67e3d4b835a4b7800b03ce5d0b86b5186c (lint, fuzz-smoke, unit, integration postgres, mysql84, mysql97, mariadb all success); `ok github.com/azrtydxb/momo/driver/sqlitedriver` in the unit job. The gate was clean at the commit (13e6b3e).

Deviations: the package comment moved out of config.go into the new doc.go (a package has one doc comment); doc.go is untagged so it also documents the no-cgo stub. It has sections on requirements (cgo, C toolchain, no Windows, sqlitepure as the alternative), process-wide effects (sqlite_vec.Auto through sqlite3_auto_extension), hardening (`-tags sqlite_omit_load_extension`; every connection also verifies load_extension is refused) and the Config defaults.
