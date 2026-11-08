# momo-11 Task 2: GORM static reader and fixture module

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: -

## Description

Implements plan momo-11-adoption Task 2: GORM static reader and fixture module (`.procoder/plans/momo-11-adoption.md:124`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-91. Files: `importer/gorm/read.go`.

## Acceptance criteria

- [x] `TestImportGORM` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestReadReportsImplicitSaveWithPosition` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-91, part] `TestImportGORM` passes for the part of S-91 this task delivers; the full criterion is verified in story 227.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `TestImportGORM` (internal/compat/gormfixture, PostgreSQL): green in ci-agent run 42457430544 on 304bcb8, zero diff between the schema GORM AutoMigrate creates and the schema the imported models describe. Red before: run 42458280952 on throwaway branch commit cbc8b20 (foreign keys dropped from the imported schema) failed with a drop_fk change; that branch is deleted.
- `TestReadReportsImplicitSaveWithPosition`, `TestReadFixtureModels` and more in importer/gorm/read_test.go: green in the same run. They first failed in run 42456706948 (a test-input mistake and a false composite-index report, both fixed).

## Deviations from the plan

- The fixture compares against PostgreSQL (gorm.io/driver/postgres), not SQLite: only the PostgreSQL migration dialect exists.
- zero-value-handling is reported for non-pointer fields that have a default tag (GORM replaces the zero value with the default on create and skips zero fields in struct updates), not for every field without a default.
- importer.Field gained Serial, UniqueIndex, IndexName and UniqueName, and importer.Model gained ForeignKeys, so index names, serial keys and constraints survive the import.
- The fixture module's go.mod and go.sum were produced by a temporary GitHub Actions job (go get at latest, go mod tidy); gorm v1.31.2 and driver/postgres v1.6.3.

