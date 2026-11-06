# History error column is not URL-redacted and --url is visible in the process list

Status: done 2026-11-06
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: 003-sprint-3-postgresql-hardening-migration-safety-bugs-query
Type: bug
Severity: s4

## Description

The `error` text stored in the history table comes from the server and is not passed through the connection-string redactor (migrate/runner.go ~260), so a URL echoed in a server message can be persisted. Separately, `--url` puts credentials in the process list on shared hosts.

Expected: history errors are redacted before storing; the CLI prefers env or stdin for the URL and warns on --url.

## Acceptance criteria

- [x] a server error containing a URL is stored redacted in history
- [x] a documented env or stdin route exists for the URL
- [x] a test pins both
- [x] a regression test pins the fix: red before the change, green after

## Evidence

- TestHistoryErrorIsRedacted (transactional and no-tx): a server error echoing postgres://admin:s3cretpw@... is stored as postgres://***@... in the rolled_back and failed rows, rest of the message kept. Mutation (RedactURLs removed in runner.go) fails it. TestRedactURLs pins the shared redactor.
- Routes: MOMO_DATABASE_URL, migrate.url_env and the new `--url -` (stdin), documented in docs/migrations.md and docs/ai-guide.md; TestMigrateURLRoutes covers env without warning and stdin (exit 3 on an unreachable host, no leak, no warning), empty stdin exits 2.
- TestMigrateURLRoutes: `--url` prints the process-list warning naming MOMO_DATABASE_URL; mutations (warning discarded, stdin case broken) fail it.
- Red first: the history error was stored verbatim and there was no stdin route or warning.

