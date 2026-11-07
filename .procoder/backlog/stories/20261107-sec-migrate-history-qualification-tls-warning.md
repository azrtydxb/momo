# Security: schema-qualify the history table, stronger TLS warning, tamper-detection docs

Status: open
Created: 2026-11-07
Epic: momo-04-migrations-core-postgresql
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

Security audit Lows: the momo_migrations history table is unqualified, so a search_path object can intercept it with the migrator's privileges; the TLS warning misses unset sslmode, DSN-style strings and PGSSLMODE and recommends require instead of verify-full; hash plus rehash --accept bypass tamper detection without that being documented; drift and baseline run model DDL on the live database. Audit report: .procoder/research/security-audit-2026-11-07.md and .procoder/research/maintainability-audit-2026-11-07.md.

## Acceptance criteria

- [ ] History and lock statements use a schema-qualified table name (golden or integration test)
- [ ] The TLS warning fires for unset sslmode, key=value DSNs and PGSSLMODE and recommends verify-full (tests)
- [ ] docs/migrations.md explains what hash and rehash --accept trust and what drift and baseline execute

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
