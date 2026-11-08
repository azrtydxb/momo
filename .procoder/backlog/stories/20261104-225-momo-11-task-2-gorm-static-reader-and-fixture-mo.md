# momo-11 Task 2: GORM static reader and fixture module

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: -

## Description

Implements plan momo-11-adoption Task 2: GORM static reader and fixture module (`.procoder/plans/momo-11-adoption.md:124`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-91. Files: `importer/gorm/read.go`.

## Acceptance criteria

- [x] `TestImportGORM` exists (adapted as listed under Deviations: PostgreSQL instead of SQLite, schema built by the real generator), failed before the implementation, and passes now
- [x] `TestReadReportsImplicitSaveWithPosition` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-91, part] `TestImportGORM` passes for the part of S-91 this task delivers; the full criterion is verified in story 227.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- `TestImportGORM` (internal/compat/gormfixture, PostgreSQL) now emits the imported models, loads them with the real generator (gen.Load) and diffs the generated schema against the schema GORM AutoMigrate creates. Green in ci-agent run 42461526110 on b54c9fa: the only difference is the composite index the importer reports as dropped. Red before: run 42460543015 on 7691e35 failed with the real differences (GORM names a plain unique constraint uni_<table>_<column>); the first version of the test, which compared hand-built schemas, was also proven red on a throwaway branch (run 42458280952).
- importer/gorm/read_test.go (fixture models, keys decided before columns, has-many key on the child, embedded prefixes, ignored tags and bad numbers, alias-qualified DeletedAt, escaped semicolons) and the emit tests in importer/report_test.go: green in run 42461526110.

- `procoder check` on the tree this story ships is clean (0 blocking). The gate lints the nested fixture module through an untracked go.work at the worktree root, which CI never sees.

## Deviations from the plan

- Momo shapes two things differently on purpose, which the test normalises on GORM's side and the importer reports: foreign keys are named <table>_<column>_fkey, and a plain UNIQUE column becomes a unique index of the same name. Composite indexes are reported as dropped-index and not imported.
- The fixture compares against PostgreSQL (gorm.io/driver/postgres), not SQLite: only the PostgreSQL migration dialect exists.
- zero-value-handling is reported for non-pointer fields that have a default tag (GORM replaces the zero value with the default on create and skips zero fields in struct updates), not for every field without a default.
- importer.Field gained Serial, UniqueIndex, IndexName and UniqueName, and importer.Model gained ForeignKeys, so index names, serial keys and constraints survive the import.
- The fixture module's go.mod and go.sum were produced by a temporary GitHub Actions job (go get at latest, go mod tidy); gorm v1.31.2 and driver/postgres v1.6.3.
- GORM's integer sizes were checked against gorm.io/driver/postgres v1.6.3 DataTypeOf: up to 16 bits smallint, up to 32 integer, otherwise bigint, an unsigned type one bit wider, varchar up to 10485760; sql.NullInt32 takes int32 from its first field. importer/gorm/read_test.go (TestReadSizesFollowTheDialector) covers every size. The fixture module proves them against a real GORM database: internal/compat/gormfixture/sized.go has int32, uint16, int8 and sql.NullInt32 columns and a has-many with OnDelete SET NULL, and TestImportGORM diffs them through the generator.
- Reported rather than guessed: a TableName that does not return a literal, a NamingStrategy in the package (naming warning; the importer assumes default naming), every gorm tag option it does not handle (check, comment, ...), composite indexes (dropped-index), a pointer type tagged not null, and unique together with uniqueIndex.

## Scope of this story

The `momo import gorm` command and the importer documentation page belong to story 228 (the import command), and the migration guides to stories 237 and 238; neither is part of this reader and fixture story. Story 227 verifies the full S-91 criterion.
