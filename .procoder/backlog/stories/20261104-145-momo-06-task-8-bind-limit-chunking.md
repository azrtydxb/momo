# momo-06 Task 8: Bind-limit chunking

Status: done 2026-11-06
Created: 2026-11-04
Epic: momo-06-relations
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-06-relations Task 8: Bind-limit chunking (`.procoder/plans/momo-06-relations.md:2342`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-5, S-6. Files: `options_maxparams.go`, `load_test.go`. Contributes to S-5; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestEagerLoadChunking` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-6, part] this story's tests pass for the part of S-6 this task delivers; the full criterion is verified in story 146.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- TestEagerLoadChunking: red first (`undefined: momo.WithMaxParams`), green on Postgres with `-tags integration`; four chunked statements, four comments.
- Mutations caught: no dialect copy, ignored n, transaction dropping the limit, chunk test fails when the limit is raised.
- `procoder check` 0 blocking; `make fmt-check vet staticcheck gosec tidy-check` and `go test -race ./...` pass; committed as feat(relations): bind-limit chunking with WithMaxParams.

### Review fixes

- WithMaxParams(n) now panics at New for n <= 0 (0 or negative read as "no limit" in every guard), clamps n above the dialect limit to the dialect limit, and clones Vector.Metrics/Indexes in the dialect copy. Doc states n semantics and that the limit covers Tx, OnConn handles, Load/LoadAll, bulk inserts and raw In().
- Re-checked the New() reorder against story 122's WithFieldHooks: both orders work (TestWithMaxParamsComposesWithFieldHooks).
- New Postgres-free tests in options_maxparams_limits_test.go: rejection of 0/negative, clamp, deep copy, Load chunking on the fake driver, Load inside Tx, OnConn inheritance, scope-leaves-no-room validation error, raw In() rejection under a lowered limit.
- Mutations caught: clamp removed, slices.Clone removed, OnConn using db.drv.Dialect(), Tx using db.drv.Dialect(), load scope check removed, checkMaxParams removed, chunk size ignoring the limit, panic removed.
