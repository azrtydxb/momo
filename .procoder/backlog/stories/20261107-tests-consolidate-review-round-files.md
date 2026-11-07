# Tests: consolidate test files named after review rounds

Status: open
Created: 2026-11-07
Epic: momo-01-foundation
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

Maintainability item 2: about 16 test files are named after review rounds (_review, _polish, _more, _fixes, _repair, _hardening) and load alone has 14 files. Audit report: .procoder/research/security-audit-2026-11-07.md and .procoder/research/maintainability-audit-2026-11-07.md.

## Acceptance criteria

- [ ] Test files are organised by the production file they test; no review-round suffixes remain
- [ ] A repo check refuses those suffixes
- [ ] Test count unchanged or higher (CI)

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
