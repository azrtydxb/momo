# History error column is not URL-redacted and --url is visible in the process list

Status: open
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: -
Type: bug
Severity: s4

## Description

The `error` text stored in the history table comes from the server and is not passed through the connection-string redactor (migrate/runner.go ~260), so a URL echoed in a server message can be persisted. Separately, `--url` puts credentials in the process list on shared hosts.

Expected: history errors are redacted before storing; the CLI prefers env or stdin for the URL and warns on --url.

## Acceptance criteria

- [ ] a server error containing a URL is stored redacted in history
- [ ] a documented env or stdin route exists for the URL
- [ ] a test pins both
- [ ] a regression test pins the fix: red before the change, green after

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->

