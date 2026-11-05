# momo-05 Task 20: Automatic timestamps and the injectable clock (S-68)

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-05-writes-queries Task 20: Automatic timestamps and the injectable clock (S-68) (`.procoder/plans/momo-05-writes-queries.md:1635`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-68. Files: `clock.go`, `timestamps_test.go`, `db.go`.

## Acceptance criteria

- [x] `TestAutoTimestamps` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-68] `TestAutoTimestamps` asserts insert sets both, update sets only updated, an explicit Set wins, and the injected clock is used; fails if updated_at changes on a no-op guarded update that matched no row.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: `go test -run TestAutoTimestamps .` failed before the implementation (`TestAutoTimestampsSQL`: insert rendered no created_at/updated_at).
- Green: `TestAutoTimestamps` (Postgres, `-tags integration -race`) and `TestAutoTimestampsSQL` pass. They cover insert setting both, update setting only updated_at, an explicit created_at on insert winning (also non-zero Model fields), the injected clock, the clock read once per multi-row statement, and a no-match guarded update leaving updated_at unchanged.
- Full integration run: `go test -tags integration -race -count=1 .` ok. Also `go vet ./...`, `gofmt -l .` clean, `go test ./...` ok.
- Note: `Update.Set` keeps refusing updated_at, so an explicit assignment winning applies to insert only.

- Gate: `procoder check` 0 blocking on merge into sprint-2; fresh review found 0 Critical/0 Important (4 Minor, folded into the story 120 follow-up).
