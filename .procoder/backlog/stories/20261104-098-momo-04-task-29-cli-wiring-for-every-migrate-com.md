# momo-04 Task 29: CLI wiring for every migrate command and exit codes

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 29: CLI wiring for every migrate command and exit codes (`.procoder/plans/momo-04-migrations.md:2950`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-10, S-11, S-26, S-27, S-28, S-46. Files: `cmd/momo/migrate.go`, `cmd/momo/main.go`. Contributes to S-11, S-27, S-28, S-46; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [x] `TestCLIContracts` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestOfflineCommandsNoDatabase` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestCLIExitOneOnDrift` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestWindowsPaths` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-10] `TestCLIContracts`: each command's exit codes and outputs as documented in Interfaces; offline commands run with no database URL set; fails if an offline command opens a connection.
- [ ] [S-26] The windows-latest CI job runs `go test ./...` for the generator, CLI and SQLite+sqlite-vec packages; `TestWindowsPaths` asserts generated imports and migration paths use forward slashes; fails if the job is skipped.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

### Implementation evidence (2026-11-05)

- Red first: with only the tests written, `go vet` and `go test ./cmd/momo` failed with `undefined: runMigrate`. Green after `cmd/momo/migrate.go`: `MOMO_REQUIRE_INTEGRATION=1 ... go test -tags integration -race -count=1 ./cmd/...` is `ok` (TestCLIContracts, TestOfflineCommandsNoDatabase, TestCLIExitOneOnDrift against the running PostgreSQL, plus TestMigrateLifecycle, TestMigrateDiffRefusesOverwrite, TestMigrateBaselineAndResolve, TestMigrateUsageErrors, TestMigrateRedactsURL).
- Deviation, tests: the plan's momo.yaml snippets lack `version: 1`, which `config.Load` requires, so the tests add it.
- Ticked, `TestWindowsPaths` (2026-11-05): decision was to follow the plan, so migrations may live in nested directories. `Discover`, `ComputeSum`, `VerifySum` and `AppendSum` now walk the tree with `fs.WalkDir`, with slash-separated paths, the root `momo.sum` skipped, and files ordered by version. The second subtest is restored as written in the plan (it asserts `archive/2026/20260101000000_init.up.sql` is recorded); only the file modes are the gosec-clean 0o750 and 0o600. It failed first (nested file not recorded) and passes now: `go test ./cmd/momo -run TestWindowsPaths -count=1 -v` is PASS for both subtests, and `GOOS=windows go vet ./cmd/momo/` is clean. Added tests: `TestDiscoverWalksNestedDirectoriesOrderedByVersion`, `TestDiscoverRejectsVersionInTwoDirectories`, `TestSumWalksNestedDirectories`, and the integration test `TestMovingAppliedMigrationIntoArchiveKeepsHistory` (apply, move into archive/2026, refresh momo.sum: Plan and Status show nothing pending, the checksum gate passes).
- NOT ticked, S-10: wired diff, plan, apply, status, validate [--db], baseline, drift and resolve with exit codes 0/1/2/3. `rebase`, `lint`, `rollback` and `new --custom` have no engine support yet (Engine.Rebase, Engine.Lint, down migrations and custom-migration creation do not exist) and return exit 2 "not available yet"; the plan does not explicitly allow that, so the criterion stays open until those engine stories land.
- NOT ticked, S-26: the windows-latest CI job was not run or changed here.

### Review fixes (2026-11-05)

- Redaction (finding 1): `TestRedactURL` calls `redactURL` directly with fake wrapped errors that carry raw passwords: URL form, percent-encoded password, a decoded password given as a secret, raw `@`, `/` and `#` in the password, `?password=` and another secret in a query, key=value DSN (plain and quoted) and a URL inside a longer message. The key=value case was not covered by `redactURL` at all, so a `password=` rule was added. Mutation kills (each reintroduced, then restored): `redactURL` as a no-op, the secrets loop removed, the userinfo regex removed, the URL-with-query rule removed, and the key=value rule removed all fail `TestRedactURL`.
- Overwrite refusal (finding 2): files are published with `os.Link`, which fails when the target exists, so a file that appears after the early check is still never replaced (with an exclusive-create fallback where hard links are unsupported); `momo.sum` keeps the rename. `TestPublishNewNeverReplaces` fails when `Link` is replaced by `Rename`. `TestMigrateDiffRefusesOverwrite` now fixes the clock (`migrateClock`) and pre-creates the exact target (a directory of that name, which the migration walk ignores). It asserts exit 1 naming the target and that nothing else was written, and it fails when the fixed clock is ignored, so it can no longer return early.
- Partial state (finding 3): `writeNewFiles` writes and fsyncs every temporary first, publishes the migration files, renames `momo.sum` last, then fsyncs the directory. Any failure removes all temporaries; a failure after some files were published removes those new files. `TestWriteNewFilesRollsBackWhenSumFails` (a non-empty directory named momo.sum) fails when the removal of published files, or the temporary cleanup, is dropped. Dotfiles and dot directories (`.momo-*.tmp`, `.DS_Store`, `.git`) are skipped everywhere in the walk used by `Discover`, `ComputeSum` and `VerifySum`; every other file, including a nested README, stays hashed, as the plan's "every file" requires. `TestWalkSkipsDotEntries` fails without the rule. The `momo migrate hash` command that the validate and apply messages name did not exist; it now recomputes `momo.sum` and writes it atomically (exit 0; 2 for a missing directory; 3 for I/O), tested by `TestMigrateHashRepairsSum`. The plan's Task 29 command list now includes it.
- Unknown command (finding 4): the word goes through `errf`; `TestMigrateUnknownCommandIsRedacted` (`momo migrate postgres://u:topsecret@h/db`) fails with the raw print.
- Help (finding 5): `baseline -h` and `resolve -h` exit 0, and flags are parsed once (the argument checks moved into the shared engine setup). `TestMigrateHelpAndStrayArguments`.
- Exit mappings (finding 6), now in the help text: `ErrBaselineMismatch` and `ErrBranchConflict` exit 1 (findings); `ErrNoSnapshot` exits 2 (the user must run diff); an unreadable migration directory in `validate` exits 3 (execution error, per the plan's table). Pinned by `TestMigrateValidateUnreadableDirectoryExitsThree`.
- Wire tests (finding 7): `TestMigrateApplyLockWait` covers a held advisory lock (exit 3 naming the holder after `--lock-wait 600ms`; success once released; `--force-unlock` cannot take a live PostgreSQL advisory lock, which the help now says: the flag targets the SQLite lock row). `TestMigrateStatusExitsOneOnDirty` pins status exit 1. validate, plan, apply, status, drift, baseline, resolve and hash reject a stray positional with exit 2 and redact it (`TestMigrateHelpAndStrayArguments`, `TestMigrateStrayArgumentIsRedacted`). Mutations: ignoring `--lock-wait`, a status that never reports dirty, and the positional check removed each fail their tests. The early `Lstat` check in `writeNewFiles` is an optimisation: link plus rollback gives the same outcome without it, so removing it is an equivalent mutant.
