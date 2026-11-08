# Refactor: one comparison method set and consistent error construction

Status: open
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

Maintainability items 5 and 7: the comparison method set is written seven times (Column, Agg, exprOps, permission wrappers) and Agg already lacks methods; errors are built through 10 helpers and 28 inline literals, and some bare errors.New values cannot be matched with errors.Is. Audit report: .procoder/research/security-audit-2026-11-07.md and .procoder/research/maintainability-audit-2026-11-07.md.

## Acceptance criteria

- [ ] One shared comparison implementation backs Column, Agg, expressions and permission wrappers; Agg gains the missing methods (tests)
- [ ] Every library error is matchable with errors.Is against a sentinel (test sweeping error sites)
- [ ] No public API removed without a deprecation

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
