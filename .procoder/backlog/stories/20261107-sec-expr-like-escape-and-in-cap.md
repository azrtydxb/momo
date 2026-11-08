# Security: LIKE ESCAPE clause and a bound-parameter cap on In

Status: done 2026-11-07
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

Security audit M1 and a Low: Like and ILike emit no ESCAPE clause, so user input escaped with EscapeLike still acts as a wildcard; typed In and NotIn have no bound-parameter cap. Audit report: .procoder/research/security-audit-2026-11-07.md and .procoder/research/maintainability-audit-2026-11-07.md.

## Acceptance criteria

- [x] Like, ILike and their Not forms emit the ESCAPE clause matching EscapeLike on every dialect (golden tests)
- [x] An escaped user value containing % and _ matches only literally (integration test on CI)
- [x] In and NotIn over more values than the dialect's bind-parameter limit return ErrValidation instead of failing at the driver (test)
- [x] docs describe safe LIKE with user input

## Evidence

Verified on CI only, run 42401247171 on commit ee72c02 (conclusion success, all jobs including unit, race, fuzz-smoke and integration on pg16 and pg17). The integration job runs with MOMO_REQUIRE_INTEGRATION=1 per the Makefile, so the integration test cannot skip.

- Like, ILike and their Not forms emit ESCAPE: TestLikeFormsCarryEscapeClausePerDialect (PostgreSQL, SQLite, MySQL goldens, Not wrappers), TestILikeStillUnsupportedOnSQLite, existing TestContainsLiteralSQLPerDialect. Not forms are Not around Like and ILike; no separate NotLike exists.
- Escaped value with % and _ matches literally: TestLikeWithEscapedUserInputMatchesLiterally (integration tag, Like on every dialect, ILike on PostgreSQL, intentional wildcard still works).
- In and NotIn over the bind limit return ErrValidation: TestTypedInOverBindLimitIsValidationError (all three dialect limits, exact limit passes), TestTypedInRespectsMaxParamsAndPriorBinds (WithMaxParams, prior binds). The tuple IN path has the same guard.
- Docs: docs/ai-guide.md section on safe LIKE with user input and In cap; docs/capabilities.md typed predicates row.
