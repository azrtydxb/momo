# momo-01 Task 18: Test database URL helpers and PostgreSQL containers

Status: open
Created: 2026-11-04
Epic: momo-01-foundation
Sprint: -

## Description

Implements plan momo-01-foundation Task 18: Test database URL helpers and PostgreSQL containers (`.procoder/plans/momo-01-foundation.md:1562`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-20, S-47. Files: `internal/testdb/testdb.go`, `internal/testdb/isolation_integration_test.go`, `internal/testdb/compose.yaml`, `go.mod`.

## Acceptance criteria

- [ ] `TestResolve` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestEnvVarNames` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestSQLitePathIsInTempDir` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestRandomSuffixIsUniqueAndSafe` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestPostgresURLIsolated` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-20] `TestCIWorkflowCoverage` parses .github/workflows/ci.yml and asserts it runs unit, generator, compile-fail, race and fuzz-smoke tests, plus integration tests against pgvector/pgvector:pg17 and SQLite+sqlite-vec, on Go 1.26 and 1.27; fails if any job is skipped.
- [ ] [S-47] `TestAppRoleCannotDDL` runs in integration CI, which connects as a non-superuser application role; `TestAppRoleCannotDDL` asserts DDL fails for it; fails if tests run as superuser.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

