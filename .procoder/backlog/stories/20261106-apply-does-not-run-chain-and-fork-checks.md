# Apply does not run chain and fork checks

Status: open
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: -
Type: bug
Severity: s3

## Description

`apply` builds its pending list in migrate/engine.go pendingFrom (lines 124-132) without the chain/fork checks that `validate` runs. When two branches each add a migration and are merged, an older pending version can sort before an already applied newer one; apply runs it after the newer ones. Only `validate` catches forks, so a production deploy that skips validate applies migrations out of order against a snapshot chain that no longer matches.

Expected: apply refuses (non-zero exit, clear message) when the pending set has a fork, or a pending version older than the latest applied version.

## Acceptance criteria

- [ ] apply exits non-zero and applies nothing when a pending version is older than the latest applied version
- [ ] apply exits non-zero on a forked snapshot chain with the same error validate reports
- [ ] a linear chain still applies unchanged
- [ ] a regression test pins the fix: red before the change, green after

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->

