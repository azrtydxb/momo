# momo-04 drift detects out-of-band view, trigger and policy body changes

Status: done 2026-11-10
Created: 2026-11-05
Epic: momo-04-migrations-core-postgresql
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

As an operator running `momo migrate drift`, I need a view query, trigger WHEN condition or row-level security policy expression (USING, WITH CHECK) that someone changed directly in the database to be reported as Changed, so that a clean drift report means those bodies match the migrations.

Today Drift compares these objects by identity and definition apart from their text, because PostgreSQL rewrites that text (pg_get_viewdef, pg_get_expr, pg_get_triggerdef) and canonicalizing the snapshot side needs a scratch schema (DDL, and the CREATE privilege on the database). Task 18 therefore lists them in `DriftReport.Unverified` instead. Table text (checks, defaults, index predicates) is a different case: Drift already canonicalizes it in a rolled-back scratch transaction and, when it cannot, falls back to the declared text and reports the tables in `DriftReport.CanonicalizeSkipped`. That reported fallback is acceptable for table text, where the worst outcome is a phantom difference; it is not acceptable for the bodies in this story, where a silent skip hides a real change, so the requirement below stands for views, triggers and policies. This is security relevant: an RLS policy loosened out of band (for example `USING (true)`) leaves the report Clean.

Done means Drift detects a changed body without executing DDL against the application schema, or documents and enforces the exact privilege it needs, and `Unverified` shrinks to the objects that still cannot be compared.

## Acceptance criteria

- [x] An RLS policy whose USING or WITH CHECK is changed in the live database after a baseline is reported in `DriftReport.Changed` with a Detail naming `using` or `check`.
- [x] A view whose query is changed out of band is reported in Changed; a view declared in a different spelling than pg_get_viewdef reports is not.
- [x] A trigger whose WHEN condition is changed out of band is reported in Changed; a differently spelled but equivalent condition is not.
- [x] Drift stays read-only: with the counting driver, no DDL statement runs against the application schema, or the privilege required is documented and a missing privilege returns an error wrapping `ErrCanonicalizePrivilege` rather than a silent skip.
- [x] `DriftReport.Unverified` no longer lists objects whose bodies are now compared, and its doc comment says which kinds remain.
- [x] An integration test as the non-superuser `momo_app` role covers the chosen privilege behaviour.

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->

### Sprint 7 (2026-11-10)

Drift now canonicalizes a snapshot that declares view, trigger WHEN or policy text strictly (the Baseline step: a scratch schema in a transaction that is always rolled back) and compares that text (`migrate/drift.go`, `hasBodyText`). The chosen privilege option: it needs CREATE on the database and a writable session, and a role without it gets an error wrapping `ErrCanonicalizePrivilege`, never a skip. A snapshot without such bodies keeps the tolerant table-text path.

- Criterion 1: `TestDriftReportsOutOfBandBodyChanges/policy_using` (USING (true)) and `TestDriftReportsOutOfBandPolicyCheckChange` (detail names `using` / `check`) in `migrate/postgres/drift_bodies_test.go`.
- Criterion 2: `.../view_query` and `.../materialized_view_query` report Changed; `TestDriftBodiesEquivalentSpellingIsClean` declares `id>0` against the server's `id > 0` and the report is Clean.
- Criterion 3: `.../trigger_when`; the same clean test covers the equivalent spelling `new.id>0`.
- Criterion 4: `TestDriftBodiesRunsDDLOnlyInRolledBackTransaction` (every CREATE/ALTER/DROP between a BEGIN and its ROLLBACK, no COMMIT or write, no scratch schema left) and the privilege error of criterion 6.
- Criterion 5: `TestDriftBodiesEquivalentSpellingIsClean` asserts `Unverified` is empty; the `Unverified` and `Drift` doc comments say it lists objects only for a dialect that cannot canonicalize. Unit: `TestCompareExactReportsBodyChanges`, `TestHasBodyText` in `migrate/drift_bodies_internal_test.go`.
- Criterion 6: `TestDriftBodiesNeedCreateOnDatabase` (as `momo_app`: ErrCanonicalizePrivilege naming CREATE and the database) and `TestDriftTablesOnlyRunsAsApplicationRole` (no bodies: no privilege needed, tables listed in CanonicalizeSkipped).
- Red: ci-agent run 42770799423 on throwaway branch worktree-agent-ad6b3740d97d2a3ae-red (headSha 907b7f3616dd7c898a868df2175a0915abb01cfb): the new tests with the implementation absent: `TestDriftReportsOutOfBandBodyChanges` (all four subtests) and `TestDriftBodiesEquivalentSpellingIsClean` fail.
- Green: ci-agent run 42776939577 on 0dafa15e799e4e052ca9b05bc94eed3b231d421f, all jobs success. Docs: docs/migrations.md (Drift, roles), docs/capabilities.md.
