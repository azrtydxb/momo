# momo-03 Task 12: Iterator closes the cursor on early exit (S-4 cursor guarantee)

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-03-drivers-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-03-drivers Task 12: Iterator closes the cursor on early exit (S-4 cursor guarantee) (`.procoder/plans/momo-03-drivers.md:1480`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-4. Files: `driver/pgxdriver/iter_test.go`.

## Acceptance criteria

- [x] `TestIterClosesCursor` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-4] `TestIterClosesCursor`: Each/Iter close the cursor on early break, callback error and context cancellation, asserted via pool stats (no acquired connections afterwards); fails if a return path skips Close.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Test: driver/pgxdriver/iter_test.go TestIterClosesCursor, subtests: break, panic in loop body, panic in Each callback, callback error, cancelled Each, cancelled Iter, pinned connection (Release succeeds, not busy), Tx stays usable after early exit. Each asserts PoolStats().AcquiredConns == 0; goroutine count is checked at the end.
- Red: the first run failed because the plan fixture left Identity/SoftDelete at 0, which added "id IS NULL" and returned no rows (fixture fix: derived indexes set to -1). The implementation (defer r.Close() in Each/Iter) already existed on sprint-2, so there was no red against missing behaviour.
- Mutation: removing both defer r.Close() calls in select.go failed break, panic, callback error and cancel subtests with "cursor left open ... acquired"; restored.
- Run: MOMO_REQUIRE_INTEGRATION=1 go test -tags integration -race -count=1 ./driver/pgxdriver gives ok. Gate: procoder check 0 blocking; go build, go vet, make staticcheck clean.

