# Testing: a query registry listing compiled SQL of named queries (issue #66)

Status: open
Created: 2026-11-10
Epic: momo-09-observability-testing
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

An app that moves from sqlc loses its `.sql` files, which it linted statically and froze for N-1 compatibility tests. Kuvryn Orbit has five tests that read `queries/*.sql` and a previous-release compatibility test. Issue #66 asks for a way to list the SQL an application issues without running it.

## Acceptance criteria

- [ ] named queries can be registered as builders (`momo.RegisterQuery(name, func(...) Compilable)` or equivalent) and a helper returns each name's `Compiled` SQL per dialect, without a database (unit test)
- [ ] `momotest` offers a helper to write the registry to golden files and to fail when a registered query's SQL changes unexpectedly (test)
- [ ] a documented example shows running the previous release's registered SQL against a new schema (docs page, compile-checked)
- [ ] registering the same name twice panics with a clear message (test)
