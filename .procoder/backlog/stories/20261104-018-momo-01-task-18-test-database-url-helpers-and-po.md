# momo-01 Task 18: Test database URL helpers and PostgreSQL containers

Status: open
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: 001-foundation-of-the-query-core-dialects-schema-base-types

## Description

Implements plan momo-01-foundation Task 18: Test database URL helpers and PostgreSQL containers (`.procoder/plans/momo-01-foundation.md:1562`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-20, S-47. Files: `internal/testdb/testdb.go`, `internal/testdb/isolation_integration_test.go`, `internal/testdb/compose.yaml`, `go.mod`. Contributes to S-20, S-47; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestResolve` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestEnvVarNames` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSQLitePathIsInTempDir` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestRandomSuffixIsUniqueAndSafe` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestPostgresURLIsolated` exists as written in the plan, failed before the implementation, and passes now
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

