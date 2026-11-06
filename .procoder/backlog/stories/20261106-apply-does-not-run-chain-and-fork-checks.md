# Apply does not run chain and fork checks

Status: done 2026-11-06
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: 003-sprint-3-postgresql-hardening-migration-safety-bugs-query
Type: bug
Severity: s3

## Description

`apply` builds its pending list in migrate/engine.go pendingFrom (lines 124-132) without the chain/fork checks that `validate` runs. When two branches each add a migration and are merged, an older pending version can sort before an already applied newer one; apply runs it after the newer ones. Only `validate` catches forks, so a production deploy that skips validate applies migrations out of order against a snapshot chain that no longer matches.

Expected: apply refuses (non-zero exit, clear message) when the pending set has a fork, or a pending version older than the latest applied version.

## Acceptance criteria

- [x] apply exits non-zero and applies nothing when a pending version is older than the latest applied version
- [x] apply exits non-zero on a forked snapshot chain with the same error validate reports
- [x] a linear chain still applies unchanged
- [x] a regression test pins the fix: red before the change, green after

## Evidence

- TestApplyRefusesPendingOlderThanApplied: history holds applied 20260103000000, pending 20260102000000 gives ErrOutOfOrder naming both versions and runs no statement. Red before (apply ran it); mutation (outOfOrder call removed) fails it. Integration twin TestApplyRefusesOutOfOrderOnPostgres runs in CI.
- TestApplyRefusesForkedChain: two snapshots sharing a parent give ErrBranchConflict, the message contains a problem ValidateOffline reports for the same directory, and no migration runs. Mutation (Chain call removed) fails it.
- TestApplyLinearChainStillApplies: a linear two-snapshot chain applies both migrations; the existing engine tests (no snapshots, WithoutSum) pass unchanged.
- Red first for all three: Apply had no chain or order check, so both refusal tests failed with a nil error before the change.

