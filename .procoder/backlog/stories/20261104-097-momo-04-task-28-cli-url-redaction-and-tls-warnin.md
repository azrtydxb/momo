# momo-04 Task 28: CLI URL redaction and TLS warning

Status: open
Created: 2026-11-04
Epic: momo-04-migrations-core-postgresql
Sprint: -

## Description

Implements plan momo-04-migrations Task 28: CLI URL redaction and TLS warning (`.procoder/plans/momo-04-migrations.md:2894`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-46. Files: `cmd/momo/migrate_redact.go`.

## Acceptance criteria

- [ ] `TestCLIRedactsURL` exists as written in the plan, failed before the implementation, and passes now
- [ ] `TestCLITLSWarning` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-46] `TestCLIRedactsURL` runs failing CLI commands with a password-bearing URL and asserts the password never appears in output; `TestCLITLSWarning` asserts the warning for sslmode=disable on a remote host; fails if a URL is echoed.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

