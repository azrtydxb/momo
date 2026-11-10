# momo-13 Task 3: Runnable examples, run through testopen.Each

Status: done 2026-11-10
Created: 2026-11-04
Epic: momo-13-docs-release
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each
Carried: 007-sprint-7-finish-every-open-backlog-story-auditing-each — Superseded plan task: its single TestExamples over testopen.Each with Run(ctx, db) library functions was replaced by self-contained examples/<name>/main.go programs, each with its own example_test.go on examples/internal/exampletest, PostgreSQL only. User decision 2026-11-10 (.procoder/ask/decisions.md, Sprint 7 backlog decisions): examples stay PostgreSQL only and S-21 in .procoder/specs/momo.md is reworded to match; TestExamplesAreWired (internal/repocheck) fails when an example has no test. The story criteria still name TestExamples and both databases, and a criterion may not be weakened, so the story is carried, not closed.

## Description

Implements plan momo-13-docs-release Task 3: Runnable examples, run through testopen.Each (`.procoder/plans/momo-13-docs-release.md:275`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-21. Files: `examples/momo.yaml`, `examples/migrations/embed.go`, `examples/internal/open/open.go`, `examples/crud/example.go`, `examples/examples_test.go`.

## Acceptance criteria

- [x] Every example has its own `example_test.go` built on `examples/internal/exampletest`, and `TestExamplesAreWired` fails when an example has no test (decision 2026-11-10: the plan's single `TestExamples` is replaced by these per-example PostgreSQL tests)
- [x] [S-21] Every example runs end to end against PostgreSQL in CI through its own `example_test.go`; docs/ contains architecture, capability matrix, migration safety and recovery guide, contributor guide and supported versions; fails if an example is not wired into the test.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Left open: this repository runs its examples through per-example tests built on examples/internal/exampletest, not a single TestExamples over testopen.Each, and only the PostgreSQL entry exists. TestExamplesAreWired (internal/repocheck) already fails when an example has no test. No code was changed here.
- Carry reason: the plan's single `TestExamples` over `testopen.Each` with `Run(ctx, db)` library functions was superseded by self-contained programs under `examples/<name>/main.go`, each with its own `example_test.go` built on `examples/internal/exampletest` (`examples/internal/exampletest/exampletest.go`, `Open` and `CheckGenerated`), on PostgreSQL only with per-example PostgreSQL migrations (`examples/*/migrations`). `TestExamplesAreWired` (`internal/repocheck/examples_test.go:65`) fails when an example has no test. `testopen.Each` now exists (`internal/testdb/testopen/testopen.go:111`) with the MySQL and SQLite entries, but no example ships migrations for them.
- Needs decision: S-21 says the examples run against both databases. Options: (1) keep PostgreSQL-only examples and reword S-21 to say so; (2) generate per-dialect migrations for each example and run them through `testopen.Each`; (3) run only the examples whose SQL is dialect-neutral on MySQL and SQLite. Not decided here.
- Decision 2026-11-10 (`.procoder/ask/decisions.md`, "Sprint 7 backlog decisions"): option 1. The examples stay PostgreSQL only and S-21 in `.procoder/specs/momo.md` is reworded to match. This story stays carried: its plan task (a single `TestExamples` over `testopen.Each`) is superseded by the per-example tests above.
- Criteria reworded 2026-11-10 to the per-example PostgreSQL tests (`.procoder/ask/decisions.md`, "Sprint 7 docs decisions"; S-21 in the spec already says PostgreSQL). Every example directory has an `example_test.go` on examples/internal/exampletest, run by the integration (postgres) job; `TestExamplesAreWired` (internal/repocheck/examples_test.go) fails when one lacks its test. Shown failing on throwaway branch worktree-agent-acf2b-red-examples (sha 201691f, examples/crud-zero-null/example_test.go deleted): ci-agent run 42830371829 FAIL TestExamplesAreWired. Green: ci-agent run 42839158680 (sha 5e86d37, all 9 jobs success), integration (postgres) job.
