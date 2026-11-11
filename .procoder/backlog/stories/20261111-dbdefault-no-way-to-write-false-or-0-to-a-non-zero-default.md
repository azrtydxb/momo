# dbdefault: no way to write false or 0 to a non-zero default (#90)

Status: done 2026-11-11
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #90. A `dbdefault` field treats its Go zero value as "unset", so a
column such as `active boolean NOT NULL DEFAULT true` or `level int NOT NULL
DEFAULT 3` can never store an explicit `false` or `0` through `Insert.Model`:
the column is left out of the INSERT and the database default replaces the
value. Since #77 (`introspect.dbdefault: all`) tags every defaulted column, the
bug spreads: the default now applies, but an explicit false or 0 is silently
replaced.

Reproduce: model `Active bool` tagged `default:true,dbdefault`; insert
`Account{Active: false}`; the row stores true.

Expected: a way to tell "unset" from "false/0". Decision of the user
(2026-11-11): allow `dbdefault` on a pointer field of a NOT NULL column. nil
means "use the database default" (column omitted from INSERT, read back through
RETURNING); a non-nil value is always written, false and 0 included.

Root cause: the generator refused `dbdefault` on any pointer field (the field
was always treated as nullable, so nil had to mean NULL), leaving no way to
express "unset" other than the zero value. The runtime already treats a nil
pointer as unset and a pointer to a zero value as set, and copies the stored
value back into the pointer, so the change is in the generator: a `dbdefault`
pointer keeps the column NOT NULL and the descriptor non-nullable.

## Acceptance criteria

- [x] a regression test pins the fix: red before the change, green after
- [x] `dbdefault` on a pointer field of a NOT NULL column is accepted by the generator; the column stays NOT NULL and the descriptor flags carry no nullable bit; `sql.Null*` types and the `nullable` tag are still refused
- [x] PostgreSQL integration: a nil pointer takes the database default and the stored value is read back into the pointer; an explicit false and an explicit 0 are written (also in a mixed `Models` batch)
- [x] SQLite and MySQL: explicit false and 0 are written; a nil pointer is read back on SQLite where RETURNING exists, and refused with ErrUnsupported on MySQL (no RETURNING), which is documented
- [x] `momo generate --from-db` with `introspect.dbdefault: all` gives a NOT NULL bool or number column with a non-zero constant default a pointer type; zero, text and computed defaults keep the plain type
- [x] `momo generate` warns when a non-pointer dbdefault field is a bool or number whose default is a non-zero constant
- [x] docs/ai-guide.md no longer says false always stores true without the pointer escape; docs/upgrading.md has a 0.8.0 section; docs/reference.md is regenerated from CI
- [x] the full ci-agent run is green

## Evidence

- Regression test, red before the change: ci-agent run 42978931588 (branch
  worktree-agent-red-90, test-only commit d7fdbb6, branch since deleted). Unit job:
  `--- FAIL: TestDBDefaultPointerIsNotNull` (`dbdefault_test.go:90: field Active:
dbdefault on a nullable field`) and `--- FAIL: TestDBDefaultAllTagsConstantDefaults`
  (`scope_test.go:191`: `Version *int64` and `Active *bool` patterns not matched).
- Green after the change: ci-agent run 42986332089 at headSha
  3c572e1f4adc56d374ed01a0786443b8ce54ca21, all 9 jobs success (unit, lint,
  integration postgres, integration-mysql mysql84/mysql97/mariadb, fuzz-smoke,
  bench-module, apidiff-docs). A first green attempt (42979271308, 5881eb0) failed on
  my own missing `regexp` import, a docs code-fence rule and the internal/repocheck
  package list; fixed in 3c572e1.
- Generator accepts a dbdefault pointer (NOT NULL column, no nullable flag), still
  refuses sql.Null* and the nullable tag: gen/dbdefault_test.go.
- PostgreSQL/SQLite/MySQL: TestDBDefaultPointerLive (insert_dbdefault_ptr_integration_test.go)
  passed in the integration jobs: nil takes the default and is read back, explicit false
  and 0 are written, also in a mixed Models batch; MySQL (no RETURNING) refuses a nil
  pointer with ErrUnsupported and still writes explicit values.
- --from-db: gen/fromdb/scope_test.go and TestGenerateFromDBDBDefaultAll (postgres job).
- Warning: gen/dbdefault_warn_test.go (unit job).
- Docs: docs/ai-guide.md, docs/migrations.md, docs/upgrading.md (Upgrading to 0.8.0).
  The regenerated-docs and regenerated-integration artifacts were not produced for
  this run (nothing to regenerate); apidiff-docs and docscheck passed.
