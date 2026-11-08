# momo-03 Task 23: Pure-Go build proof (S-89)

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-03-drivers-sqlite
Sprint: -

## Description

Implements plan momo-03-drivers Task 23: Pure-Go build proof (S-89) (`.procoder/plans/momo-03-drivers.md:2235`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-89. Files: `driver/sqlitepure/nocgo_test.go`, `driver/sqlitepure/classify.go`.

## Acceptance criteria

- [x] `TestSQLitePureGo` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-89] `TestSQLitePureGo` runs the SQLite suite with CGO_ENABLED=0 and asserts vector queries return ErrUnsupported; fails if the build requires cgo.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

Red: red run 42466012379 on 0261a54d9b5091ec6d4053b47d088c198aae0867 (tests only on top of sprint-5, throwaway branch worktree-agent-aa450d3e0da3c4a3b-red, deleted afterwards; all seven jobs failed); the sqlitepure package did not compile on that head, so TestSQLitePureGo failed with the rest. Green: ci-agent run 42466819006 on 7d7faa67e3d4b835a4b7800b03ce5d0b86b5186c (lint, fuzz-smoke, unit, integration postgres, mysql84, mysql97, mariadb all success). `make test-sqlitepure` (CGO_ENABLED=0 go test -count=1 -tags=integration ./driver/sqlitepure/...) is a step of the unit job in ci-agent and in ci.yml and passed in the same run: `ok github.com/azrtydxb/momo/driver/sqlitepure`, which proves the package and its tests build and pass with cgo off. TestSQLitePureGo asserts that `SELECT vec_version()` and a `vec_distance_L2` query fail with momo.ErrUnsupported (with Op set). TestSQLitePureImportGraphHasNoCgoDriver runs `CGO_ENABLED=0 go list -deps` and fails on mattn/go-sqlite3, sqlite-vec, "C" or driver/sqlitedriver (it was widened in ba8b308 to cover pgxdriver as well; see story 191 for its run). The gate was clean at each commit.

Deviations: the `vec_` rule is in classify.go as the plan says, but it is applied before the result-code switch and the caller's operation ("select" or "exec") is recorded on the error through classifyOp. The plan's "-race ok" with CGO_ENABLED=0 is not possible (the race detector needs cgo): the race run is the integration job (cgo on) and the no-cgo run is `make test-sqlitepure` without -race. The plan test imports sqlitedriver; ours does not, for the reason in story 187.
