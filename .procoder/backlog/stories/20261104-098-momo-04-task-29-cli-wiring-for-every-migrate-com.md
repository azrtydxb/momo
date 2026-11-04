# momo-04 Task 29: CLI wiring for every migrate command and exit codes

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 29: CLI wiring for every migrate command and exit codes (`.procoder/plans/momo-04-migrations.md:2950`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-10, S-11, S-26, S-27, S-28, S-46. Files: `cmd/momo/migrate.go`, `cmd/momo/main.go`.

## Acceptance criteria

- [ ] `TestCLIContracts` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestOfflineCommandsNoDatabase` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestCLIExitOneOnDrift` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestWindowsPaths` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-10] `TestCLIContracts`: each command's exit codes and outputs as documented in Interfaces; offline commands run with no database URL set; fails if an offline command opens a connection.
- [ ] [S-11] `TestStagedChange` on a populated DB: add nullable column → data migration backfill → SET NOT NULL applies and all rows satisfy the constraint; fails if diff emits NOT NULL directly for a populated table without a default.
- [ ] [S-26] The windows-latest CI job runs `go test ./...` for the generator, CLI and SQLite+sqlite-vec packages; `TestWindowsPaths` asserts generated imports and migration paths use forward slashes; fails if the job is skipped.
- [ ] [S-27] `TestMigrationBranchConflict`: two migrations with the same parent snapshot fail `momo migrate validate`; `TestMigrateRebase` regenerates the later one on top of the other and the result applies with a no-change round trip; fails if two heads are accepted.
- [ ] [S-28] `TestMigrateLint` asserts each listed risky operation is reported with severity and an alternative and that `momo migrate lint` exits 1 on errors; `TestLockTimeoutApplied` asserts `SHOW lock_timeout` inside an applying migration equals the configured value; fails if apply runs without timeouts on PostgreSQL.
- [ ] [S-46] `TestCLIRedactsURL` runs failing CLI commands with a password-bearing URL and asserts the password never appears in output; `TestCLITLSWarning` asserts the warning for sslmode=disable on a remote host; fails if a URL is echoed.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

