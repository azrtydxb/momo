# momo-05 Task 27: Typed model hooks (S-85)

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 27: Typed model hooks (S-85) (`.procoder/plans/momo-05-writes-queries.md:2238`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-85. Files: `hooks.go`, `hooks_test.go`, `meta.go`.

## Acceptance criteria

- [x] `TestModelHooks` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-85] `TestModelHooks` asserts invocation order, error abort before SQL, and zero reflection in the call path (benchmark allocation check); fails if a hook runs for a builder that did not opt in.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: run 42455587059 on the throwaway branch worktree-agent-aecd38e0a7c8adf10-red (tests only, implementation absent, branch deleted) failed to build: undefined HookSet, runModelHook and afterLoad.
- Green: green run 42455574669 on a5e569e0712fc7d785b63df4afac3ecfac87054e (ci-agent: lint including tidy-check, unit, integration postgres and fuzz-smoke all success).
- Tests: TestModelHooks (order insert, load, update; BeforeUpdate veto aborts with no SQL; BeforeInsert value reaches the driver without touching the caller's model), TestModelHooksOnlyForOptedInBuilders (a hook runs only for its own builder kind), TestModelHooksBeforeAbortWithoutSQL, TestModelHooksAfterLoadOnEveryRead (All, Each, Iter, First, ScanModel), TestModelHookDispatchAllocations (zero allocations on the dispatch). They use the fake driver, so they run in the unit job.
- Deviation: the plan test uses the shared integration suite; the fake driver is used so the assertions run without a database. Hooks do not run for SQL, and relation loads do not run the related type's AfterLoad (documented on HookSet).
