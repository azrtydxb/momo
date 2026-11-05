# Sprint 1: Foundation of the query core

Status: closed 2026-11-04
Created: 2026-11-04

## Goal

Plan momo-01 is fully delivered and its gate is green. The uncommitted spike becomes the tested foundation every later plan builds on:

- dialects with full capability flags and quoting;
- schema base types with JSON round-trip, format versions, normalization and canonical sort;
- structured errors and sentinels;
- the fake driver and fixtures;
- placeholders, raw SQL and SELECT rendering under golden and fuzz tests;
- keyset pagination, query immutability, cursor closing and the compile-fail harness;
- the `internal/testdb` harness with per-test PostgreSQL schemas;
- CI on free runners, security workflows, release-please and the import-graph guard.

Done means the Task 23 foundation gate passes locally and in CI against PostgreSQL.

## Result

committed: 23
done: 23 (20261104-001-momo-01-task-1-repository-licence-and-community, 20261104-002-momo-01-task-2-makefile-and-local-command-parity, 20261104-003-momo-01-task-3-dialect-full-capability-set-flavo, 20261104-004-momo-01-task-4-schema-base-types-with-json-round, 20261104-005-momo-01-task-5-snapshot-format-versions-and-refu, 20261104-006-momo-01-task-6-normalization-of-types-expression, 20261104-007-momo-01-task-7-canonical-sort-for-every-base-col, 20261104-008-momo-01-task-8-error-sentinels-structured-error, 20261104-009-momo-01-task-9-fake-driver-and-shared-model-fixt, 20261104-010-momo-01-task-10-spike-repairs-flags-qualified-ta, 20261104-011-momo-01-task-11-placeholders-quoting-and-the-bui, 20261104-012-momo-01-task-12-raw-sql-placeholders, 20261104-013-momo-01-task-13-select-rendering-golden-tests, 20261104-014-momo-01-task-14-keyset-pagination-with-determini, 20261104-015-momo-01-task-15-query-immutability-under-concurr, 20261104-016-momo-01-task-16-cursor-closing-and-empty-result, 20261104-017-momo-01-task-17-compile-fail-harness-and-valid-e, 20261104-018-momo-01-task-18-test-database-url-helpers-and-po, 20261104-019-momo-01-task-19-ci-workflow-core-with-the-postgr, 20261104-020-momo-01-task-20-security-and-supply-chain-workfl, 20261104-021-momo-01-task-21-release-automation-and-pr-title, 20261104-022-momo-01-task-22-import-graph-guard, 20261104-023-momo-01-task-23-foundation-gate)
carried: 0

## Retro

**What slowed us down**

- Backlog seeding put spec criteria on stories that only contribute to them. 119 stories could never close, and a further 38 shared criteria blocked every owner except the last. Both were fixed mid-sprint by moving those criteria into "Contributes to" notes, or by scoping each to its owning story.
- The plan assumed greenfield code, but the spike already implemented Sort, Order, the immutability clips and several checks. "Failed before the implementation" was impossible for stories 007, 011, 012, 013, 015, 016, 020 and 022 until we adopted the mutation-check precedent.
- Several plan-level defects only surfaced during the build:
  - go.mod was never committed by any task;
  - CreatedAt/UpdatedAt were referenced before they existed;
  - the schema-qualified golden left columns unqualified;
  - tests were too weak (TestQueryReuseNoLeak, the raw-SQL too-many-arguments case, the nullable keyset key).
- My own lexical-sort merge of go.sum broke `make tidy-check`; the gate caught it.
- A stale `.git/index.lock` and parallel agents sharing one git index caused commit collisions until worktrees were used.

**What we change next sprint**

- Before seeding stories from a plan, check each task against the existing code. Any task that adopts existing code gets the mutation-check criterion wording from the start.
- Every agent brief carries the rules: never tick an unverified box, mutation-check pre-existing code, and stop on plan conflicts.
- Independent stories run in parallel worktrees. The lead merges them, and resolves go.mod/go.sum with `rm go.sum && go mod tidy`, never a text sort.
- Each agent's "passes" claim gets one mutation from the lead before merge, wherever a test is the only guard.

**One adaptation worth keeping**

- The lead reviews every story's red evidence before merge. That caught three weak tests and the schema-qualified column inconsistency, which the agents' own green runs would have shipped.

**Not yet verified:** the goal's "green in CI" half. The branch is not pushed, so CI has not run.
