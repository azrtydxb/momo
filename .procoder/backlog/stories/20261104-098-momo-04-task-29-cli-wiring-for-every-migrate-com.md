# momo-04 Task 29: CLI wiring for every migrate command and exit codes

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Implements plan momo-04-migrations Task 29: CLI wiring for every migrate command and exit codes (`.procoder/plans/momo-04-migrations.md:2950`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-10, S-11, S-26, S-27, S-28, S-46. Files: `cmd/momo/migrate.go`, `cmd/momo/main.go`. Contributes to S-11, S-27, S-28, S-46; those criteria are verified in the stories that define their named tests.

## Acceptance criteria

- [ ] `TestCLIContracts` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestOfflineCommandsNoDatabase` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestCLIExitOneOnDrift` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestWindowsPaths` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-10] `TestCLIContracts`: each command's exit codes and outputs as documented in Interfaces; offline commands run with no database URL set; fails if an offline command opens a connection.
- [ ] [S-26] The windows-latest CI job runs `go test ./...` for the generator, CLI and SQLite+sqlite-vec packages; `TestWindowsPaths` asserts generated imports and migration paths use forward slashes; fails if the job is skipped.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

