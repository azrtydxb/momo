# plain-struct Query ignores the naming preset

Status: done 2026-11-11
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #92, found by NovaCRM's port from Prisma.

Reproduction: `momo.Query[T]` into a plain struct whose fields are untagged,
against SQL that returns camelCase columns (`endpointId`, `nextAttemptAt`), as
under `naming: prisma`.

Observed vs expected: every field is named by snake_case (`endpoint_id`), so
the result is an `ErrValidation` unless each field has a `column:` tag. The
project's naming preset should name the columns.

Root cause: `collectStructFields` in raw_struct.go called a private snake_case
and had no way to reach the preset; the reflection plan was cached by type only.

Fix (design, see report): `momo.WithNaming(preset)` stores the preset on the DB
(and so on its transactions, `OnConn` and replica handles); `Query` and
`QueryRows` read it from the Querier and pass it to the plan builder, which
caches plans by (type, preset). Without the option the column is
`naming.ToSnake`, as before. A `column:` tag still wins.

## Acceptance criteria

- [x] a regression test pins the fix: red before the change, green after
- [x] under a camelCase preset an untagged plain struct scans camelCase
      columns through both Query and QueryRows, a tag still wins, and without
      the option the snake_case default is unchanged
- [x] docs/upgrading.md (0.8.0), docs/ai-guide.md and docs/capabilities.md
      describe WithNaming
- [x] docs/reference.md lists the new API, taken from the CI artifact

## Evidence

- Red: test-only commit 2c52ec6 on throwaway branch worktree-agent-red-92, ci-agent run 42978359451 failed to build the root test package: undefined: momo.WithNaming (raw_struct_test.go:235 and :246). Branch deleted.
- Green: run 42999792466 on c4a7d80 (branch worktree-agent-a565a4b39df5f7711), all ci-agent jobs green. Earlier runs on this branch failed for fixable reasons only: the core may not import gen/naming (moved to internal/snake), docs/reference.md rows, and the googleuuid golden.
- Docs: docs/upgrading.md (0.8.0, WithNaming), docs/ai-guide.md, docs/capabilities.md, docs/reference.md (FieldNamer and WithNaming rows, taken from the gen-check diff of the run on this branch because the regenerated-docs artifact carried rows of other branches).
