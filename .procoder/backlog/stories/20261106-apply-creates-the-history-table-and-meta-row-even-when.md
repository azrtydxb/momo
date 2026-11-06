# apply creates the history table and meta row even when nothing is pending

Status: open
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: -
Type: bug
Severity: s4

## Description

migrate/engine.go 379: `apply` creates momo_migrations and its meta row before it knows whether any version is pending. A no-op apply against a database still writes DDL and rows, surprising read-mostly environments and role setups.

Expected: no writes when nothing is pending (or document and gate it).

## Acceptance criteria

- [ ] apply with no pending migrations on a database without the history table creates nothing
- [ ] apply with pending migrations still creates the history table first
- [ ] a regression test pins the fix: red before the change, green after

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->

