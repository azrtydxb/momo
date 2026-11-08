# momo-12 Task 4: Momo and GORM adapters

Status: open
Created: 2026-11-04
Epic: momo-12-performance
Sprint: -

## Description

Implements plan momo-12-performance Task 4: Momo and GORM adapters (`.procoder/plans/momo-12-performance.md:348`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-19. Files: `bench/models/momo/customer.go`, `bench/adapters/momo/momo.go`, `bench/adapters/gormad/gorm.go`, `bench/adapters/gormad/gorm_test.go`, `bench/adapters/gormad/open.go`. Contributes to S-19; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestGormAdapter` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- TestGormAdapter: bench/adapters/gormad/gorm_test.go, run by the bench-module integration step against pgvector/pgvector:0.8.7-pg17. The Momo adapter (bench/adapters/momo) and its generated models (bench/models/momo, produced by momo generate on CI, run 42463438648) are covered by TestBenchEquivalence in story 243.
  Red proof: ci-agent run 42465164717 (headSha 0c7f14e) on branch worktree-agent-a51f3a530eba5c020-adapters-red, which keeps the tests and removes the implementations: bench-module fails with `no required module provides package` for the removed adapter packages and unit fails with `momo.InsertQuery[suite.Document] has no field or method Unnest`.
  Green proof: ci-agent run 42465734475, headSha eaec549861a73104452a68df580ea15421eac7fb, all eight jobs green (lint, unit, fuzz-smoke, integration postgres, integration mysql84/mysql97/mariadb, bench-module).
- Dependency resolution and code generation ran on CI, not locally: throwaway workflows ran go mod tidy, momo generate and ent generate and uploaded the output, which was committed; make bench-generate reproduces it.
  Gate: commits went through the procoder commit hook, with a hand-written go.work (untracked) so the hook can lint the nested bench module.
