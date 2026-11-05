# momo-04 Task 18: Baseline and drift

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 18: Baseline and drift (`.procoder/plans/momo-04-migrations.md:2115`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-10, S-11. Files: `migrate/baseline.go`. Contributes to S-10, S-11; those criteria are verified in the stories that define their named tests.

## Dependencies

- Hard dependency (Task 11): drift must call `postgres.Dialect.InspectWithInvalid`, not plain `Inspect`, and report every returned invalid index name as Changed with Detail "invalid index (failed CONCURRENTLY build)". A plain Inspect hides a failed CONCURRENTLY build.

## Hard requirement (Task 12 review)

A dialect hook `Canonicalize(ctx, conn, *schema.Schema) error` creates the declared views and functions in a scratch schema (dependencies first), reads back `pg_get_viewdef`, `pg_get_functiondef` and the policy and trigger expressions, and rewrites the declared text. Baseline stores the canonical text, not the inspected text. Without it the first Diff after baseline drops and recreates every view and its dependents, losing materialized view data.

## Acceptance criteria

- [ ] Drift excludes `momo_migrations` and honours `InspectOptions.Exclude` (the dialect Inspect does not apply Exclude)
- [ ] Baseline then diff gives zero changes for a declared view written differently from pg_get_viewdef's output, for a declared function with a differently formatted body, and for a materialized view (data kept)

- [ ] `TestBaselineAndDrift` exists as written in the plan, failed before the implementation, and passes now
- [ ] Drift reports invalid indexes (via `InspectWithInvalid`) as Changed with Detail "invalid index (failed CONCURRENTLY build)"
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

