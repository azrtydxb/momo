# momo-08 Task 5: migrate/mysql inspection and rendering

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-08-mysql
Sprint: -

## Description

Implements plan momo-08-mysql Task 5: migrate/mysql inspection and rendering (`.procoder/plans/momo-08-mysql.md:680`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-31, S-60. Files: `migrate/mysql/inspect.go`, `migrate/mysql/render.go`, `migrate/mysql/normalize_test.go`, `migrate/mysql/roundtrip_integration_test.go`. Contributes to S-31; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestMySQLNormalizeType` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestMySQLMigrationRoundTrip` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-60, part] `TestMySQLMigrationRoundTrip` passes for the part of S-60 this task delivers; the full criterion is verified in story 204.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Green run: ci-agent run 42464754775 on 4fbb8ddff7eea92368afd359ec94377897e03c89 (earlier green run: 42462329606) (lint, unit, fuzz-smoke, integration postgres, integration-mysql mysql84, mysql97 and mariadb all success). Each MySQL job runs the roundtrip subtest of its own server.
- Red run: ci-agent run 42460447363 on the throwaway branch worktree-agent-aa139b45101cda3e1-red (sprint-5 plus the tests only, branch deleted): the unit and all integration jobs failed with `undefined: lockName`, `undefined: Dialect` and `undefined: applyDDLDirective`, so TestMySQLNormalizeType and TestMySQLMigrationRoundTrip did not build without the implementation. The red state is a compile failure, not a value mismatch, because the package has no non-test file there.
- TestMySQLNormalizeType: migrate/mysql/normalize_test.go, the plan's cases plus alias and idempotence cases; implementation is the MySQL branch of schema.NormalizeType (mysqlType in schema/normalize.go). A MySQL case was added to the FuzzNormalize dialect loop.
- TestMySQLMigrationRoundTrip: migrate/mysql/roundtrip_integration_test.go. It differs from the plan text in two ways, both because of server rules: it uses the current engine API (migratetest.NewEngine, Engine.Diff, Apply, Inspect, Drift), and the example schema moves the CHECK and the stored generated column off the foreign key columns (MySQL refuses both on a column a foreign key with a referential action uses; this comes from the MySQL reference manual and was not observed on CI) and declares the generated column nullable (MariaDB does not take NOT NULL there; also from the documentation, not observed). The test also covers a default, a comment, a datetime(6) default and a json column (MariaDB's longtext alias). It asserts schema.Equal of the inspected and declared schema, a clean Drift, an empty migrate.Diff and no new migration from Engine.Diff.
- Inspection: migrate/mysql/inspect.go (information_schema queries with bound schema name, MySQL and MariaDB variants for STATISTICS and CHECK_CONSTRAINTS). Rendering: migrate/mysql/render.go, unit tests in migrate/mysql/render_test.go.
- Gate: the procoder commit gate (golangci-lint and formatters) ran on every commit of this branch and passed.
- Review round (collations, default spellings, widened round trip): the roundtrip now covers collated, unsigned, tinyint(1), enum, set, NOW/CURRENT_TIMESTAMP/TRUE defaults, an expression default, a CHECK with and/or/in, DESC, prefix, unique prefix, fulltext and (MySQL only) functional indexes, and foreign keys with SET NULL, RESTRICT and ON UPDATE, on all three servers; TestMySQLDropForeignKey proves the implicit index goes with a dropped key. Green in run 42464754775 on 4fbb8ddff7eea92368afd359ec94377897e03c89 (the final code commit).
