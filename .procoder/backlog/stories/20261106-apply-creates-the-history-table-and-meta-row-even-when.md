# apply creates the history table and meta row even when nothing is pending

Status: done 2026-11-06
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: 003-sprint-3-postgresql-hardening-migration-safety-bugs-query
Type: bug
Severity: s4

## Description

migrate/engine.go 379: `apply` creates momo_migrations and its meta row before it knows whether any version is pending. A no-op apply against a database still writes DDL and rows, surprising read-mostly environments and role setups.

Expected: no writes when nothing is pending (or document and gate it).

## Acceptance criteria

- [x] apply with no pending migrations on a database without the history table creates nothing
- [x] apply with pending migrations still creates the history table first
- [x] a regression test pins the fix: red before the change, green after

## Evidence

- TestApplyNothingPendingWritesNothing: with no history table (and with everything applied) Apply executes zero statements and the table is not created. Mutation (early return removed) fails it. TestApplyNothingPendingCreatesNoHistoryTable runs the same on PostgreSQL in CI.
- TestApplyWithPendingCreatesHistoryFirst: the first statement is CREATE TABLE IF NOT EXISTS momo_migrations and the row ends applied.
- Red first: Apply called EnsureHistory before knowing what is pending, so the first test saw the CREATE TABLE.

