# Advisory xact lock helpers and FOR UPDATE OF

Status: done 2026-11-07
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: -

## Description

GitHub issue #32. A port of NovaCRM needs two locking constructs that today go through raw SQL: transaction-scoped advisory locks (snapshot writes guard a logical resource with pg_advisory_xact_lock over hashtext of a name) and FOR UPDATE OF <table> SKIP LOCKED on a joined select (the webhook dispatcher claims delivery rows through a join). Done means Tx helpers for both advisory lock forms with int64 and string keys, refused outside a transaction, and a LockOf select option that names the locked tables, quoted, with ErrUnsupported on dialects without row locks.

## Acceptance criteria

- [x] Tx.AdvisoryXactLock, AdvisoryXactLockString, TryAdvisoryXactLock and TryAdvisoryXactLockString exist and render the pg_advisory_xact_lock family, string keys through hashtext.
- [x] The same names on DB fail with ErrValidation; non-PostgreSQL transactions fail with ErrUnsupported.
- [x] SelectQuery.LockOf renders FOR UPDATE or FOR SHARE OF with quoted names before NOWAIT or SKIP LOCKED, in any call order; empty names and LockOf without a lock mode fail with ErrValidation; SQLite fails with ErrUnsupported.
- [x] Unit goldens cover the rendering and the refusals.
- [x] Integration test on PostgreSQL: two connections, SKIP LOCKED job claim through a join with OF, NOWAIT busy, and advisory lock contention.
- [x] Docs list the new API and its dialect behaviour.

## Evidence

CI run 42409402654 on d9487a1 (ci-agent): unit, lint, fuzz-smoke and integration (postgres) all success. lockof_unit_test.go covers SQL strings and refusals; lockof_integration_test.go covers the two-transaction SKIP LOCKED claim, NOWAIT busy and advisory contention. Docs updated in capabilities.md and ai-guide.md.

