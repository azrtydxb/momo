# The momo:destructive directive is never enforced

Status: open
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: -
Type: bug
Severity: s3

## Description

`-- momo:destructive` is an accepted directive (migrate/files.go 73) and ErrDestructive text points to it (migrate/errors.go 17), but nothing reads it on apply. A hand-written DROP TABLE or DROP COLUMN applies with no check, and the error text implies a guard exists.

Expected: either apply requires an explicit opt-in for files that contain destructive statements or carry the directive, or the directive and the error text are corrected to say it is informational only.

## Acceptance criteria

- [ ] apply of a file with the destructive directive or a DROP statement is refused without an explicit opt-in flag, or the directive is documented and worded as informational
- [ ] ErrDestructive text matches actual behaviour
- [ ] generated destructive migrations keep working with --allow-destructive
- [ ] a regression test pins the fix: red before the change, green after

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->

