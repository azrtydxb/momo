# Security: redact encode errors, mask sensitive args, refuse simple protocol

Status: open
Created: 2026-11-07
Epic: momo-03-drivers-postgresql
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

Security audit M2, M3 and a Low: pgx client-side encode errors quoting bound values reach callers unredacted from Query and Exec; arguments of fields tagged sensitive reach hooks when argument logging is on; FromSQLDB and FromConnConfig accept simple protocol, which interpolates values client-side. Audit report: .procoder/research/security-audit-2026-11-07.md and .procoder/research/maintainability-audit-2026-11-07.md.

## Acceptance criteria

- [x] Query and Exec errors are scrubbed of bound values like COPY and batch errors (test)
- [x] Hooks receive a masked value for arguments bound from sensitive fields (test)
- [x] FromSQLDB and FromConnConfig refuse or override simple protocol with a clear error or documented behaviour (test)
- [x] docs state the guarantees

## Evidence

CI run 42402073664 (ci.yml, SHA caf220e) concluded success; the integration jobs run with MOMO_REQUIRE_INTEGRATION=1 per the Makefile.

- Query and Exec errors scrubbed: TestClassifyErrorRedactsEncodeErrors, TestRowsAndTxErrorsAreRedacted (unit, no database) and TestQueryAndExecEncodeErrorsAreRedacted (integration, pool and database/sql adapter) pass; errors.Is on the cause still holds.
- Hooks get a masked value: TestHooksSeeMaskedSensitiveInsertArgs, TestHooksSeeMaskedSensitiveUpdateArgs and TestHookCannotMutateBoundArgs pass; the fake driver still receives the real value.
- Simple protocol refused: TestFromConnConfigRefusesSimpleProtocol (unit) and TestFromSQLDBRefusesSimpleProtocolOnFirstUse (integration) return ErrUnsupported; TestFromSQLDBAcceptsExtendedProtocol passes.
- Docs: section "Secrets in errors, hooks and connections" in docs/capabilities.md.
