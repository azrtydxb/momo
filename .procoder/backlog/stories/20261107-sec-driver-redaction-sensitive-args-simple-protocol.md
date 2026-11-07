# Security: redact encode errors, mask sensitive args, refuse simple protocol

Status: open
Created: 2026-11-07
Epic: momo-03-drivers-postgresql
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

Security audit M2, M3 and a Low: pgx client-side encode errors quoting bound values reach callers unredacted from Query and Exec; arguments of fields tagged sensitive reach hooks when argument logging is on; FromSQLDB and FromConnConfig accept simple protocol, which interpolates values client-side. Audit report: .procoder/research/security-audit-2026-11-07.md and .procoder/research/maintainability-audit-2026-11-07.md.

## Acceptance criteria

- [ ] Query and Exec errors are scrubbed of bound values like COPY and batch errors (test)
- [ ] Hooks receive a masked value for arguments bound from sensitive fields (test)
- [ ] FromSQLDB and FromConnConfig refuse or override simple protocol with a clear error or documented behaviour (test)
- [ ] docs state the guarantees

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
