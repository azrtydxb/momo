# momo-04 Task 9: Dependency-aware ordering of managed objects

Status: done 2026-11-05
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 9: Dependency-aware ordering of managed objects (`.procoder/plans/momo-04-migrations.md:1063`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-36, S-37, S-39. Files: `migrate/order.go`. Contributes to S-36, S-37, S-39; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestDropViewBeforeDependentView` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestCreateViewAfterDependency` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestCycleIsAnError` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Failing run (before migrate/order.go existed): `go test ./migrate/ -run 'TestDrop|TestCreate|TestCycle|TestOrder'` failed to build with `undefined: Order` and `undefined: OrderErr`.
- Passing run: the same command prints `ok github.com/azrtydxb/momo/migrate`; `go test -race -count=1 ./migrate/... ./schema/...` passes.
- Mutation check: flipping the drop_view sort direction made TestDropViewBeforeDependentView fail ("got base_v then top_v"); reverted.
- Extra tests cover cyclic FK create (add_fk split), cyclic drop (drop_fk first), partitions, phase order, determinism and unbreakable cycles.
- Gate: `procoder check` 0 blocking findings; `make staticcheck`, `go build ./...`, `go vet ./...` clean.
- Commit: feat(migrate): dependency-aware ordering of managed objects.
