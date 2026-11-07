# Refactor: split InsertQuery modes and unify select rendering

Status: open
Created: 2026-11-07
Epic: momo-05-writes-queries
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

Maintainability items 6 and 8: InsertQuery is a five-mode type in a 1072-line insert.go that duplicates transaction scope from db.go; renderBody is 160 lines and ExprSelect is a second partial renderer. Audit report: .procoder/research/security-audit-2026-11-07.md and .procoder/research/maintainability-audit-2026-11-07.md.

## Acceptance criteria

- [ ] insert.go is split by mode with shared rendering and no duplicated transaction scope
- [ ] Select and ExprSelect share one renderer; all golden tests unchanged
- [ ] No behaviour change (full CI)

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
