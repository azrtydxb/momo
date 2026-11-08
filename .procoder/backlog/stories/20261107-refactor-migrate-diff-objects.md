# Refactor: one diff path for views, functions, triggers and policies

Status: done 2026-11-07
Created: 2026-11-07
Epic: momo-04-migrations-core-postgresql
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

Maintainability item 9: migrate/diff.go has four copy-pasted diff blocks for views, functions, triggers and policies although a generic helper exists. Audit report: .procoder/research/security-audit-2026-11-07.md and .procoder/research/maintainability-audit-2026-11-07.md.

## Acceptance criteria

- [x] The four object kinds diff through one generic helper
- [x] Migration golden and integration tests unchanged (CI)

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->

- Generic helper: views, functions, triggers and policies all go through diffObjects in migrate/diff.go (commit 18c7c90); the four copy-pasted blocks are gone.
- Tests unchanged: no existing test edited; one test added (TestDiffTriggerPolicyChanges). GitHub Actions run 42403306963 on 18c7c90 concluded success, including the integration jobs.
