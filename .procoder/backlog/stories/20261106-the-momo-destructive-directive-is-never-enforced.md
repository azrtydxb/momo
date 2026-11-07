# The momo:destructive directive is never enforced

Status: done 2026-11-06
Created: 2026-11-06
Epic: momo-04-migrations-core-postgresql
Sprint: 003-sprint-3-postgresql-hardening-migration-safety-bugs-query
Type: bug
Severity: s3

## Description

`-- momo:destructive` is an accepted directive (migrate/files.go 73) and ErrDestructive text points to it (migrate/errors.go 17), but nothing reads it on apply. A hand-written DROP TABLE or DROP COLUMN applies with no check, and the error text implies a guard exists.

Expected: either apply requires an explicit opt-in for files that contain destructive statements or carry the directive, or the directive and the error text are corrected to say it is informational only.

## Acceptance criteria

- [x] apply of a file with the destructive directive or a DROP statement is refused without an explicit opt-in flag, or the directive is documented and worded as informational
- [x] ErrDestructive text matches actual behaviour
- [x] generated destructive migrations keep working with --allow-destructive
- [x] a regression test pins the fix: red before the change, green after

## Evidence

- Chosen resolution: the directive is informational. docs/migrations.md section 5, docs/ai-guide.md, docs/capabilities.md and the Directives godoc say apply does not read it.
- TestDestructiveErrorTextMatchesBehavior: ErrDestructive no longer mentions the directive and names --allow-destructive; fails with the old text.
- TestDiffDestructiveHeader (existing) and TestDestructiveErrorTextMatchesBehavior: diff with AllowDestructive still writes -- momo:destructive; without it ErrDestructive is returned.
- TestDestructiveErrorTextMatchesBehavior: red before the change, green after.
