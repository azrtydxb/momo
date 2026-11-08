# momo-10 Task 1: Integration module with pinned tooling

Status: done 2026-11-07
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: -

## Description

Implements plan momo-10-ecosystem-generators Task 1: Integration module with pinned tooling (`.procoder/plans/momo-10-ecosystem-generators.md:41`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `internal/integration/go.mod`, `internal/integration/go.sum`, `internal/integration/tools.go`, `internal/integration/tools.lock`, `internal/integration/integration_test.go`.

## Acceptance criteria

- [x] `TestIntegrationModuleBuilds` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Files: internal/integration/go.mod, go.sum, tools.go, tools.lock, integration_test.go. `make test` now also runs the module.
- TestIntegrationModuleBuilds red: CI run 42414302688 (branch worktree-agent-a70d1eb94160543bf-red, the test alone on sprint-5) failed with "integration_test.go:22: github.com/99designs/gqlgen: exit status 1". Green: CI run 42412136570 (commit 015deab, lint, unit, fuzz-smoke and integration postgres) succeeded.
- go.mod pins gqlgen v0.17.97, grpc v1.84.0, protobuf v1.36.12, protoc-gen-go-grpc v1.6.2 and jsonschema/v6 v6.0.3. tools.lock records the SHA-256 of the three protoc v36.2 archives, computed from the release assets. Nothing consumes it yet: TestToolsLockWellFormed only checks that it is well formed and complete. Story 219 must verify the archive SHA-256 against it before running protoc and fail closed.
- The nested module is covered by the gates: make tidy-check runs go mod tidy -diff there, make vet vets it with the tools tag, and dependabot has a gomod entry for /internal/integration.
- The module requires momo through a blank import in tools.go, so go mod tidy keeps the replace directive.

