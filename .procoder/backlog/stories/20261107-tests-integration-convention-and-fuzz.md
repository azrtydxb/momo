# Tests: one integration-test convention and every fuzz target in CI

Status: open
Created: 2026-11-07
Epic: momo-01-foundation
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

Maintainability item 3 and fuzz note: integration tests use three conventions (build tag, filename, runtime skip) and untagged ones silently skip on the macOS and Windows jobs; 5 of 15 fuzz targets never run in CI. Audit report: .procoder/research/security-audit-2026-11-07.md and .procoder/research/maintainability-audit-2026-11-07.md.

## Acceptance criteria

- [ ] Every integration test uses the integration build tag and testdb; a CI check fails on a database test without the tag
- [ ] macOS and Windows jobs state they run unit tests only
- [ ] Every fuzz target runs in fuzz-smoke

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
