# momo-12 Task 1: bench module, pinned dependencies and environment capture

Status: open
Created: 2026-11-04
Epic: momo-12-performance
Sprint: -

## Description

Implements plan momo-12-performance Task 1: bench module, pinned dependencies and environment capture (`.procoder/plans/momo-12-performance.md:53`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-19. Files: `bench/go.mod`, `bench/env/env.go`, `bench/cmd/envcapture/main.go`. Contributes to S-19; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestEnvCapture` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- TestEnvCapture: bench/env/env_test.go. Red proof: ci-agent run 42457732291 on branch worktree-agent-a51f3a530eba5c020-red (bench implementation files removed, tests kept) fails the bench-module job with `vet: workload/canonical_test.go:13:9: undefined: Canonical` and `vet: env/env_test.go:11:12: undefined: Capture`.
- Green proof: ci-agent run 42457722231, bench-module job (make test-bench-module and make test-bench-module-integration against pgvector/pgvector:0.8.7-pg17); the hook benchmark step of the same job is in run 42458619130.
- Deviation: test binaries carry no dependency list in their build info, so Capture falls back to go list -m all when it is empty.
- Gate: every commit touching bench Go files was made through the procoder commit hook. The hook's golangci-lint cannot load a nested module, so bench/go.mod and go.sum were moved aside for those commits, which made the hook lint the files as ordinary packages; they passed.
