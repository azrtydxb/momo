# Security: schema-qualify the history table, stronger TLS warning, tamper-detection docs

Status: done 2026-11-07
Created: 2026-11-07
Epic: momo-04-migrations-core-postgresql
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

Security audit Lows: the momo_migrations history table is unqualified, so a search_path object can intercept it with the migrator's privileges; the TLS warning misses unset sslmode, DSN-style strings and PGSSLMODE and recommends require instead of verify-full; hash plus rehash --accept bypass tamper detection without that being documented; drift and baseline run model DDL on the live database. Audit report: .procoder/research/security-audit-2026-11-07.md and .procoder/research/maintainability-audit-2026-11-07.md.

## Acceptance criteria

- [x] History and lock statements use a schema-qualified table name (golden or integration test)
- [x] The TLS warning fires for unset sslmode, key=value DSNs and PGSSLMODE and recommends verify-full (tests)
- [x] docs/migrations.md explains what hash and rehash --accept trust and what drift and baseline execute

## Evidence

CI run 42401446787 on commit 3e3354d, conclusion success, including the integration jobs on pg16 and pg17 (go 1.26 and 1.27) that run with MOMO_REQUIRE_INTEGRATION=1.

- History and lock statements schema-qualified: every history statement (migrate/history.go, runner.go, rehash.go, engine.go Resolve, baseline.go) uses the quoted reference from Dialect.HistoryTable, resolved once under the lock. Tests in migrate/postgres/history_table_test.go: TestHistoryTableFindsExistingUnqualifiedInstall (an unqualified install in another schema is found, applies, and no second table appears), TestHistoryTableNewInstallIsQualified (the DDL and the new table are qualified), TestHistoryTableRefusesShadowingObject (a shadowing view and a temporary table are refused, not used).
- TLS warning: TestCLITLSWarning (unset sslmode, key=value strings, allow and prefer, verify-full recommended, no URL echo, local hosts quiet) and TestCLITLSWarningUsesLibpqEnvironment (PGSSLMODE, PGHOST) in cmd/momo/migrate_tls_test.go.
- docs/migrations.md: History table section (qualification, no SET search_path, rationale) and Security section (hash and rehash trust, drift and baseline run model DDL, use a scratch database).

