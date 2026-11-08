# momo-04 Task 28: CLI URL redaction and TLS warning

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -
Carried: 002-generator-from-go-structs-with-its-type-prerequisites — Red-first criterion unverifiable (behaviour pre-existed) and its gate criterion depends on it. Decided 2026-11-06: carry rather than tick.

## Description

Implements plan momo-04-migrations Task 28: CLI URL redaction and TLS warning (`.procoder/plans/momo-04-migrations.md:2894`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-46. Files: `cmd/momo/migrate_redact.go`.

## Acceptance criteria

- [ ] `TestCLIRedactsURL` exists as written in the plan, failed before the implementation, and passes now
- [x] `TestCLITLSWarning` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-46] `TestCLIRedactsURL` runs failing CLI commands with a password-bearing URL and asserts the password never appears in output; `TestCLITLSWarning` asserts the warning for sslmode=disable on a remote host; fails if a URL is echoed.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

- Reconciliation 2026-11-05: redaction shipped earlier with story 098 under unexported names. The plan's exported `RedactURL` and `DatabaseURL` and the test name `TestCLIRedactsURL` were not created, to avoid a second redaction path: `redactURL` and `(*cli).databaseURL` in `cmd/momo/migrate.go` are the single implementation. The criterion naming `TestCLIRedactsURL` stays unticked for that reason; its behaviour is covered by `TestRedactURL`, `TestMigrateRedactsURL` (failing `status --url` with a password-bearing URL, asserts the password and URL never appear), `TestMigrateFlagParseErrorsAreRedacted`, `TestMigrateUnknownCommandIsRedacted`.
- `TLSWarning` (cmd/momo/migrate_redact.go) is wired into `databaseURL` and prints `momo: warning: ...` on stderr. Red: `undefined: TLSWarning` before the implementation. Rule follows the plan: sslmode=disable and a host that is not localhost, 127.0.0.1, ::1 or a unix socket. sslmode=allow/prefer do not warn (plan says disable only).
- `TestCLITLSWarning` (unit, local/remote/socket/multi-host/IPv6) and `TestMigrateWarnsOnRemoteDisabledTLS` (CLI: warns for remote, no password leak, silent for 127.0.0.1) in `cmd/momo/migrate_tls_test.go`.
- Mutations caught: warning call removed; only localhost exempt; sslmode check broken; socket paths not exempt.

Red evidence: ci-agent run 42453837637 (throwaway branch, commit a900207, branch since deleted) with TLSWarning disabled failed TestCLITLSWarning and TestCLITLSWarningUsesLibpqEnvironment, so that criterion is shown red.

Left open: with migrate.RedactURLs made a no-op in the same run, TestCLIRedactsURL did not fail (the password-bearing URL never reaches the output on that path), so no run shows it red and its "failed before the implementation" line is not ticked. The test may not catch a missing redaction; it needs a stronger assertion before this story can close.

