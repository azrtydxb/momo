# Docs and build: current version strings, honest ARCHITECTURE, package doc, Go floor

Status: open
Created: 2026-11-07
Epic: momo-13-docs-release
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

Maintainability top items 1 and 4 and security M3/info: README, llms.txt, ai-guide, capabilities and supported-versions still name v0.1.0; release-please has no extra-files to keep them current; ARCHITECTURE.md presents about 12 APIs with no code (WithInterceptor, WithReplicas, MaxRows, WithMaxInList, momovet, momotest, momolog ...) as a binding contract; the root package has no package doc; go.mod's go 1.26.0 floor carries stdlib advisories. Audit report: .procoder/research/security-audit-2026-11-07.md and .procoder/research/maintainability-audit-2026-11-07.md.

## Acceptance criteria

- [ ] Every doc names the current version and release-please updates them via extra-files
- [ ] ARCHITECTURE.md marks unimplemented APIs as planned or removes them; no doc claims an API that does not exist
- [ ] The root package has a package doc comment
- [ ] The go directive or toolchain floor is raised past the stdlib advisories, CI matrix unchanged otherwise

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
