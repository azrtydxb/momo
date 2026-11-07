# Docs and build: current version strings, honest ARCHITECTURE, package doc, Go floor

Status: done 2026-11-07
Created: 2026-11-07
Epic: momo-13-docs-release
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

Maintainability top items 1 and 4 and security M3/info: README, llms.txt, ai-guide, capabilities and supported-versions still name v0.1.0; release-please has no extra-files to keep them current; ARCHITECTURE.md presents about 12 APIs with no code (WithInterceptor, WithReplicas, MaxRows, WithMaxInList, momovet, momotest, momolog ...) as a binding contract; the root package has no package doc; go.mod's go 1.26.0 floor carries stdlib advisories. Audit report: .procoder/research/security-audit-2026-11-07.md and .procoder/research/maintainability-audit-2026-11-07.md.

## Acceptance criteria

- [x] Every doc names the current version and release-please updates them via extra-files
- [x] ARCHITECTURE.md marks unimplemented APIs as planned or removes them; no doc claims an API that does not exist
- [x] The root package has a package doc comment
- [x] The go directive or toolchain floor is raised past the stdlib advisories, CI matrix unchanged otherwise

## Evidence

- Versions: README, llms.txt, ai-guide, capabilities, migrations, supported-versions and the example READMEs name v0.3.0 on lines tagged x-release-please-version; release-please-config.json lists them under extra-files. internal/repocheck TestDocsNameManifestVersion asserts every tagged line carries the manifest version and the key docs are listed; the quickstart README test reads the manifest. CI run 42402102352.
- ARCHITECTURE.md: APIs without code (MaxRows, WithMaxInList, typed Raw, Interceptor, WithInterceptor, replicas and unit of work, Meta.Hooks, View and WritableModeler, RefreshMaterializedView, GoMigration, Engine Lint, Rebase and online Validate, sqlite and mysql drivers and dialects, importers, momotest, momolog, momovet, otelmomo, bench, generator plugins) moved to a Planned, not implemented section; names that exist were corrected.
- doc.go adds the root package comment pointing to docs/ai-guide.md.
- go.mod is go 1.26.10 (latest 1.26 patch on go.dev/dl); CI setup-go 1.26 and 1.27 resolve to the latest patch; temp-module tests declare the same floor; docs state it.
