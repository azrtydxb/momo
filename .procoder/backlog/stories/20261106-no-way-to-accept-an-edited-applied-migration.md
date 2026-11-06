# No way to accept an edited applied migration

Status: open
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: -
Type: bug
Severity: s3

## Description

After an applied migration file is edited, `migrate hash` rewrites momo.sum but the checksum in the history table stays, so apply stays blocked. The only route is a manual `UPDATE momo_migrations SET checksum = ...`.

Expected: a command such as `migrate rehash --version V --accept` that updates the history checksum for a named version after printing the diff, and records the action.

## Acceptance criteria

- [ ] a rehash command with --version and --accept updates the history checksum for that version only
- [ ] without --accept nothing changes and the command explains why
- [ ] after rehash, status and apply no longer report a checksum mismatch
- [ ] history rows of other versions are untouched
- [ ] a regression test pins the fix: red before the change, green after

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->

