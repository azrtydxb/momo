# Refactor: one diff path for views, functions, triggers and policies

Status: open
Created: 2026-11-07
Epic: momo-04-migrations-core-postgresql
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

Maintainability item 9: migrate/diff.go has four copy-pasted diff blocks for views, functions, triggers and policies although a generic helper exists. Audit report: .procoder/research/security-audit-2026-11-07.md and .procoder/research/maintainability-audit-2026-11-07.md.

## Acceptance criteria

- [ ] The four object kinds diff through one generic helper
- [ ] Migration golden and integration tests unchanged (CI)

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
