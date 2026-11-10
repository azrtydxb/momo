# momo-04 Task 26: Migration lint with severities and safer alternatives

Status: done 2026-11-10
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-04-migrations Task 26: Migration lint with severities and safer alternatives (`.procoder/plans/momo-04-migrations.md:2764`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-28. Files: `migrate/lint.go`.

## Acceptance criteria

- [x] `TestMigrateLint` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-28] `TestMigrateLint` asserts each listed risky operation is reported with severity and an alternative and that `momo migrate lint` exits 1 on errors; `TestLockTimeoutApplied` asserts `SHOW lock_timeout` inside an applying migration equals the configured value; fails if apply runs without timeouts on PostgreSQL.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

### Sprint 7 (2026-11-10)

Built `migrate/lint.go`: `LintSQL`, `Engine.Lint` (pending, needs a database), `Engine.LintAll` (whole directory, offline), `HasErrors`, rules R1-R7. `momo migrate lint [--db]` is wired in `cmd/momo/migrate.go`.

- `TestMigrateLint` (migrate/lint_test.go): the plan's five cases plus check, truncate, drop column and a quoted qualified table, each reported with error severity and an alternative. Also `TestMigrateLintQuietOnSafeSQL`, `TestMigrateLintSetNotNullIsAWarning`, `TestMigrateLintSQLiteRebuildAndEnum`, `TestEngineLintAllReadsTheDirectory`.
- Red: ci-agent run 42770799423 on throwaway branch worktree-agent-ad6b3740d97d2a3ae-red (headSha 907b7f3616dd7c898a868df2175a0915abb01cfb): the new tests with the implementation absent: migrate package build failed, `undefined: RuleNotNullWithoutDefault` and others; `TestMigrateLintExitCodes` failed.
- Exit 1 on errors: `TestMigrateLintExitCodes` (error exits 1, warnings alone and clean exit 0).
- `TestLockTimeoutApplied` (migrate/postgres/lock_test.go) asserts `SHOW lock_timeout` inside an applying migration; it passed in the green run.
- Green: ci-agent run 42776939577 on 0dafa15e799e4e052ca9b05bc94eed3b231d421f, all jobs success.
- Deviations from the plan: `SET NOT NULL` is a warning (the staged flow ends with it), the `--db` row-count rule for SQLite rebuilds is not built because `momo migrate` does not select SQLite, and the `-- momo:guard` alternative text names `--enforce-not-null`, the shipped design.
