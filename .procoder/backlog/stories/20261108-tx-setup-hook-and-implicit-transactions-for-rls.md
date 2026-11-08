# Per-transaction setup hook and implicit transactions for RLS tenant scoping

Status: done 2026-11-08
Created: 2026-11-08
Epic: momo-03-drivers-postgresql
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

Multi-tenant applications that use row-level security need every statement to run in a transaction that first sets a transaction-local setting such as app.tenant_id from the request context (issue #42). Today only an explicit transaction can call Tx.SetLocal; auto-commit statements on a *DB and the transactions momo opens itself (chunked inserts, graph creation, relation writes) run without any setup, and Hook.BeforeQuery cannot run SQL on the statement's connection. Done means the DB can be configured with a setup hook that runs after every transaction momo begins, and with an opt-in mode that wraps auto-commit statements in short transactions, safe behind PgBouncer in transaction mode.

## Acceptance criteria

- [x] WithTxSetup runs once after every Begin, DB.Tx and internally opened transaction, not per savepoint, and a setup error rolls back and is returned (integration test on CI)
- [x] WithImplicitTx runs Select, Insert, Update, Delete, Exec and Scalars in a short transaction with the setup applied (integration test with a real RLS policy: a forgotten scope returns zero rows, a scoped one returns only the tenant's rows)
- [x] Iter, Each and Rows commit when exhausted or closed and roll back on error, with no connection leak (integration test checking the pool's acquired connections)
- [x] Chunked inserts apply the setup in their internal transactions (integration test: WITH CHECK policy, forced multi-chunk insert)
- [x] Only transaction-local set_config is used, safe behind PgBouncer in transaction mode (integration test through PGBOUNCER_URL with concurrent tenants)
- [x] docs have a multi-tenant RLS section

## Evidence

CI run 42412048167 green on 54b8285 (lint, unit, fuzz-smoke, PostgreSQL integration with MOMO_REQUIRE_INTEGRATION=1, so the tests cannot skip).

- Setup once per real transaction, error rolls back: TestTxSetupRunsOncePerTransactionNotPerSavepoint, TestTxSetupErrorRollsBack.
- Implicit transactions, fail-closed RLS, chunked inserts, streaming and no connection leak: TestImplicitTxRLSFailClosed, TestTxSetupWithoutImplicitTxLeavesAutoCommitUnscoped.
- PgBouncer in transaction mode with concurrent tenants: TestImplicitTxPgBouncer.
- Docs: multi-tenant row-level security section in docs/capabilities.md.
