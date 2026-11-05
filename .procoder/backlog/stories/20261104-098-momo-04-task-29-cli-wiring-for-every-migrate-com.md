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
- [ ] `TestWindowsPaths` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-10] `TestCLIContracts`: each command's exit codes and outputs as documented in Interfaces; offline commands run with no database URL set; fails if an offline command opens a connection.
- [ ] [S-26] The windows-latest CI job runs `go test ./...` for the generator, CLI and SQLite+sqlite-vec packages; `TestWindowsPaths` asserts generated imports and migration paths use forward slashes; fails if the job is skipped.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

### Implementation evidence (2026-11-05)

- Red first: with only the tests written, `go vet` and `go test ./cmd/momo` failed with `undefined: runMigrate`. Green after `cmd/momo/migrate.go`: `MOMO_REQUIRE_INTEGRATION=1 ... go test -tags integration -race -count=1 ./cmd/...` is `ok` (TestCLIContracts, TestOfflineCommandsNoDatabase, TestCLIExitOneOnDrift against the running PostgreSQL, plus TestMigrateLifecycle, TestMigrateDiffRefusesOverwrite, TestMigrateBaselineAndResolve, TestMigrateUsageErrors, TestMigrateRedactsURL).
- Deviation, tests: the plan's momo.yaml snippets lack `version: 1`, which `config.Load` requires, so the tests add it.
- NOT ticked, `TestWindowsPaths`: the plan's second subtest expects `migrate.ComputeSum` to record `archive/2026/...`, but the engine (ComputeSum, Discover) only handles the top level of the migration directory by design. The test was adapted to pin the actual behaviour (nested files not recorded, top-level names bare, no backslash in momo.sum), so it is not "as written". Decision for the lead: recurse in ComputeSum and Discover, or amend the plan.
- NOT ticked, S-10: wired diff, plan, apply, status, validate [--db], baseline, drift and resolve with exit codes 0/1/2/3. `rebase`, `lint`, `rollback` and `new --custom` have no engine support yet (Engine.Rebase, Engine.Lint, down migrations and custom-migration creation do not exist) and return exit 2 "not available yet"; the plan does not explicitly allow that, so the criterion stays open until those engine stories land.
- NOT ticked, S-26: the windows-latest CI job was not run or changed here.
