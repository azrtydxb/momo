# momo-09 Task 6: Tenant scope helper

Status: done 2026-11-07
Created: 2026-11-04
Epic: momo-09-observability-testing
Sprint: -

## Description

Implements plan momo-09-observability-testing Task 6: Tenant scope helper (`.procoder/plans/momo-09-observability-testing.md:1108`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-18. Files: `tenant.go`, `obs_tenant_test.go`. Contributes to S-18; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestTenantFilter` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `TestTenantFilter`: red run 42409637729 (tests committed first in fa87dbf, build failed with undefined symbols), green run 42410680387 on 8d0f6d7.
- Gate: lint, unit and integration jobs green on CI; commits fa87dbf, d5f35dd, 8d0f6d7.
- Fixed head: green run 42454191287 on 6a37944, after TenantFilter was changed to fail the build (ErrValidation, also under Not and Or) instead of emitting 1 = 0. Red evidence for the original test is a build failure (undefined symbols, run 42409637729).
