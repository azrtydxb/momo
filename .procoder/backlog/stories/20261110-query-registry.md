# Testing: a query registry listing compiled SQL of named queries (issue #66)

Status: done
Created: 2026-11-10
Epic: momo-09-observability-testing
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

An app that moves from sqlc loses its `.sql` files, which it linted statically and froze for N-1 compatibility tests. Kuvryn Orbit has five tests that read `queries/*.sql` and a previous-release compatibility test. Issue #66 asks for a way to list the SQL an application issues without running it.

## Acceptance criteria

- [x] named queries can be registered as builders (`momo.RegisterQuery(name, func(...) Compilable)` or equivalent) and a helper returns each name's `Compiled` SQL per dialect, without a database (unit test)
- [x] `momotest` offers a helper to write the registry to golden files and to fail when a registered query's SQL changes unexpectedly (test)
- [x] a documented example shows running the previous release's registered SQL against a new schema (docs page, compile-checked)
- [x] registering the same name twice panics with a clear message (test)

## Evidence

All evidence is ci-agent run 42774576317 (headSha 2813509754f03ba58eb66c93c04f0bae141ce7b7), all 9 jobs green.

- Register and compile per dialect without a database: raw_registry_test.go (TestQueryRegistryCompilesPerDialect, TestQueryRegistryCompileErrorNamesQuery, TestRegisterQueryUsesDefaultRegistry, TestOfflineNeverExecutes).
- Golden helper that fails on changed, added or removed SQL: momotest/querygolden_test.go (TestQueryGoldenDetectsChanges, TestParseQueryGoldenRoundTrip).
- Documented N-1 compatibility example: docs/testing.md "Listing the SQL: a query registry", a complete program compiled by docscheck (TestDocsCodeBlocks). It is compiled, not executed.
- Duplicate name panics naming the query: TestQueryRegistryDuplicatePanics.
