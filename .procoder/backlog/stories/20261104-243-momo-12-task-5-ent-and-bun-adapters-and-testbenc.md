# momo-12 Task 5: Ent and Bun adapters and TestBenchEquivalence

Status: open
Created: 2026-11-04
Epic: momo-12-performance
Sprint: -
Carried: 007-sprint-7-finish-every-open-backlog-story-auditing-each — The S-19 criterion needs a committed make bench reference run on dedicated hardware, which a person has to run; the adapters and TestBenchEquivalence are in place.

## Description

Implements plan momo-12-performance Task 5: Ent and Bun adapters and TestBenchEquivalence (`.procoder/plans/momo-12-performance.md:459`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-19. Files: `bench/adapters/ent/schema/customer.go`, `bench/adapters/ent/ent.go`, `bench/adapters/bunad/bun.go`, `bench/equivalence_test.go`, `bench/impls_test.go`.

## Acceptance criteria

- [x] `TestBenchEquivalence` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-19] `make bench` reproduces published results; bench/results contains raw output, environment and pinned versions; `TestBenchEquivalence` asserts each compared ORM returns identical rows per workload; fails if a workload's SQL semantics differ between ORMs.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- TestBenchEquivalence: bench/equivalence_test.go with openAllImpls in bench/impls_test.go. It checks RecentOrders for two countries, CustomerByID, ErrNoRow, CustomersWithOrders, SetCustomerStatus visible through the pgx reference, InsertOrder and a 2500-row BulkInsertOrders for the five adapters against the handwritten pgx reference.
  Red proof: ci-agent run 42465164717 (headSha 0c7f14e) on branch worktree-agent-a51f3a530eba5c020-adapters-red, which keeps the tests and removes the implementations: bench-module fails with `no required module provides package` for the removed adapter packages and unit fails with `momo.InsertQuery[suite.Document] has no field or method Unnest`.
  Green proof: ci-agent run 42465734475, headSha eaec549861a73104452a68df580ea15421eac7fb, all eight jobs green (lint, unit, fuzz-smoke, integration postgres, integration mysql84/mysql97/mariadb, bench-module).
- The Ent code is generated into bench/adapters/ent/entgen by make bench-generate with an SPDX header, committed.
- NOT ticked: the S-19 criterion that `make bench` reproduces published results and bench/results holds a committed reference run. That needs a full run on a dedicated machine, which this environment does not have; the tooling (make bench, bench/scripts/run.sh) is in place and TestBenchResultsLayout proves the layout.
  Gate: commits went through the procoder commit hook, with a hand-written go.work (untracked) so the hook can lint the nested bench module.
- Carry reason: the unticked S-19 criterion needs a committed `make bench` reference run from a dedicated machine, which no agent environment provides (shared CI runner numbers would contradict docs/PERFORMANCE.md policy). Tooling is shipped: Makefile:112 (`bench`), bench/scripts/run.sh, bench/results/README.txt; layout proven by TestBenchResultsLayout (bench/scripts/run_test.go). Needs a human to run `make bench` on dedicated hardware and commit bench/results.
