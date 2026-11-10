# momo-04 drift detects out-of-band view, trigger and policy body changes

Status: open
Created: 2026-11-05
Epic: momo-04-migrations-core-postgresql
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

As an operator running `momo migrate drift`, I need a view query, trigger WHEN condition or row-level security policy expression (USING, WITH CHECK) that someone changed directly in the database to be reported as Changed, so that a clean drift report means those bodies match the migrations.

Today Drift compares these objects by identity and definition apart from their text, because PostgreSQL rewrites that text (pg_get_viewdef, pg_get_expr, pg_get_triggerdef) and canonicalizing the snapshot side needs a scratch schema (DDL, and the CREATE privilege on the database). Task 18 therefore lists them in `DriftReport.Unverified` instead. Table text (checks, defaults, index predicates) is a different case: Drift already canonicalizes it in a rolled-back scratch transaction and, when it cannot, falls back to the declared text and reports the tables in `DriftReport.CanonicalizeSkipped`. That reported fallback is acceptable for table text, where the worst outcome is a phantom difference; it is not acceptable for the bodies in this story, where a silent skip hides a real change, so the requirement below stands for views, triggers and policies. This is security relevant: an RLS policy loosened out of band (for example `USING (true)`) leaves the report Clean.

Done means Drift detects a changed body without executing DDL against the application schema, or documents and enforces the exact privilege it needs, and `Unverified` shrinks to the objects that still cannot be compared.

## Acceptance criteria

- [ ] An RLS policy whose USING or WITH CHECK is changed in the live database after a baseline is reported in `DriftReport.Changed` with a Detail naming `using` or `check`.
- [ ] A view whose query is changed out of band is reported in Changed; a view declared in a different spelling than pg_get_viewdef reports is not.
- [ ] A trigger whose WHEN condition is changed out of band is reported in Changed; a differently spelled but equivalent condition is not.
- [ ] Drift stays read-only: with the counting driver, no DDL statement runs against the application schema, or the privilege required is documented and a missing privilege returns an error wrapping `ErrCanonicalizePrivilege` rather than a silent skip.
- [ ] `DriftReport.Unverified` no longer lists objects whose bodies are now compared, and its doc comment says which kinds remain.
- [ ] An integration test as the non-superuser `momo_app` role covers the chosen privilege behaviour.

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
