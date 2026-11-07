# Security: LIKE ESCAPE clause and a bound-parameter cap on In

Status: open
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

Security audit M1 and a Low: Like and ILike emit no ESCAPE clause, so user input escaped with EscapeLike still acts as a wildcard; typed In and NotIn have no bound-parameter cap. Audit report: .procoder/research/security-audit-2026-11-07.md and .procoder/research/maintainability-audit-2026-11-07.md.

## Acceptance criteria

- [ ] Like, ILike and their Not forms emit the ESCAPE clause matching EscapeLike on every dialect (golden tests)
- [ ] An escaped user value containing % and _ matches only literally (integration test on CI)
- [ ] In and NotIn over more values than the dialect's bind-parameter limit return ErrValidation instead of failing at the driver (test)
- [ ] docs describe safe LIKE with user input

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
