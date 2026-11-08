# momo-05 Task 29: Interceptors and the per-handle privacy recipe (S-86)

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-05-writes-queries
Sprint: -

## Description

Implements plan momo-05-writes-queries Task 29: Interceptors and the per-handle privacy recipe (S-86) (`.procoder/plans/momo-05-writes-queries.md:2394`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-86. Files: `interceptor.go`, `interceptor_test.go`, `docs/PRIVACY_RULES.md`.

## Acceptance criteria

- [x] `TestInterceptors` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-86] `TestInterceptors` asserts an interceptor can add a predicate and deny a mutation, applies only to its handle, and is visible in compiled SQL; fails if an interceptor registered on one handle affects another.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: run 42455587059 on the throwaway branch worktree-agent-aecd38e0a7c8adf10-red (tests only, implementation absent, branch deleted) failed to build in the root package (interceptor_test.go uses WithInterceptor and Operation).
- Green: green run 42458998121 on 08f022ed7aa823b0e52cfca67853a90da8dfa131, the merged tree with the review fixes (ci-agent: all jobs success; the first green run was 42455574669 on a5e569e).
- Tests: TestInterceptors (predicate added to the executed select and count, visible in compiled SQL, delete denied with no SQL, a plain handle on the same driver unaffected), TestInterceptorsAreNotAFilterForTheMutationGuard, TestInterceptorsFollowTheHandleIntoTransactions (a Tx from the handle has it, a Tx from another handle does not), TestInterceptorDeniesInsert. docs/PRIVACY_RULES.md holds the tenant recipe and the not-a-security-boundary statement.
